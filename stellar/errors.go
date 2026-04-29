package stellar

import "errors"

var (
	// ErrInvalidAddress reports malformed Stellar account ids.
	ErrInvalidAddress = errors.New("invalid Stellar address")
	// ErrInvalidAmount reports malformed or non-positive XLM amounts.
	ErrInvalidAmount = errors.New("amount must be greater than 0")
	// ErrAccountNotFunded reports missing on-chain accounts.
	ErrAccountNotFunded = errors.New("account not funded")
	// ErrInsufficientBalance reports reserve-aware payment failures.
	ErrInsufficientBalance = errors.New("insufficient balance after reserve")
	// ErrMainnetFriendbot reports invalid Friendbot usage outside testnet.
	ErrMainnetFriendbot = errors.New("friendbot is only available on testnet")
	// ErrFriendbotLimit reports Friendbot rate limiting.
	ErrFriendbotLimit = errors.New("friendbot limit reached")
)
