package nebula

import "errors"

var (
	// ErrWalletNotFound reports missing wallet state.
	ErrWalletNotFound = errors.New("wallet not found")
	// ErrCorruptWallet reports malformed wallet data on disk.
	ErrCorruptWallet = errors.New("wallet file is corrupted")
	// ErrInvalidSecret reports malformed Stellar seeds.
	ErrInvalidSecret = errors.New("invalid Stellar secret key")
	// ErrInvalidAddress reports malformed public addresses.
	ErrInvalidAddress = errors.New("invalid Stellar address")
	// ErrInvalidAmount reports malformed or non-positive amounts.
	ErrInvalidAmount = errors.New("amount must be greater than 0")
	// ErrAccountNotFunded reports missing on-chain accounts.
	ErrAccountNotFunded = errors.New("account not funded")
	// ErrUnsupportedNetwork reports invalid network selection.
	ErrUnsupportedNetwork = errors.New("unsupported network")
	// ErrInsufficientBalance reports reserve-aware spending failures.
	ErrInsufficientBalance = errors.New("insufficient balance after reserve")
	// ErrMainnetFriendbot reports invalid Friendbot usage.
	ErrMainnetFriendbot = errors.New("friendbot is only available on testnet")
	// ErrInvalidPassphrase reports decryption failures.
	ErrInvalidPassphrase = errors.New("invalid passphrase")
	// ErrFriendbotLimit reports Friendbot rate limiting.
	ErrFriendbotLimit = errors.New("friendbot limit reached")
)
