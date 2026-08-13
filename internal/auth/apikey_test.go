package auth

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey_FormatPrefixAndHash(t *testing.T) {
	token, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	if !strings.HasPrefix(token, "fhk_") {
		t.Errorf("token %q does not start with fhk_", token)
	}
	// The display prefix is a non-secret leading slice of the token, shorter than
	// the whole token, so it identifies a key without revealing it.
	if !strings.HasPrefix(token, prefix) {
		t.Errorf("prefix %q is not a leading slice of token %q", prefix, token)
	}
	if len(prefix) >= len(token) {
		t.Errorf("prefix %q must be shorter than the full token (len %d >= %d)", prefix, len(prefix), len(token))
	}
	if !strings.HasPrefix(prefix, "fhk_") {
		t.Errorf("prefix %q does not start with fhk_", prefix)
	}

	// What we store is the hash, never the token; the returned hash must match
	// HashAPIKey(token) so an authenticating lookup can find it.
	if hash != HashAPIKey(token) {
		t.Errorf("returned hash %q != HashAPIKey(token) %q", hash, HashAPIKey(token))
	}
	if hash == token {
		t.Error("hash must not equal the plaintext token")
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	t1, h1, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	t2, h2, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if t1 == t2 {
		t.Error("two generated tokens are identical")
	}
	if h1 == h2 {
		t.Error("two generated hashes are identical")
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	const token = "fhk_example-token"
	if HashAPIKey(token) != HashAPIKey(token) { //nolint:staticcheck // SA4000: intentional determinism check, calling twice is the point
		t.Error("HashAPIKey is not deterministic for the same input")
	}
	if HashAPIKey("fhk_a") == HashAPIKey("fhk_b") {
		t.Error("distinct tokens hashed to the same value")
	}
}
