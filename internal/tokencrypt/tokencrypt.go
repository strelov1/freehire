// Package tokencrypt encrypts secrets at rest (the Gmail refresh token) with
// AES-256-GCM (AEAD): confidentiality + integrity, a fresh random nonce per
// encryption prepended to the ciphertext, all base64-encoded for text columns.
package tokencrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Cipher encrypts and decrypts short secrets with a fixed 32-byte key.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a 32-byte (AES-256) key.
func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("tokencrypt: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns base64(nonce || ciphertext+tag).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt; it fails on a wrong key, tampering, or malformed input.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("tokencrypt: bad encoding: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("tokencrypt: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("tokencrypt: decrypt failed: %w", err)
	}
	return string(plain), nil
}
