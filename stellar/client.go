package stellar

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	hProtocol "github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

const (
	// NetworkTestnet is the Stellar test network identifier used by Nebula.
	NetworkTestnet = "testnet"
	// NetworkMainnet is the Stellar public network identifier used by Nebula.
	NetworkMainnet                     = "mainnet"
	minimumAccountReserveStroops int64 = 10_000_000
)

// Client wraps Horizon access and transaction submission for Nebula modules.
type Client struct {
	network    string
	passphrase string
	horizon    *horizonclient.Client
}

// NewClient constructs a network client for testnet or mainnet.
func NewClient(networkName string) (*Client, error) {
	networkName = strings.ToLower(strings.TrimSpace(networkName))
	switch networkName {
	case NetworkMainnet:
		return &Client{network: networkName, passphrase: network.PublicNetworkPassphrase, horizon: horizonclient.DefaultPublicNetClient}, nil
	case "", NetworkTestnet:
		return &Client{network: NetworkTestnet, passphrase: network.TestNetworkPassphrase, horizon: horizonclient.DefaultTestNetClient}, nil
	default:
		return nil, fmt.Errorf("unsupported network: %s", networkName)
	}
}

// Network returns the selected Stellar network name.
func (c *Client) Network() string {
	return c.network
}

// Passphrase returns the selected network passphrase.
func (c *Client) Passphrase() string {
	return c.passphrase
}

// Account reloads an account from Horizon.
func (c *Client) Account(address string) (hProtocol.Account, error) {
	return withRateLimitRetry(func() (hProtocol.Account, error) {
		return c.horizon.AccountDetail(horizonclient.AccountRequest{AccountID: strings.TrimSpace(address)})
	})
}

// Payments returns recent payment-like operations for an account.
func (c *Client) Payments(address string, limit int) (operations.OperationsPage, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	return withRateLimitRetry(func() (operations.OperationsPage, error) {
		return c.horizon.Payments(horizonclient.OperationRequest{
			ForAccount: strings.TrimSpace(address),
			Limit:      uint(limit),
			Order:      horizonclient.OrderDesc,
		})
	})
}

// SubmitTransaction submits a signed transaction.
func (c *Client) SubmitTransaction(tx *txnbuild.Transaction) (hProtocol.Transaction, error) {
	return withRateLimitRetry(func() (hProtocol.Transaction, error) {
		return c.horizon.SubmitTransaction(tx)
	})
}

// SubmitEnvelopeXDR submits a signed base64 XDR envelope.
func (c *Client) SubmitEnvelopeXDR(txe string) (hProtocol.Transaction, error) {
	gtx, err := txnbuild.TransactionFromXDR(txe)
	if err != nil {
		return hProtocol.Transaction{}, fmt.Errorf("parse transaction xdr: %w", err)
	}
	tx, ok := gtx.Transaction()
	if !ok {
		return hProtocol.Transaction{}, fmt.Errorf("fee bump transactions are not supported")
	}
	return c.SubmitTransaction(tx)
}

// Balance loads the account balance and reports whether the account exists on-chain.
func (c *Client) Balance(address string) (string, bool, error) {
	account, err := c.Account(address)
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return FormatStroops(minimumAccountReserveStroops), false, nil
		}
		return "", false, err
	}
	balance, err := account.GetNativeBalance()
	if err != nil {
		return "", false, fmt.Errorf("read native balance: %w", err)
	}
	return balance, true, nil
}

// SendPayment signs and submits a reserve-aware native XLM payment.
func (c *Client) SendPayment(secret, destination, amount, memo string) (string, error) {
	if err := ValidateAddress(destination); err != nil {
		return "", err
	}
	sendStroops, err := ParseAmountToStroops(amount)
	if err != nil {
		return "", err
	}
	full, err := parseFull(secret)
	if err != nil {
		return "", err
	}
	account, err := c.Account(full.Address())
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			return "", ErrAccountNotFunded
		}
		return "", fmt.Errorf("reload account: %w", err)
	}
	nativeBalance, err := account.GetNativeBalance()
	if err != nil {
		return "", fmt.Errorf("read native balance: %w", err)
	}
	balanceStroops, err := ParseAmountToStroops(nativeBalance)
	if err != nil {
		return "", fmt.Errorf("parse balance: %w", err)
	}
	if sendStroops > balanceStroops-minimumAccountReserveStroops {
		return "", ErrInsufficientBalance
	}
	tx, err := c.PaymentTx(account, destination, amount, memo)
	if err != nil {
		return "", err
	}
	signed, err := tx.Sign(c.passphrase, full)
	if err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}
	resp, err := c.SubmitTransaction(signed)
	if err != nil {
		return "", fmt.Errorf("submit transaction: %w", err)
	}
	return resp.Hash, nil
}

