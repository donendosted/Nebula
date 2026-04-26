package wallet

import "time"

const (
	AssetCodeXLM                       = "XLM"
	DefaultHistoryLimit                = 10
	MinimumAccountReserveStroops int64 = 10_000_000
)

type Network string

const (
	NetworkTestnet Network = "testnet"
	NetworkMainnet Network = "mainnet"
)

type Wallet struct {
	Address string
	Secret  string
}

type Balance struct {
	AssetCode string
	Amount    string
}

type AccountInfo struct {
	Address     string
	Network     Network
	Balance     []Balance
	Funded      bool
	Reserve     string
	LastUpdated time.Time
}

type HistoryEntry struct {
	Hash         string
	Type         string
	Direction    string
	Amount       string
	AssetCode    string
	Counterparty string
	CreatedAt    time.Time
	Successful   bool
}

type SendResult struct {
	Hash      string
	Amount    string
	AssetCode string
}
