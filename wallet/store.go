package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nebula/internal/metrics"

	"github.com/dgraph-io/badger/v4"
)

// Store manages encrypted HD wallet roots and derived account metadata.
type Store struct {
	rootDir  string
	dbDir    string
	db       *badger.DB
	readonly bool
}

// NewStore opens the default Nebula wallet database at ~/.nebula/wallet.db.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return NewStoreAt(filepath.Join(home, defaultRootDirName))
}

// NewStoreAt opens a Nebula wallet database rooted at rootDir.
func NewStoreAt(rootDir string) (*Store, error) {
	return newStoreAt(rootDir, false)
}

// NewReadOnlyStore opens the default Nebula wallet database in read-only mode.
func NewReadOnlyStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return NewReadOnlyStoreAt(filepath.Join(home, defaultRootDirName))
}

// NewReadOnlyStoreAt opens a Nebula wallet database rooted at rootDir in read-only mode.
func NewReadOnlyStoreAt(rootDir string) (*Store, error) {
	return newStoreAt(rootDir, true)
}

func newStoreAt(rootDir string, readOnly bool) (*Store, error) {
	dbDir := filepath.Join(rootDir, defaultWalletDBDirName)
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, fmt.Errorf("create nebula directory: %w", err)
	}
	if err := os.Chmod(rootDir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return nil, fmt.Errorf("set nebula directory permissions: %w", err)
	}
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		return nil, fmt.Errorf("create wallet db directory: %w", err)
	}
	if err := os.Chmod(dbDir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return nil, fmt.Errorf("set wallet db directory permissions: %w", err)
	}
	opts := badger.DefaultOptions(dbDir)
	opts.Logger = nil
	opts.ValueDir = dbDir
	opts.ReadOnly = readOnly
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open wallet db: %w", err)
	}
	return &Store{rootDir: rootDir, dbDir: dbDir, db: db, readonly: readOnly}, nil
}

// Close releases database resources.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// RootDir returns ~/.nebula.
func (s *Store) RootDir() string {
	return s.rootDir
}

// DBDir returns the BadgerDB directory used for encrypted wallet state.
func (s *Store) DBDir() string {
	return s.dbDir
}

// ReadOnly reports whether the store was opened in read-only mode.
func (s *Store) ReadOnly() bool {
	return s != nil && s.readonly
}

// ProposalDir returns the directory used for multisig proposals.
func (s *Store) ProposalDir() string {
	return filepath.Join(s.rootDir, defaultProposalDirName)
}

// IndexDir returns the BadgerDB directory used for local transaction indexes.
func (s *Store) IndexDir() string {
	return filepath.Join(s.rootDir, defaultIndexDBDirName)
}

// CreateWallet creates a new encrypted HD wallet root and derives account 0.
func (s *Store) CreateWallet(opts CreateOptions) (WalletSummary, string, error) {
	mnemonic, err := GenerateMnemonic(opts.Words)
	if err != nil {
		return WalletSummary{}, "", err
	}
	summary, err := s.createWallet(opts.Name, mnemonic, opts.Passphrase)
	if err == nil {
		metrics.RecordWalletAction("create", summary.Name)
	}
	return summary, mnemonic, err
}

// ImportWallet imports an existing mnemonic, encrypts it, and derives account 0.
func (s *Store) ImportWallet(opts ImportOptions) (WalletSummary, error) {
	summary, err := s.createWallet(opts.Name, opts.Mnemonic, opts.Passphrase)
	if err == nil {
		metrics.RecordWalletAction("create", summary.Name)
	}
	return summary, err
}

func (s *Store) createWallet(name, mnemonic, passphrase string) (WalletSummary, error) {
	if s.ReadOnly() {
		return WalletSummary{}, fmt.Errorf("wallet store is read-only")
	}
	normalized, err := NormalizeMnemonic(mnemonic)
	if err != nil {
		return WalletSummary{}, err
	}
	if strings.TrimSpace(passphrase) == "" {
		return WalletSummary{}, ErrInvalidPassphrase
	}
	if strings.TrimSpace(name) == "" {
		name = defaultWalletName(normalized)
	}
	id := walletID(name)
	cipher, err := encryptText(normalized, passphrase)
	if err != nil {
		return WalletSummary{}, err
	}
	now := time.Now().UTC()
	record := WalletRecord{
		ID:                id,
		Name:              strings.TrimSpace(name),
		MnemonicCipher:    cipher,
		MnemonicWordCount: len(strings.Fields(normalized)),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	account, _, err := DeriveAccount(normalized, defaultAccountIndex, "primary")
	if err != nil {
		return WalletSummary{}, err
	}
	account.WalletID = id
	account.CreatedAt = now
	config, _ := s.Config()
	config.ActiveWalletID = id
	config.ActiveAccountIndex = defaultAccountIndex

	if err := s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(walletKey(id)); err == nil {
			return ErrWalletExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err := putJSON(txn, walletKey(id), record); err != nil {
			return err
		}
		if err := putJSON(txn, derivedKey(id, defaultAccountIndex), account); err != nil {
			return err
		}
		return putJSON(txn, []byte(configBucketKey), config)
	}); err != nil {
		return WalletSummary{}, err
	}
	return WalletSummary{
		WalletRecord:       record,
		Accounts:           []DerivedAccount{account},
		Active:             true,
		ActiveAccountIndex: defaultAccountIndex,
	}, nil
}

