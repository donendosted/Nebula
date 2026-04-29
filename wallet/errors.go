package wallet

import "errors"

var (
	// ErrWalletNotFound reports a missing encrypted wallet root.
	ErrWalletNotFound = errors.New("wallet not found")
	// ErrWalletExists reports duplicate wallet identifiers.
	ErrWalletExists = errors.New("wallet already exists")
	// ErrInvalidPassphrase reports failed decryption.
	ErrInvalidPassphrase = errors.New("invalid passphrase")
	// ErrInvalidMnemonic reports malformed or checksum-invalid phrases.
	ErrInvalidMnemonic = errors.New("invalid mnemonic")
	// ErrInvalidWordsCount reports unsupported mnemonic sizes.
	ErrInvalidWordsCount = errors.New("invalid mnemonic word count")
	// ErrAccountNotDerived reports missing local derivation state.
	ErrAccountNotDerived = errors.New("account not derived")
	// ErrSensitiveActionRejected reports a missing explicit confirmation.
	ErrSensitiveActionRejected = errors.New("sensitive action requires confirmation")
)
