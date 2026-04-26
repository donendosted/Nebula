package nebula

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"
)

const (
	configDirName  = "nebula"
	configFileName = "config.json"
	walletsDirName = "wallets"
)

type walletFile struct {
	Name               string          `json:"name"`
	Address            string          `json:"address"`
	CreatedAt          time.Time       `json:"created_at"`
	TestnetFundingUsed int             `json:"testnet_funding_used"`
	EncryptedSecret    encryptedSecret `json:"encrypted_secret"`
}

type configFile struct {
	Network      Network `json:"network"`
	ActiveWallet string  `json:"active_wallet"`
}

// Store manages encrypted wallet metadata and active-wallet configuration.
type Store struct {
	baseDir string
}

// NewStore builds a wallet store rooted at the platform config directory.
func NewStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return &Store{baseDir: filepath.Join(dir, configDirName)}, nil
}

// BaseDir returns the root nebula config directory.
func (s *Store) BaseDir() string {
	return s.baseDir
}

// WalletsDir returns the encrypted wallet directory.
func (s *Store) WalletsDir() string {
	return filepath.Join(s.baseDir, walletsDirName)
}

// ConfigPath returns the active-wallet config path.
func (s *Store) ConfigPath() string {
	return filepath.Join(s.baseDir, configFileName)
}

// WalletPath returns the per-wallet metadata path.
func (s *Store) WalletPath(address string) string {
	return filepath.Join(s.WalletsDir(), address+".json")
}

// Ensure creates secure storage directories.
func (s *Store) Ensure() error {
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.baseDir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return err
	}
	if err := os.MkdirAll(s.WalletsDir(), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.WalletsDir(), 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return err
	}
	return nil
}

// CreateWallet generates, encrypts, stores, and activates a new wallet.
func (s *Store) CreateWallet(name, passphrase string) (WalletMeta, error) {
	full, err := keypair.Random()
	if err != nil {
		return WalletMeta{}, fmt.Errorf("generate keypair: %w", err)
	}
	return s.saveWallet(name, full.Seed(), passphrase)
}

// ImportWallet stores an existing Stellar seed as an encrypted wallet.
func (s *Store) ImportWallet(name, secret, passphrase string) (WalletMeta, error) {
	return s.saveWallet(name, strings.TrimSpace(secret), passphrase)
}

// ListWallets returns all stored wallets with the active one first.
func (s *Store) ListWallets() ([]WalletMeta, error) {
	if err := s.Ensure(); err != nil {
		return nil, fmt.Errorf("prepare config dir: %w", err)
	}
	entries, err := os.ReadDir(s.WalletsDir())
	if err != nil {
		return nil, fmt.Errorf("read wallets dir: %w", err)
	}

	active := ""
	cfg, err := s.loadConfig()
	if err == nil {
		active = cfg.ActiveWallet
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	wallets := make([]WalletMeta, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		meta, err := s.loadWalletMeta(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		meta.Active = meta.Address == active
		wallets = append(wallets, meta)
	}
	if len(wallets) == 0 {
		return nil, ErrWalletNotFound
	}
	sort.Slice(wallets, func(i, j int) bool {
		if wallets[i].Active != wallets[j].Active {
			return wallets[i].Active
		}
		if wallets[i].Name != wallets[j].Name {
			return strings.ToLower(wallets[i].Name) < strings.ToLower(wallets[j].Name)
		}
		return wallets[i].Address < wallets[j].Address
	})
	return wallets, nil
}

// ActiveWallet returns the currently selected wallet metadata.
func (s *Store) ActiveWallet() (WalletMeta, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WalletMeta{}, ErrWalletNotFound
		}
		return WalletMeta{}, err
	}
	if strings.TrimSpace(cfg.ActiveWallet) == "" {
		return WalletMeta{}, ErrWalletNotFound
	}
	meta, err := s.loadWalletMeta(cfg.ActiveWallet)
	if err != nil {
		return WalletMeta{}, err
	}
	meta.Active = true
	return meta, nil
}

// SwitchActiveWallet changes the active wallet by address or exact name.
func (s *Store) SwitchActiveWallet(identifier string) (WalletMeta, error) {
	wallets, err := s.ListWallets()
	if err != nil {
		return WalletMeta{}, err
	}
	identifier = strings.TrimSpace(identifier)
	for _, item := range wallets {
		if item.Address == identifier || item.Name == identifier {
			cfg, err := s.loadConfig()
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return WalletMeta{}, err
			}
			cfg.ActiveWallet = item.Address
			if err := s.saveConfig(cfg); err != nil {
				return WalletMeta{}, err
			}
			item.Active = true
			return item, nil
		}
	}
	return WalletMeta{}, ErrWalletNotFound
}

// UnlockActiveWallet decrypts the active wallet with a passphrase.
func (s *Store) UnlockActiveWallet(passphrase string) (UnlockedWallet, error) {
	meta, err := s.ActiveWallet()
	if err != nil {
		return UnlockedWallet{}, err
	}
	return s.UnlockWallet(meta.Address, passphrase)
}

// UnlockWallet decrypts a stored wallet with a passphrase.
func (s *Store) UnlockWallet(identifier, passphrase string) (UnlockedWallet, error) {
	target, err := s.findWallet(identifier)
	if err != nil {
		return UnlockedWallet{}, err
	}
	record, err := s.readWalletFile(target.Address)
	if err != nil {
		return UnlockedWallet{}, err
	}
	secret, err := decryptSecret(record.EncryptedSecret, passphrase)
	if err != nil {
		return UnlockedWallet{}, err
	}
	return UnlockedWallet{
		Meta:       target,
		Secret:     secret,
		Passphrase: passphrase,
	}, nil
}

