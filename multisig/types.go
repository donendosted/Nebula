package multisig

import "time"

// ThresholdConfig captures the source account threshold state after a change.
type ThresholdConfig struct {
	MasterWeight uint8 `json:"master_weight"`
	Low          uint8 `json:"low"`
	Medium       uint8 `json:"medium"`
	High         uint8 `json:"high"`
}

// Proposal stores a multisig signing artifact that can be passed between parties.
type Proposal struct {
	ID                string          `json:"id"`
	Kind              string          `json:"kind"`
	WalletID          string          `json:"wallet_id"`
	AccountIndex      uint32          `json:"account_index"`
	SourceAddress     string          `json:"source_address"`
	Network           string          `json:"network"`
	Sequence          int64           `json:"sequence"`
	RequiredThreshold uint8           `json:"required_threshold"`
	XDR               string          `json:"xdr"`
	Signers           []ProposalSig   `json:"signers"`
	CreatedAt         time.Time       `json:"created_at"`
	SubmittedHash     string          `json:"submitted_hash,omitempty"`
	ThresholdSnapshot ThresholdConfig `json:"threshold_snapshot"`
}

// ProposalSig records one signature attachment event.
type ProposalSig struct {
	Address  string    `json:"address"`
	SignedAt time.Time `json:"signed_at"`
}
