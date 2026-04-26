package nebula

import "time"

const (
	// AssetCodeXLM is the native Stellar asset code.
	AssetCodeXLM = "XLM"
	// DefaultHistoryLimit is the default recent history size.
	DefaultHistoryLimit = 10
	// MaximumHistoryLimit is the highest supported history limit.
	MaximumHistoryLimit = 20
	// MinimumAccountReserveStroops is the minimum reserve kept in every account.
	MinimumAccountReserveStroops int64 = 10_000_000
)

// Network identifies the Stellar network to use.
type Network string

const (
	// NetworkTestnet is the Stellar test network.
	NetworkTestnet Network = "testnet"
	// NetworkMainnet is the Stellar public network.
	NetworkMainnet Network = "mainnet"
)

// WalletMeta describes a stored wallet without exposing its secret.
type WalletMeta struct {
	Name               string    `json:"name"`
	Address            string    `json:"address"`
	SecretPath         string    `json:"-"`
	CreatedAt          time.Time `json:"created_at"`
	Active             bool      `json:"-"`
	TestnetFundingUsed int       `json:"testnet_funding_used"`
}

// Balance describes a wallet asset balance.
type Balance struct {
	AssetCode string
	Amount    string
}

// AccountInfo is the current wallet state on a specific network.
type AccountInfo struct {
	Name        string
	Address     string
	Network     Network
	Balance     []Balance
	Funded      bool
	Reserve     string
	LastUpdated time.Time
}

// HistoryEntry is a recent transaction-like event.
type HistoryEntry struct {
	Hash         string
	Type         string
	Direction    string
	Amount       string
	AssetCode    string
	Counterparty string
	CreatedAt    time.Time
	Successful   bool
	ExplorerURL  string
}

// SendResult is a submitted payment result.
type SendResult struct {
	Hash      string
	Amount    string
	AssetCode string
}

// FundResult captures Friendbot status.
type FundResult struct {
	Hash         string
	FundedCount  int
	LimitReached bool
}

// UnlockedWallet contains decrypted wallet material for command and UI sessions.
type UnlockedWallet struct {
	Meta       WalletMeta
	Secret     string
	Passphrase string
}

// Client creates a Stellar API client for the unlocked wallet on the selected network.
func (u UnlockedWallet) Client(networkValue Network) (*Client, error) {
	return NewClient(u.Secret, networkValue)
}
