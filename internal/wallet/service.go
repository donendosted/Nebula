package wallet

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	hProtocol "github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

type Service struct {
	storage *Storage
	verbose bool
}

type ServiceOptions struct {
	Verbose bool
}

func NewService(opts ServiceOptions) (*Service, error) {
	storage, err := NewStorage()
	if err != nil {
		return nil, err
	}

	return &Service{
		storage: storage,
		verbose: opts.Verbose,
	}, nil
}

func (s *Service) StorageDir() string {
	return s.storage.BaseDir()
}

func (s *Service) CreateWallet() (Wallet, error) {
	full, err := keypair.Random()
	if err != nil {
		return Wallet{}, fmt.Errorf("generate keypair: %w", err)
	}

	return s.storage.SaveWallet(full.Seed())
}

func (s *Service) ImportWallet(secret string) (Wallet, error) {
	return s.storage.SaveWallet(strings.TrimSpace(secret))
}

func (s *Service) Wallet() (Wallet, error) {
	return s.storage.LoadWallet()
}

func (s *Service) Address() (string, error) {
	w, err := s.Wallet()
	if err != nil {
		return "", err
	}
	return w.Address, nil
}

func (s *Service) ListWallets() ([]Wallet, error) {
	return s.storage.ListWallets()
}

func (s *Service) SwitchWallet(address string) (Wallet, error) {
	return s.storage.SwitchWallet(address)
}

func (s *Service) CurrentNetwork(override string) (Network, error) {
	if strings.TrimSpace(override) != "" {
		networkValue := Network(strings.ToLower(strings.TrimSpace(override)))
		if !networkValue.Valid() {
			return "", ErrUnsupportedNetwork
		}
		return networkValue, nil
	}

	return s.storage.LoadNetwork()
}

func (s *Service) SetNetwork(networkValue Network) error {
	return s.storage.SaveNetwork(networkValue)
}

func (s *Service) ToggleNetwork() (Network, error) {
	current, err := s.CurrentNetwork("")
	if err != nil {
		return "", err
	}

	if current == NetworkTestnet {
		return NetworkMainnet, s.storage.SaveNetwork(NetworkMainnet)
	}

	return NetworkTestnet, s.storage.SaveNetwork(NetworkTestnet)
}

func (s *Service) AccountInfo(networkValue Network) (AccountInfo, error) {
	w, err := s.Wallet()
	if err != nil {
		return AccountInfo{}, err
	}

	client := horizonClient(networkValue)
	account, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: w.Address})
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return AccountInfo{
				Address:     w.Address,
				Network:     networkValue,
				Funded:      false,
				Reserve:     FormatStroops(MinimumAccountReserveStroops),
				LastUpdated: time.Now(),
			}, nil
		}
		return AccountInfo{}, fmt.Errorf("fetch account: %w", err)
	}

	nativeBalance, err := account.GetNativeBalance()
	if err != nil {
		return AccountInfo{}, fmt.Errorf("read native balance: %w", err)
	}

	return AccountInfo{
		Address: w.Address,
		Network: networkValue,
		Funded:  true,
		Balance: []Balance{{
			AssetCode: AssetCodeXLM,
			Amount:    nativeBalance,
		}},
		Reserve:     FormatStroops(MinimumAccountReserveStroops),
		LastUpdated: time.Now(),
	}, nil
}

func (s *Service) SendXLM(networkValue Network, destination, amount, memo string) (SendResult, error) {
	w, err := s.Wallet()
	if err != nil {
		return SendResult{}, err
	}

	destination = strings.TrimSpace(destination)
	if err := ValidateAddress(destination); err != nil {
		return SendResult{}, err
	}

	sendStroops, err := ParseAmountToStroops(amount)
	if err != nil {
		return SendResult{}, err
	}

	client := horizonClient(networkValue)
	sourceAccount, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: w.Address})
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return SendResult{}, ErrAccountNotFunded
		}
		return SendResult{}, fmt.Errorf("load source account: %w", err)
	}

	nativeBalance, err := sourceAccount.GetNativeBalance()
	if err != nil {
		return SendResult{}, fmt.Errorf("read native balance: %w", err)
	}

	balanceStroops, err := ParseAmountToStroops(nativeBalance)
	if err != nil {
		return SendResult{}, fmt.Errorf("parse current balance: %w", err)
	}

	if sendStroops > balanceStroops-MinimumAccountReserveStroops {
		return SendResult{}, fmt.Errorf("%w: spendable=%s", ErrInsufficientBalance, FormatStroops(maxInt64(0, balanceStroops-MinimumAccountReserveStroops)))
	}

	op := &txnbuild.Payment{
		Destination: destination,
		Amount:      FormatStroops(sendStroops),
		Asset:       txnbuild.NativeAsset{},
	}

	params := txnbuild.TransactionParams{
		SourceAccount:        &sourceAccount,
		IncrementSequenceNum: true,
		Operations:           []txnbuild.Operation{op},
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
	}

	if memo = strings.TrimSpace(memo); memo != "" {
		params.Memo = txnbuild.MemoText(memo)
	}

	tx, err := txnbuild.NewTransaction(params)
	if err != nil {
		return SendResult{}, fmt.Errorf("build transaction: %w", err)
	}

	full, err := ParseSecret(w.Secret)
	if err != nil {
		return SendResult{}, err
	}

	tx, err = tx.Sign(networkPassphrase(networkValue), full)
	if err != nil {
		return SendResult{}, fmt.Errorf("sign transaction: %w", err)
	}

	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return SendResult{}, formatHorizonError("submit transaction", err)
	}

	return SendResult{
		Hash:      resp.Hash,
		Amount:    FormatStroops(sendStroops),
		AssetCode: AssetCodeXLM,
	}, nil
}

