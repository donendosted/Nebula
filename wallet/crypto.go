package wallet

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

type encryptedBlob struct {
	KDF        string `json:"kdf"`
	Cipher     string `json:"cipher"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func encryptText(plaintext, passphrase string) (encryptedBlob, error) {
	if passphrase == "" {
		return encryptedBlob{}, ErrInvalidPassphrase
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return encryptedBlob{}, fmt.Errorf("generate salt: %w", err)
	}
	key, err := scrypt.Key([]byte(passphrase), salt, kdfN, kdfR, kdfP, keyLength)
	if err != nil {
		return encryptedBlob{}, fmt.Errorf("derive encryption key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedBlob{}, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedBlob{}, fmt.Errorf("create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encryptedBlob{}, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return encryptedBlob{
		KDF:        "scrypt",
		Cipher:     "aes-256-gcm",
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptText(blob encryptedBlob, passphrase string) (string, error) {
	if passphrase == "" {
		return "", ErrInvalidPassphrase
	}
	salt, err := base64.StdEncoding.DecodeString(blob.Salt)
	if err != nil {
		return "", ErrInvalidPassphrase
	}
	nonce, err := base64.StdEncoding.DecodeString(blob.Nonce)
	if err != nil {
		return "", ErrInvalidPassphrase
	}
	ciphertext, err := base64.StdEncoding.DecodeString(blob.Ciphertext)
	if err != nil {
		return "", ErrInvalidPassphrase
	}
	key, err := scrypt.Key([]byte(passphrase), salt, kdfN, kdfR, kdfP, keyLength)
	if err != nil {
		return "", fmt.Errorf("derive encryption key: %w", err)
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
