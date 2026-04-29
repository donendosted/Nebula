package wallet

import "time"

const (
	defaultWords            = 24
	defaultAccountIndex     = uint32(0)
	configBucketKey         = "config"
	walletBucketPrefix      = "wallet:"
	derivedAccountKeyPrefix = "derived:"
	defaultRootDirName      = ".nebula"
	defaultWalletDBDirName  = "wallet.db"
	defaultProposalDirName  = "proposals"
	defaultIndexDBDirName   = "index.db"
)

// Config stores active wallet state.
type Config struct {
	ActiveWalletID     string `json:"active_wallet_id"`
	ActiveAccountIndex uint32 `json:"active_account_index"`
	Network            string `json:"network"`
}

// WalletRecord stores encrypted mnemonic material and metadata for a wallet root.
type WalletRecord struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	MnemonicCipher    encryptedBlob `json:"mnemonic_cipher"`
	MnemonicWordCount int           `json:"mnemonic_word_count"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

// DerivedAccount is a derived Stellar account under a wallet root.
type DerivedAccount struct {
	WalletID  string    `json:"wallet_id"`
	Index     uint32    `json:"index"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

// WalletSummary is the public listing shape returned to the CLI.
type WalletSummary struct {
	WalletRecord
	Accounts           []DerivedAccount `json:"accounts"`
	Active             bool             `json:"active"`
	ActiveAccountIndex uint32           `json:"active_account_index"`
}

// CreateOptions describes wallet creation from fresh entropy.
type CreateOptions struct {
	Name       string
	Passphrase string
	Words      int
}

// ImportOptions describes wallet creation from an existing mnemonic.
type ImportOptions struct {
	Name       string
	Mnemonic   string
	Passphrase string
}

// SensitiveAction is a CLI-facing guard for irreversible operations.
type SensitiveAction struct {
	Reason string
}
