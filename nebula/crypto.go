package nebula

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	kdfN      = 1 << 15
	kdfR      = 8
	kdfP      = 1
	keyLength = 32
)

type encryptedSecret struct {
	KDF        string `json:"kdf"`
	Cipher     string `json:"cipher"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func encryptSecret(secret, passphrase string) (encryptedSecret, error) {
	if passphrase == "" {
		return encryptedSecret{}, ErrInvalidPassphrase
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return encryptedSecret{}, fmt.Errorf("generate salt: %w", err)
	}
	key, err := scrypt.Key([]byte(passphrase), salt, kdfN, kdfR, kdfP, keyLength)
	if err != nil {
		return encryptedSecret{}, fmt.Errorf("derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedSecret{}, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedSecret{}, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encryptedSecret{}, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)
	return encryptedSecret{
		KDF:        "scrypt",
		Cipher:     "aes-256-gcm",
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptSecret(data encryptedSecret, passphrase string) (string, error) {
	if passphrase == "" {
		return "", ErrInvalidPassphrase
	}
	salt, err := base64.StdEncoding.DecodeString(data.Salt)
	if err != nil {
		return "", ErrCorruptWallet
	}
	nonce, err := base64.StdEncoding.DecodeString(data.Nonce)
	if err != nil {
		return "", ErrCorruptWallet
	}
	ciphertext, err := base64.StdEncoding.DecodeString(data.Ciphertext)
	if err != nil {
		return "", ErrCorruptWallet
	}

	key, err := scrypt.Key([]byte(passphrase), salt, kdfN, kdfR, kdfP, keyLength)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidPassphrase
	}
	return string(plaintext), nil
}