// ListWallets returns all stored wallet roots and derived accounts.
func (s *Store) ListWallets() ([]WalletSummary, error) {
	config, _ := s.Config()
	records := map[string]WalletSummary{}
	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek([]byte(walletBucketPrefix)); it.ValidForPrefix([]byte(walletBucketPrefix)); it.Next() {
			var record WalletRecord
			if err := readJSONItem(it.Item(), &record); err != nil {
				return err
			}
			records[record.ID] = WalletSummary{
				WalletRecord:       record,
				Accounts:           []DerivedAccount{},
				Active:             record.ID == config.ActiveWalletID,
				ActiveAccountIndex: config.ActiveAccountIndex,
			}
		}
		for it.Seek([]byte(derivedAccountKeyPrefix)); it.ValidForPrefix([]byte(derivedAccountKeyPrefix)); it.Next() {
			var account DerivedAccount
			if err := readJSONItem(it.Item(), &account); err != nil {
				return err
			}
			summary := records[account.WalletID]
			summary.Accounts = append(summary.Accounts, account)
			records[account.WalletID] = summary
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrWalletNotFound
	}
	out := make([]WalletSummary, 0, len(records))
	for _, summary := range records {
		sort.Slice(summary.Accounts, func(i, j int) bool {
			return summary.Accounts[i].Index < summary.Accounts[j].Index
		})
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// Config returns persisted active wallet state.
func (s *Store) Config() (Config, error) {
	var cfg Config
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(configBucketKey))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return readJSONItem(item, &cfg)
	})
	return cfg, err
}

// CurrentNetwork returns the persisted network or testnet by default.
func (s *Store) CurrentNetwork() string {
	cfg, err := s.Config()
	if err != nil {
		return "testnet"
	}
	if strings.TrimSpace(cfg.Network) == "" {
		return "testnet"
	}
	return cfg.Network
}

// SetNetwork persists the selected network string.
func (s *Store) SetNetwork(network string) error {
	if s.ReadOnly() {
		return fmt.Errorf("wallet store is read-only")
	}
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	cfg.Network = strings.TrimSpace(strings.ToLower(network))
	return s.db.Update(func(txn *badger.Txn) error {
		return putJSON(txn, []byte(configBucketKey), cfg)
	})
}

// ToggleNetwork flips between testnet and mainnet.
func (s *Store) ToggleNetwork() (string, error) {
	next := "mainnet"
	if s.CurrentNetwork() == "mainnet" {
		next = "testnet"
	}
	return next, s.SetNetwork(next)
}

// SetActiveWallet sets the selected wallet root and account index.
func (s *Store) SetActiveWallet(walletID string, accountIndex uint32) error {
	if s.ReadOnly() {
		return fmt.Errorf("wallet store is read-only")
	}
	summary, err := s.Wallet(walletID)
	if err != nil {
		return err
	}
	found := false
	for _, account := range summary.Accounts {
		if account.Index == accountIndex {
			found = true
			break
		}
	}
	if !found {
		return ErrAccountNotDerived
	}
	cfg := Config{ActiveWalletID: summary.ID, ActiveAccountIndex: accountIndex}
	return s.db.Update(func(txn *badger.Txn) error {
		return putJSON(txn, []byte(configBucketKey), cfg)
	})
}

// Wallet returns one wallet root and its derived accounts.
func (s *Store) Wallet(id string) (WalletSummary, error) {
	list, err := s.ListWallets()
	if err != nil {
		return WalletSummary{}, err
	}
	id = strings.TrimSpace(id)
	for _, item := range list {
		if item.ID == id || item.Name == id {
			return item, nil
		}
	}
	return WalletSummary{}, ErrWalletNotFound
}

