package wallet

import "errors"

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrCorruptWallet       = errors.New("wallet file is corrupted")
	ErrInvalidSecret       = errors.New("invalid Stellar secret key")
	ErrInvalidAddress      = errors.New("invalid Stellar address")
	ErrInvalidAmount       = errors.New("amount must be greater than 0")
	ErrAccountNotFunded    = errors.New("account not funded")
	ErrUnsupportedNetwork  = errors.New("unsupported network")
	ErrInsufficientBalance = errors.New("insufficient balance after reserve")
	ErrMainnetFriendbot    = errors.New("friendbot is only available on testnet")
)
