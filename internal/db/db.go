package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nebula/indexer"
	"nebula/wallet"
)

// Handles holds the shared Badger-backed stores for one process.
type Handles struct {
	Wallet         *wallet.Store
	Index          *indexer.Store
	WalletReadOnly bool
}

// OpenForTUI opens the Nebula stores once for the lifetime of the TUI process.
//
// Wallet storage prefers read-write mode, but falls back to read-only mode when
// another writer already owns the lock so the TUI can still inspect data.
// Index storage is opened read-only because TUI refresh paths should never write.
func OpenForTUI() (*Handles, error) {
	walletStore, err := wallet.NewStore()
	walletReadOnly := false
	if err != nil {
		if !IsLockError(err) {
			return nil, err
		}
		walletStore, err = wallet.NewReadOnlyStore()
		if err != nil {
			return nil, friendlyLockError(err, "wallet")
		}
		walletReadOnly = true
	}

	indexStore, err := indexer.NewReadOnlyStoreAt(filepath.Join(walletStore.RootDir(), "index.db"))
	if err != nil {
		_ = walletStore.Close()
		return nil, friendlyLockError(err, "index")
	}

	return &Handles{
		Wallet:         walletStore,
		Index:          indexStore,
		WalletReadOnly: walletReadOnly,
	}, nil
}

// Close releases all opened stores.
func (h *Handles) Close() error {
	if h == nil {
		return nil
	}
	var errs []string
	if h.Index != nil {
		if err := h.Index.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if h.Wallet != nil {
		if err := h.Wallet.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// IsLockError reports whether an error is a Badger directory lock conflict.
func IsLockError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "cannot acquire directory lock")
}

func friendlyLockError(err error, target string) error {
	if IsLockError(err) {
		return fmt.Errorf("%s database is in use by another process. Close the CLI or run the TUI in read-only mode", target)
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("cannot access %s database: %w", target, err)
	}
	return err
}