// Derive persists a new account under an existing wallet root.
func (s *Store) Derive(walletID string, passphrase string, index uint32, name string) (DerivedAccount, error) {
	if s.ReadOnly() {
		return DerivedAccount{}, fmt.Errorf("wallet store is read-only")
	}
	record, mnemonic, err := s.unlockWalletRecord(walletID, passphrase)
	if err != nil {
		return DerivedAccount{}, err
	}
	account, _, err := DeriveAccount(mnemonic, index, name)
	if err != nil {
		return DerivedAccount{}, err
	}
	account.WalletID = record.ID
	account.CreatedAt = time.Now().UTC()
	if account.Name == "" {
		account.Name = fmt.Sprintf("%s-%d", record.Name, index)
	}
	err = s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(derivedKey(record.ID, index)); err == nil {
			return nil
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		record.UpdatedAt = time.Now().UTC()
		if err := putJSON(txn, walletKey(record.ID), record); err != nil {
			return err
		}
		return putJSON(txn, derivedKey(record.ID, index), account)
	})
	return account, err
}

// ActiveAccount unlocks the active wallet and returns the selected derived account with its Stellar secret seed.
func (s *Store) ActiveAccount(passphrase string) (WalletSummary, DerivedAccount, string, error) {
	cfg, err := s.Config()
	if err != nil {
		return WalletSummary{}, DerivedAccount{}, "", err
	}
	if cfg.ActiveWalletID == "" {
		return WalletSummary{}, DerivedAccount{}, "", ErrWalletNotFound
	}
	return s.UnlockAccount(cfg.ActiveWalletID, cfg.ActiveAccountIndex, passphrase)
}

// UnlockAccount decrypts a wallet root and returns the derived account plus its secret seed.
func (s *Store) UnlockAccount(walletID string, index uint32, passphrase string) (WalletSummary, DerivedAccount, string, error) {
	record, mnemonic, err := s.unlockWalletRecord(walletID, passphrase)
	if err != nil {
		return WalletSummary{}, DerivedAccount{}, "", err
	}
	summary, err := s.Wallet(record.ID)
	if err != nil {
		return WalletSummary{}, DerivedAccount{}, "", err
	}
	var account DerivedAccount
	found := false
	for _, item := range summary.Accounts {
		if item.Index == index {
			account = item
			found = true
			break
		}
	}
	if !found {
		return WalletSummary{}, DerivedAccount{}, "", ErrAccountNotDerived
	}
	derived, secret, err := DeriveAccount(mnemonic, index, account.Name)
	if err != nil {
		return WalletSummary{}, DerivedAccount{}, "", err
	}
	derived.WalletID = account.WalletID
	derived.CreatedAt = account.CreatedAt
	return summary, derived, secret, nil
}

// Confirm rejects sensitive operations unless the caller explicitly confirmed them.
func Confirm(confirm bool, action SensitiveAction) error {
	if confirm {
		return nil
	}
	if strings.TrimSpace(action.Reason) == "" {
		return ErrSensitiveActionRejected
	}
	return fmt.Errorf("%w: %s", ErrSensitiveActionRejected, action.Reason)
}

func (s *Store) unlockWalletRecord(walletID string, passphrase string) (WalletRecord, string, error) {
	record, err := s.walletRecord(walletID)
	if err != nil {
		return WalletRecord{}, "", err
	}
	mnemonic, err := decryptText(record.MnemonicCipher, passphrase)
	if err != nil {
		return WalletRecord{}, "", err
	}
	return record, mnemonic, nil
}

func (s *Store) walletRecord(id string) (WalletRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return WalletRecord{}, ErrWalletNotFound
	}
	var record WalletRecord
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(walletKey(id))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrWalletNotFound
		}
		if err != nil {
			return err
		}
		return readJSONItem(item, &record)
	})
	return record, err
}

func walletID(name string) string {
	name = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), "-"))
	if name == "" {
		name = "wallet"
	}
	return name
}

func defaultWalletName(mnemonic string) string {
	words := strings.Fields(mnemonic)
	if len(words) >= 2 {
		return words[0] + "-" + words[1]
	}
	return "wallet"
}

func walletKey(id string) []byte {
	return []byte(walletBucketPrefix + id)
}

func derivedKey(walletID string, index uint32) []byte {
	return []byte(fmt.Sprintf("%s%s:%08d", derivedAccountKeyPrefix, walletID, index))
}

func putJSON(txn *badger.Txn, key []byte, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return txn.Set(key, payload)
}

func readJSONItem(item *badger.Item, target any) error {
	return item.Value(func(val []byte) error {
		return json.Unmarshal(val, target)
	})
}