func (s *Service) History(networkValue Network, limit int) ([]HistoryEntry, error) {
	w, err := s.Wallet()
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = DefaultHistoryLimit
	}

	client := horizonClient(networkValue)
	page, err := client.Payments(horizonclient.OperationRequest{
		ForAccount: w.Address,
		Limit:      uint(limit),
		Order:      horizonclient.OrderDesc,
	})
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return nil, ErrAccountNotFunded
		}
		return nil, fmt.Errorf("fetch history: %w", err)
	}

	entries := make([]HistoryEntry, 0, len(page.Embedded.Records))
	for _, record := range page.Embedded.Records {
		entry, ok := historyEntryFromOperation(w.Address, record)
		if ok {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func (s *Service) Fund(networkValue Network) (string, error) {
	if networkValue != NetworkTestnet {
		return "", ErrMainnetFriendbot
	}

	w, err := s.Wallet()
	if err != nil {
		return "", err
	}

	tx, err := horizonclient.DefaultTestNetClient.Fund(w.Address)
	if err != nil {
		return "", formatHorizonError("friendbot request", err)
	}

	return tx.Hash, nil
}

func (n Network) Valid() bool {
	return n == NetworkTestnet || n == NetworkMainnet
}

func ParseSecret(secret string) (*keypair.Full, error) {
	full, err := keypair.ParseFull(strings.TrimSpace(secret))
	if err != nil {
		return nil, ErrInvalidSecret
	}
	return full, nil
}

func ValidateAddress(address string) error {
	if _, err := keypair.ParseAddress(strings.TrimSpace(address)); err != nil {
		return ErrInvalidAddress
	}
	return nil
}

func horizonClient(networkValue Network) *horizonclient.Client {
	if networkValue == NetworkMainnet {
		return horizonclient.DefaultPublicNetClient
	}
	return horizonclient.DefaultTestNetClient
}

func networkPassphrase(networkValue Network) string {
	if networkValue == NetworkMainnet {
		return network.PublicNetworkPassphrase
	}
	return network.TestNetworkPassphrase
}

func historyEntryFromOperation(address string, record operations.Operation) (HistoryEntry, bool) {
	switch op := record.(type) {
	case operations.Payment:
		if op.Asset.Type != "native" {
			return HistoryEntry{}, false
		}
		direction := "out"
		counterparty := op.To
		if op.To == address {
			direction = "in"
			counterparty = op.From
		}
		return HistoryEntry{
			Hash:         op.TransactionHash,
			Type:         op.Base.Type,
			Direction:    direction,
			Amount:       op.Amount,
			AssetCode:    AssetCodeXLM,
			Counterparty: counterparty,
			CreatedAt:    op.LedgerCloseTime,
			Successful:   op.TransactionSuccessful,
		}, true
	case operations.CreateAccount:
		direction := "out"
		counterparty := op.Account
		amount := op.StartingBalance
		if op.Account == address {
			direction = "in"
			counterparty = op.Funder
		}
		return HistoryEntry{
			Hash:         op.TransactionHash,
			Type:         op.Type,
			Direction:    direction,
			Amount:       amount,
			AssetCode:    AssetCodeXLM,
			Counterparty: counterparty,
			CreatedAt:    op.LedgerCloseTime,
			Successful:   op.TransactionSuccessful,
		}, true
	default:
		return HistoryEntry{}, false
	}
}

func formatHorizonError(prefix string, err error) error {
	var hErr *horizonclient.Error
	if errors.As(err, &hErr) {
		if result, resultErr := hErr.ResultString(); resultErr == nil && result != "" {
			return fmt.Errorf("%s: %s", prefix, result)
		}
		return fmt.Errorf("%s: %s", prefix, hErr.Error())
	}

	return fmt.Errorf("%s: %w", prefix, err)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func NativeBalanceFromInfo(info AccountInfo) string {
	for _, balance := range info.Balance {
		if balance.AssetCode == AssetCodeXLM {
			return balance.Amount
		}
	}
	return "0.0000000"
}

func ProtocolAccountToBalances(account hProtocol.Account) ([]Balance, error) {
	nativeBalance, err := account.GetNativeBalance()
	if err != nil {
		return nil, err
	}
	return []Balance{{AssetCode: AssetCodeXLM, Amount: nativeBalance}}, nil
}
