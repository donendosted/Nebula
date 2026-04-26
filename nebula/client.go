package nebula

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

// Client is a reusable Stellar wallet client.
type Client struct {
	secret  string
	address string
	network Network
	horizon *horizonclient.Client
}

// NewClient creates a reusable wallet client from a Stellar seed and network.
func NewClient(secret string, networkValue Network) (*Client, error) {
	full, err := parseSecret(secret)
	if err != nil {
		return nil, err
	}
	if !networkValue.Valid() {
		return nil, ErrUnsupportedNetwork
	}
	return &Client{
		secret:  full.Seed(),
		address: full.Address(),
		network: networkValue,
		horizon: horizonClient(networkValue),
	}, nil
}

// Address returns the wallet public key.
func (c *Client) Address() string {
	return c.address
}

// Balance returns the current native XLM account state.
func (c *Client) Balance() (AccountInfo, error) {
	account, err := withRateLimitRetry(func() (hProtocol.Account, error) {
		return c.horizon.AccountDetail(horizonclient.AccountRequest{AccountID: c.address})
	})
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return AccountInfo{
				Address:     c.address,
				Network:     c.network,
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
		Address:     c.address,
		Network:     c.network,
		Funded:      true,
		Balance:     []Balance{{AssetCode: AssetCodeXLM, Amount: nativeBalance}},
		Reserve:     FormatStroops(MinimumAccountReserveStroops),
		LastUpdated: time.Now(),
	}, nil
}

// Send submits a native XLM payment.
func (c *Client) Send(destination, amount, memo string) (SendResult, error) {
	destination = strings.TrimSpace(destination)
	if err := ValidateAddress(destination); err != nil {
		return SendResult{}, err
	}
	sendStroops, err := ParseAmountToStroops(amount)
	if err != nil {
		return SendResult{}, err
	}

	sourceAccount, err := withRateLimitRetry(func() (hProtocol.Account, error) {
		return c.horizon.AccountDetail(horizonclient.AccountRequest{AccountID: c.address})
	})
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
	spendable := balanceStroops - MinimumAccountReserveStroops
	if sendStroops > spendable {
		return SendResult{}, fmt.Errorf("%w: spendable=%s", ErrInsufficientBalance, FormatStroops(maxInt64(0, spendable)))
	}

	tx, err := c.buildPayment(sourceAccount, destination, sendStroops, memo)
	if err != nil {
		return SendResult{}, err
	}
	resp, err := withRateLimitRetry(func() (hProtocol.Transaction, error) {
		return c.horizon.SubmitTransaction(tx)
	})
	if err != nil {
		return SendResult{}, formatHorizonError("submit transaction", err)
	}
	return SendResult{Hash: resp.Hash, Amount: FormatStroops(sendStroops), AssetCode: AssetCodeXLM}, nil
}

// History returns recent native payment activity.
func (c *Client) History(limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = DefaultHistoryLimit
	}
	if limit > MaximumHistoryLimit {
		limit = MaximumHistoryLimit
	}
	page, err := withRateLimitRetry(func() (operationsPage, error) {
		return c.horizon.Payments(horizonclient.OperationRequest{
			ForAccount: c.address,
			Limit:      uint(limit),
			Order:      horizonclient.OrderDesc,
		})
	})
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return nil, ErrAccountNotFunded
		}
		return nil, fmt.Errorf("fetch history: %w", err)
	}
	entries := make([]HistoryEntry, 0, len(page.Embedded.Records))
	for _, record := range page.Embedded.Records {
		if item, ok := historyEntryFromOperation(c.network, c.address, record); ok {
			entries = append(entries, item)
		}
	}
	return entries, nil
}

// FundTestnet requests Friendbot funding for the wallet.
func (c *Client) FundTestnet() (string, error) {
	if c.network != NetworkTestnet {
		return "", ErrMainnetFriendbot
	}
	hash, err := withRateLimitRetry(func() (string, error) {
		tx, fundErr := horizonclient.DefaultTestNetClient.Fund(c.address)
		if fundErr != nil {
			return "", fundErr
		}
		return tx.Hash, nil
	})
	if err != nil {
		if isFriendbotLimit(err) {
			return "", ErrFriendbotLimit
		}
		return "", formatHorizonError("friendbot request", err)
	}
	return hash, nil
}

type operationsPage = operations.OperationsPage

func (c *Client) buildPayment(sourceAccount hProtocol.Account, destination string, amount int64, memo string) (*txnbuild.Transaction, error) {
	op := &txnbuild.Payment{
		Destination: destination,
		Amount:      FormatStroops(amount),
		Asset:       txnbuild.NativeAsset{},
	}
	params := txnbuild.TransactionParams{
		SourceAccount:        &sourceAccount,
		IncrementSequenceNum: true,
		Operations:           []txnbuild.Operation{op},
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
	}
	if memo = strings.TrimSpace(memo); memo != "" {
		params.Memo = txnbuild.MemoText(memo)
	}
	tx, err := txnbuild.NewTransaction(params)
	if err != nil {
		return nil, fmt.Errorf("build transaction: %w", err)
	}
	full, err := parseSecret(c.secret)
	if err != nil {
		return nil, err
	}
	tx, err = tx.Sign(networkPassphrase(c.network), full)
	if err != nil {
		return nil, fmt.Errorf("sign transaction: %w", err)
	}
	return tx, nil
}

func withRateLimitRetry[T any](fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		value, err := fn()
		if err == nil {
			return value, nil
		}
		lastErr = err
		if !isRateLimitError(err) || attempt == 2 {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
	}
	return zero, lastErr
}

func parseSecret(secret string) (*keypair.Full, error) {
	full, err := keypair.ParseFull(strings.TrimSpace(secret))
	if err != nil {
		return nil, ErrInvalidSecret
	}
	return full, nil
}

// ValidateAddress rejects malformed Stellar public keys.
func ValidateAddress(address string) error {
	if _, err := keypair.ParseAddress(strings.TrimSpace(address)); err != nil {
		return ErrInvalidAddress
	}
	return nil
}

func (n Network) Valid() bool {
	return n == NetworkTestnet || n == NetworkMainnet
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

func historyEntryFromOperation(networkValue Network, address string, record operations.Operation) (HistoryEntry, bool) {
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
			ExplorerURL:  txExplorerURL(networkValue, op.TransactionHash),
		}, true
	case operations.CreateAccount:
		direction := "out"
		counterparty := op.Account
		if op.Account == address {
			direction = "in"
			counterparty = op.Funder
		}
		return HistoryEntry{
			Hash:         op.TransactionHash,
			Type:         op.Type,
			Direction:    direction,
			Amount:       op.StartingBalance,
			AssetCode:    AssetCodeXLM,
			Counterparty: counterparty,
			CreatedAt:    op.LedgerCloseTime,
			Successful:   op.TransactionSuccessful,
			ExplorerURL:  txExplorerURL(networkValue, op.TransactionHash),
		}, true
	default:
		return HistoryEntry{}, false
	}
}

func txExplorerURL(networkValue Network, hash string) string {
	explorerNetwork := "testnet"
	if networkValue == NetworkMainnet {
		explorerNetwork = "public"
	}
	return fmt.Sprintf("https://stellar.expert/explorer/%s/tx/%s", explorerNetwork, hash)
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

func isRateLimitError(err error) bool {
	var hErr *horizonclient.Error
	if errors.As(err, &hErr) && hErr.Problem.Status == 429 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "too many requests")
}

func isFriendbotLimit(err error) bool {
	if isRateLimitError(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "limit")
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
