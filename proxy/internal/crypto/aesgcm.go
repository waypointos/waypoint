// Package crypto provides AES-256-GCM authenticated encryption helpers used
// by the proxy to store and retrieve sensitive token material.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// AESGCM wraps a fixed 32-byte AES-256-GCM key and provides Seal/Open
// operations. Ciphertext is stored as nonce || ciphertext (nonce is 12 bytes).
type AESGCM struct {
	aead cipher.AEAD
}

// NewAESGCM constructs an AESGCM from a 32-byte key.
func NewAESGCM(key []byte) (*AESGCM, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("aesgcm: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: new gcm: %w", err)
	}
	return &AESGCM{aead: aead}, nil
}

// Seal encrypts and authenticates plaintext. The returned bytes are
// nonce || ciphertext where the nonce is a random 12-byte value.
func (a *AESGCM) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("aesgcm: generate nonce: %w", err)
	}
	ciphertext := a.aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Open decrypts and verifies a value previously produced by Seal.
func (a *AESGCM) Open(ciphertext []byte) ([]byte, error) {
	ns := a.aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("aesgcm: ciphertext too short")
	}
	nonce, data := ciphertext[:ns], ciphertext[ns:]
	plaintext, err := a.aead.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: open: %w", err)
	}
	return plaintext, nil
}
