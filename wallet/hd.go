package wallet

import (
	"fmt"
	"strings"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/tools/stellar-hd-wallet/crypto/derivation"
	"github.com/tyler-smith/go-bip39"
)

var allowedWordCounts = map[int]bool{12: true, 24: true}

// GenerateMnemonic returns a new BIP39 mnemonic with the requested word count.
func GenerateMnemonic(words int) (string, error) {
	if words == 0 {
		words = defaultWords
	}
	if !allowedWordCounts[words] {
		return "", ErrInvalidWordsCount
	}
	entropyBits := 128
	if words == 24 {
		entropyBits = 256
	}
	entropy, err := bip39.NewEntropy(entropyBits)
	if err != nil {
		return "", fmt.Errorf("generate entropy: %w", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("generate mnemonic: %w", err)
	}
	return mnemonic, nil
}

// NormalizeMnemonic validates and returns a normalized mnemonic phrase.
func NormalizeMnemonic(mnemonic string) (string, error) {
	mnemonic = strings.Join(strings.Fields(strings.TrimSpace(strings.ToLower(mnemonic))), " ")
	if !bip39.IsMnemonicValid(mnemonic) {
		return "", ErrInvalidMnemonic
	}
	return mnemonic, nil
}

// DeriveAccount derives a Stellar account by SEP-0005 account index.
func DeriveAccount(mnemonic string, index uint32, name string) (DerivedAccount, string, error) {
	normalized, err := NormalizeMnemonic(mnemonic)
	if err != nil {
		return DerivedAccount{}, "", err
	}
	seed := bip39.NewSeed(normalized, "")
	path := fmt.Sprintf(derivation.StellarAccountPathFormat, index)
	key, err := derivation.DeriveForPath(path, seed)
	if err != nil {
		return DerivedAccount{}, "", fmt.Errorf("derive account path %s: %w", path, err)
	}
	kp, err := keypair.FromRawSeed(key.RawSeed())
	if err != nil {
		return DerivedAccount{}, "", fmt.Errorf("create Stellar keypair: %w", err)
	}
	return DerivedAccount{
		Index:   index,
		Name:    strings.TrimSpace(name),
		Path:    path,
		Address: kp.Address(),
	}, kp.Seed(), nil
}
