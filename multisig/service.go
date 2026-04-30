package multisig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nebula/internal/metrics"
	"nebula/stellar"
	"nebula/wallet"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

// Service manages Stellar signer/threshold operations and proposal files.
type Service struct {
	wallets *wallet.Store
}

// NewService constructs a multisig service on top of the encrypted wallet store.
func NewService(wallets *wallet.Store) *Service {
	return &Service{wallets: wallets}
}

// AddSigner adds or updates a signer on the active account.
func (s *Service) AddSigner(secret, networkName, signerAddress string, weight uint8, confirm bool) (string, error) {
	if err := wallet.Confirm(confirm, wallet.SensitiveAction{Reason: "add or update signer"}); err != nil {
		return "", err
	}
	client, account, tx, err := s.setSignerTx(secret, networkName, signerAddress, weight)
	if err != nil {
		return "", err
	}
	full, err := parseFull(secret)
	if err != nil {
		return "", err
	}
	if err := validateSafety(account, signerAddress, weight, nil); err != nil {
		return "", err
	}
	signed, err := tx.Sign(client.Passphrase(), full)
	if err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}
	resp, err := client.SubmitTransaction(signed)
	if err != nil {
		return "", fmt.Errorf("submit transaction: %w", err)
	}
	return resp.Hash, nil
}

// RemoveSigner removes a signer by setting its weight to zero.
func (s *Service) RemoveSigner(secret, networkName, signerAddress string, confirm bool) (string, error) {
	if err := wallet.Confirm(confirm, wallet.SensitiveAction{Reason: "remove signer"}); err != nil {
		return "", err
	}
	return s.AddSigner(secret, networkName, signerAddress, 0, true)
}

// SetThresholds updates thresholds and optional master weight on the active account.
func (s *Service) SetThresholds(secret, networkName string, cfg ThresholdConfig, confirm bool) (string, error) {
	if err := wallet.Confirm(confirm, wallet.SensitiveAction{Reason: "change account thresholds"}); err != nil {
		return "", err
	}
	client, err := stellar.NewClient(networkName)
	if err != nil {
		return "", err
	}
	full, err := parseFull(secret)
	if err != nil {
		return "", err
	}
	account, err := client.Account(full.Address())
	if err != nil {
		return "", fmt.Errorf("reload account: %w", err)
	}
	if err := validateThresholdOrdering(cfg); err != nil {
		return "", err
	}
	if err := validateSafety(account, "", 255, &cfg); err != nil {
		return "", err
	}
	op := txnbuild.SetOptions{
		MasterWeight:    txnbuild.NewThreshold(txnbuild.Threshold(cfg.MasterWeight)),
		LowThreshold:    txnbuild.NewThreshold(txnbuild.Threshold(cfg.Low)),
		MediumThreshold: txnbuild.NewThreshold(txnbuild.Threshold(cfg.Medium)),
		HighThreshold:   txnbuild.NewThreshold(txnbuild.Threshold(cfg.High)),
	}
	tx, err := client.SetOptionsTx(account, op)
	if err != nil {
		return "", err
	}
	signed, err := tx.Sign(client.Passphrase(), full)
	if err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}
	resp, err := client.SubmitTransaction(signed)
	if err != nil {
		return "", fmt.Errorf("submit transaction: %w", err)
	}
	return resp.Hash, nil
}

