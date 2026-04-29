package indexer

import "time"

const (
	maxSyncLimit    = 200
	txKeyPrefix     = "tx:"
	acctKeyPrefix   = "acct:"
	timeKeyPrefix   = "time:"
	metaKeyPrefix   = "meta:"
	defaultDuration = 24 * time.Hour
)

// Record is the normalized local transaction shape stored by Nebula.
type Record struct {
	Account      string    `json:"account"`
	Hash         string    `json:"hash"`
	Timestamp    time.Time `json:"timestamp"`
	Amount       string    `json:"amount"`
	Direction    string    `json:"direction"`
	Counterparty string    `json:"counterparty"`
	Type         string    `json:"type"`
	AssetCode    string    `json:"asset_code"`
	ExplorerURL  string    `json:"explorer_url"`
	Successful   bool      `json:"successful"`
	LatencyMS    int64     `json:"latency_ms,omitempty"`
}

// Stats summarizes the local cache for a query scope.
type Stats struct {
	TotalTransactions int     `json:"total_transactions"`
	AverageLatencyMS  float64 `json:"average_latency_ms"`
	TotalVolumeSent   float64 `json:"total_volume_sent"`
	TotalVolumeRecv   float64 `json:"total_volume_received"`
}