// RecordTestnetFunding increments local Friendbot usage tracking for a wallet.
func (s *Store) RecordTestnetFunding(address string) (int, error) {
	record, err := s.readWalletFile(address)
	if err != nil {
		return 0, err
	}
	record.TestnetFundingUsed++
	if err := s.writeWalletFile(record); err != nil {
		return 0, err
	}
	return record.TestnetFundingUsed, nil
}

// ActiveWalletPath returns the active wallet file path.
func (s *Store) ActiveWalletPath() (string, error) {
	meta, err := s.ActiveWallet()
	if err != nil {
		return "", err
	}
	return meta.SecretPath, nil
}

// CurrentNetwork loads the persisted default network.
func (s *Store) CurrentNetwork() (Network, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NetworkTestnet, nil
		}
		return "", err
	}
	if cfg.Network == "" {
		return NetworkTestnet, nil
	}
	return cfg.Network, nil
}

// SetNetwork persists the default network selection.
func (s *Store) SetNetwork(networkValue Network) error {
	if !networkValue.Valid() {
		return ErrUnsupportedNetwork
	}
	cfg, err := s.loadConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cfg.Network = networkValue
	return s.saveConfig(cfg)
}

// ToggleNetwork switches between testnet and mainnet.
func (s *Store) ToggleNetwork() (Network, error) {
	current, err := s.CurrentNetwork()
	if err != nil {
		return "", err
	}
	if current == NetworkTestnet {
		return NetworkMainnet, s.SetNetwork(NetworkMainnet)
	}
	return NetworkTestnet, s.SetNetwork(NetworkTestnet)
}

func (s *Store) saveWallet(name, secret, passphrase string) (WalletMeta, error) {
	full, err := parseSecret(secret)
	if err != nil {
		return WalletMeta{}, err
	}
	encrypted, err := encryptSecret(full.Seed(), passphrase)
	if err != nil {
		return WalletMeta{}, err
	}
	if err := s.Ensure(); err != nil {
		return WalletMeta{}, fmt.Errorf("prepare config dir: %w", err)
	}

	record := walletFile{
		Name:               defaultWalletName(name, full.Address()),
		Address:            full.Address(),
		CreatedAt:          time.Now().UTC(),
		TestnetFundingUsed: 0,
		EncryptedSecret:    encrypted,
	}
	if existing, err := s.readWalletFile(record.Address); err == nil {
		record.CreatedAt = existing.CreatedAt
		record.TestnetFundingUsed = existing.TestnetFundingUsed
	} else if err != nil && !errors.Is(err, ErrWalletNotFound) {
		return WalletMeta{}, err
	}

	if err := s.writeWalletFile(record); err != nil {
		return WalletMeta{}, err
	}

	cfg, err := s.loadConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return WalletMeta{}, err
	}
	cfg.ActiveWallet = record.Address
	if cfg.Network == "" {
		cfg.Network = NetworkTestnet
	}
	if err := s.saveConfig(cfg); err != nil {
		return WalletMeta{}, err
	}

	return WalletMeta{
		Name:               record.Name,
		Address:            record.Address,
		SecretPath:         s.WalletPath(record.Address),
		CreatedAt:          record.CreatedAt,
		Active:             true,
		TestnetFundingUsed: record.TestnetFundingUsed,
	}, nil
}

func (s *Store) findWallet(identifier string) (WalletMeta, error) {
	wallets, err := s.ListWallets()
	if err != nil {
		return WalletMeta{}, err
	}
	identifier = strings.TrimSpace(identifier)
	for _, item := range wallets {
		if item.Address == identifier || item.Name == identifier {
			return item, nil
		}
	}
	return WalletMeta{}, ErrWalletNotFound
}

func (s *Store) loadWalletMeta(address string) (WalletMeta, error) {
	record, err := s.readWalletFile(address)
	if err != nil {
		return WalletMeta{}, err
	}
	return WalletMeta{
		Name:               record.Name,
		Address:            record.Address,
		SecretPath:         s.WalletPath(record.Address),
		CreatedAt:          record.CreatedAt,
		TestnetFundingUsed: record.TestnetFundingUsed,
	}, nil
}

func (s *Store) readWalletFile(address string) (walletFile, error) {
	raw, err := os.ReadFile(s.WalletPath(address))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return walletFile{}, ErrWalletNotFound
		}
		return walletFile{}, fmt.Errorf("read wallet: %w", err)
	}
	var record walletFile
	if err := json.Unmarshal(raw, &record); err != nil {
		return walletFile{}, ErrCorruptWallet
	}
	if record.Address == "" || record.EncryptedSecret.Ciphertext == "" {
		return walletFile{}, ErrCorruptWallet
	}
	return record, nil
}

func (s *Store) writeWalletFile(record walletFile) error {
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode wallet: %w", err)
	}
	return writeSecureFile(s.WalletPath(record.Address), append(payload, '\n'))
}

func (s *Store) loadConfig() (configFile, error) {
	raw, err := os.ReadFile(s.ConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return configFile{}, os.ErrNotExist
		}
		return configFile{}, fmt.Errorf("read config: %w", err)
	}
	var cfg configFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return configFile{}, ErrCorruptWallet
	}
	if cfg.Network != "" && !cfg.Network.Valid() {
		return configFile{}, ErrCorruptWallet
	}
	return cfg, nil
}

func (s *Store) saveConfig(cfg configFile) error {
	if err := s.Ensure(); err != nil {
		return fmt.Errorf("prepare config dir: %w", err)
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return writeSecureFile(s.ConfigPath(), append(payload, '\n'))
}

func writeSecureFile(path string, payload []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		return err
	}
	return nil
}

func defaultWalletName(name, address string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	if len(address) > 8 {
		return "wallet-" + strings.ToLower(address[:8])
	}
	return "wallet"
}