// ProposePayment creates an unsigned XDR proposal for multi-party signing.
func (s *Service) ProposePayment(secret, networkName, walletID string, accountIndex uint32, destination, amount, memo string) (Proposal, error) {
	client, err := stellar.NewClient(networkName)
	if err != nil {
		return Proposal{}, err
	}
	full, err := parseFull(secret)
	if err != nil {
		return Proposal{}, err
	}
	account, err := client.Account(full.Address())
	if err != nil {
		return Proposal{}, fmt.Errorf("reload account: %w", err)
	}
	tx, err := client.PaymentTx(account, destination, amount, memo)
	if err != nil {
		return Proposal{}, err
	}
	xdr, err := tx.Base64()
	if err != nil {
		return Proposal{}, fmt.Errorf("encode proposal xdr: %w", err)
	}
	proposal := Proposal{
		ID:            fmt.Sprintf("%s-%d-%d", walletID, accountIndex, time.Now().Unix()),
		Kind:          "payment",
		WalletID:      walletID,
		AccountIndex:  accountIndex,
		SourceAddress: full.Address(),
		Network:       networkName,
		Sequence:      tx.SequenceNumber(),
		XDR:           xdr,
		CreatedAt:     time.Now().UTC(),
		ThresholdSnapshot: ThresholdConfig{
			Low:          account.Thresholds.LowThreshold,
			Medium:       account.Thresholds.MedThreshold,
			High:         account.Thresholds.HighThreshold,
			MasterWeight: masterWeight(account),
		},
		RequiredThreshold: account.Thresholds.MedThreshold,
	}
	if err := s.saveProposal(proposal); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

// SignProposal adds the local wallet signature to a stored proposal file.
func (s *Service) SignProposal(secret string, proposalID string) (Proposal, error) {
	proposal, err := s.LoadProposal(proposalID)
	if err != nil {
		return Proposal{}, err
	}
	client, err := stellar.NewClient(proposal.Network)
	if err != nil {
		return Proposal{}, err
	}
	xdr, signerAddress, err := client.SignXDR(proposal.XDR, secret)
	if err != nil {
		return Proposal{}, err
	}
	proposal.XDR = xdr
	if !proposalHasSigner(proposal, signerAddress) {
		proposal.Signers = append(proposal.Signers, ProposalSig{Address: signerAddress, SignedAt: time.Now().UTC()})
	}
	metrics.RecordWalletAction("sign", proposal.ID)
	if err := s.saveProposal(proposal); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

// SubmitProposal submits a signed proposal if its sequence is still current.
func (s *Service) SubmitProposal(proposalID string) (string, error) {
	proposal, err := s.LoadProposal(proposalID)
	if err != nil {
		return "", err
	}
	client, err := stellar.NewClient(proposal.Network)
	if err != nil {
		return "", err
	}
	account, err := client.Account(proposal.SourceAddress)
	if err != nil {
		return "", fmt.Errorf("reload account: %w", err)
	}
	if account.Sequence >= proposal.Sequence {
		return "", fmt.Errorf("proposal sequence is stale; repropose transaction")
	}
	resp, err := client.SubmitEnvelopeXDR(proposal.XDR)
	if err != nil {
		return "", fmt.Errorf("submit transaction: %w", err)
	}
	proposal.SubmittedHash = resp.Hash
	if err := s.saveProposal(proposal); err != nil {
		return "", err
	}
	return resp.Hash, nil
}

// LoadProposal loads a stored proposal by id.
func (s *Service) LoadProposal(id string) (Proposal, error) {
	path := filepath.Join(s.wallets.ProposalDir(), strings.TrimSpace(id)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Proposal{}, err
	}
	var proposal Proposal
	if err := json.Unmarshal(data, &proposal); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func (s *Service) saveProposal(proposal Proposal) error {
	if err := os.MkdirAll(s.wallets.ProposalDir(), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.wallets.ProposalDir(), proposal.ID+".json"), payload, 0o600)
}

func (s *Service) setSignerTx(secret, networkName, signerAddress string, weight uint8) (*stellar.Client, horizon.Account, *txnbuild.Transaction, error) {
	client, err := stellar.NewClient(networkName)
	if err != nil {
		return nil, horizon.Account{}, nil, err
	}
	full, err := parseFull(secret)
	if err != nil {
		return nil, horizon.Account{}, nil, err
	}
	account, err := client.Account(full.Address())
	if err != nil {
		return nil, horizon.Account{}, nil, fmt.Errorf("reload account: %w", err)
	}
	op := txnbuild.SetOptions{
		Signer: &txnbuild.Signer{
			Address: strings.TrimSpace(signerAddress),
			Weight:  txnbuild.Threshold(weight),
		},
	}
	tx, err := client.SetOptionsTx(account, op)
	if err != nil {
		return nil, horizon.Account{}, nil, err
	}
	return client, account, tx, nil
}

func validateSafety(account horizon.Account, signerAddress string, signerWeight uint8, cfg *ThresholdConfig) error {
	resultingSigners := map[string]int32{}
	for _, signer := range account.Signers {
		resultingSigners[signer.Key] = signer.Weight
	}
	if strings.TrimSpace(signerAddress) != "" {
		resultingSigners[strings.TrimSpace(signerAddress)] = int32(signerWeight)
	}
	current := ThresholdConfig{
		Low:          account.Thresholds.LowThreshold,
		Medium:       account.Thresholds.MedThreshold,
		High:         account.Thresholds.HighThreshold,
		MasterWeight: masterWeight(account),
	}
	if cfg != nil {
		current = *cfg
	}
	if err := validateThresholdOrdering(current); err != nil {
		return err
	}
	total := int32(0)
	for _, weight := range resultingSigners {
		if weight > 0 {
			total += weight
		}
	}
	if total == 0 {
		return fmt.Errorf("unsafe signer change: resulting account would have no active signer weight")
	}
	if total < int32(current.High) {
		return fmt.Errorf("unsafe signer change: total signer weight %d is below high threshold %d", total, current.High)
	}
	return nil
}

func validateThresholdOrdering(cfg ThresholdConfig) error {
	if cfg.Low > cfg.Medium || cfg.Medium > cfg.High {
		return fmt.Errorf("invalid thresholds: require low <= medium <= high")
	}
	if cfg.High == 0 {
		return fmt.Errorf("invalid thresholds: high threshold must be > 0")
	}
	return nil
}

func masterWeight(account horizon.Account) uint8 {
	for _, signer := range account.Signers {
		if signer.Key == account.AccountID {
			return uint8(signer.Weight)
		}
	}
	return 0
}

func proposalHasSigner(proposal Proposal, address string) bool {
	for _, signer := range proposal.Signers {
		if signer.Address == address {
			return true
		}
	}
	return false
}

func parseFull(secret string) (*keypair.Full, error) {
	full, err := keypair.ParseFull(strings.TrimSpace(secret))
	if err != nil {
		return nil, fmt.Errorf("invalid Stellar secret key")
	}
	return full, nil
}
