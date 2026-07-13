package tokencrypt

import (
	"strings"
	"testing"
)

func key(b byte) []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = b
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	c, err := New(key(1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const secret = "1//refresh-token-value-abc123"
	ct, err := c.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if strings.Contains(ct, secret) {
		t.Fatal("ciphertext leaks the plaintext")
	}
	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != secret {
		t.Errorf("Decrypt = %q, want %q", got, secret)
	}
}

func TestNonceIsRandom(t *testing.T) {
	c, _ := New(key(1))
	a, _ := c.Encrypt("x")
	b, _ := c.Encrypt("x")
	if a == b {
		t.Error("same plaintext produced identical ciphertext (nonce reuse)")
	}
}

func TestWrongKeyFails(t *testing.T) {
	c1, _ := New(key(1))
	c2, _ := New(key(2))
	ct, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(ct); err == nil {
		t.Error("decrypt with the wrong key should fail")
	}
}

func TestRejectsBadKeyLength(t *testing.T) {
	if _, err := New([]byte("too-short")); err == nil {
		t.Error("expected error for a non-32-byte key")
	}
}

func TestRejectsGarbageCiphertext(t *testing.T) {
	c, _ := New(key(1))
	if _, err := c.Decrypt("not-valid-base64-!!"); err == nil {
		t.Error("expected error decrypting garbage")
	}
}
