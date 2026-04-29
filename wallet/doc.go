// Package wallet implements encrypted HD wallet storage and deterministic
// account derivation for Nebula.
//
// The package stores mnemonic-backed wallets in a local BadgerDB database at
// ~/.nebula/wallet.db, encrypts mnemonic material at rest, derives Stellar
// accounts according to SEP-0005, and provides confirmation gates for
// sensitive operations.
package wallet