// FundTestnet requests Friendbot funding for a testnet account.
func (c *Client) FundTestnet(address string) (string, error) {
	if c.network != NetworkTestnet {
		return "", ErrMainnetFriendbot
	}
	hash, err := withRateLimitRetry(func() (string, error) {
		tx, fundErr := horizonclient.DefaultTestNetClient.Fund(strings.TrimSpace(address))
		if fundErr != nil {
			return "", fundErr
		}
		return tx.Hash, nil
	})
	if err != nil {
		var hErr *horizonclient.Error
		if errors.As(err, &hErr) && hErr.Problem.Status == 429 {
			return "", ErrFriendbotLimit
		}
		return "", err
	}
	return hash, nil
}

// SignXDR appends a signature from the provided Stellar secret to a base64 XDR envelope.
func (c *Client) SignXDR(txe, secret string) (string, string, error) {
	full, err := parseFull(secret)
	if err != nil {
		return "", "", err
	}
	gtx, err := txnbuild.TransactionFromXDR(txe)
	if err != nil {
		return "", "", fmt.Errorf("parse transaction xdr: %w", err)
	}
	tx, ok := gtx.Transaction()
	if !ok {
		return "", "", fmt.Errorf("fee bump transactions are not supported")
	}
	signed, err := tx.Sign(c.passphrase, full)
	if err != nil {
		return "", "", fmt.Errorf("sign transaction: %w", err)
	}
	base64XDR, err := signed.Base64()
	if err != nil {
		return "", "", fmt.Errorf("encode signed transaction: %w", err)
	}
	return base64XDR, full.Address(), nil
}

// PaymentTx builds an unsigned native XLM payment.
func (c *Client) PaymentTx(source hProtocol.Account, destination, amount, memo string) (*txnbuild.Transaction, error) {
	op := &txnbuild.Payment{
		Destination: strings.TrimSpace(destination),
		Amount:      strings.TrimSpace(amount),
		Asset:       txnbuild.NativeAsset{},
	}
	params := txnbuild.TransactionParams{
		SourceAccount:        &source,
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
	return tx, nil
}

// SetOptionsTx builds an unsigned SetOptions transaction.
func (c *Client) SetOptionsTx(source hProtocol.Account, op txnbuild.SetOptions) (*txnbuild.Transaction, error) {
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &source,
		IncrementSequenceNum: true,
		Operations:           []txnbuild.Operation{&op},
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
	})
	if err != nil {
		return nil, fmt.Errorf("build set-options transaction: %w", err)
	}
	return tx, nil
}

func parseFull(secret string) (*keypair.Full, error) {
	kp, err := keypair.ParseFull(strings.TrimSpace(secret))
	if err != nil {
		return nil, fmt.Errorf("invalid Stellar secret key")
	}
	return kp, nil
}

// ValidateAddress rejects malformed Stellar public keys.
func ValidateAddress(address string) error {
	if _, err := keypair.ParseAddress(strings.TrimSpace(address)); err != nil {
		return ErrInvalidAddress
	}
	return nil
}

var stroopMultiplier = big.NewInt(10_000_000)

// ParseAmountToStroops converts an XLM decimal amount to stroops.
func ParseAmountToStroops(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, ErrInvalidAmount
	}
	if strings.HasPrefix(value, "+") {
		value = strings.TrimPrefix(value, "+")
	}
	if strings.HasPrefix(value, "-") {
		return 0, ErrInvalidAmount
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, ErrInvalidAmount
	}
	intPart := parts[0]
	if intPart == "" {
		intPart = "0"
	}
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if len(fracPart) > 7 {
		return 0, ErrInvalidAmount
	}
	for len(fracPart) < 7 {
		fracPart += "0"
	}
	whole, ok := new(big.Int).SetString(intPart, 10)
	if !ok {
		return 0, ErrInvalidAmount
	}
	fraction := big.NewInt(0)
	if fracPart != "" {
		var fracOK bool
		fraction, fracOK = new(big.Int).SetString(fracPart, 10)
		if !fracOK {
			return 0, ErrInvalidAmount
		}
	}
	total := new(big.Int).Mul(whole, stroopMultiplier)
	total.Add(total, fraction)
	if total.Sign() <= 0 || !total.IsInt64() {
		return 0, ErrInvalidAmount
	}
	return total.Int64(), nil
}

// FormatStroops renders stroops as a decimal XLM string.
func FormatStroops(stroops int64) string {
	sign := ""
	if stroops < 0 {
		sign = "-"
		stroops = -stroops
	}
	whole := stroops / 10_000_000
	fraction := stroops % 10_000_000
	return fmt.Sprintf("%s%d.%07d", sign, whole, fraction)
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
		var hErr *horizonclient.Error
		if !errors.As(err, &hErr) || hErr.Problem.Status != 429 || attempt == 2 {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
	}
	return zero, lastErr
}
