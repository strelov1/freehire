package discordbot

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(pub)

	timestamp := []byte("1700000000")
	body := []byte(`{"type":1}`)
	sig := ed25519.Sign(priv, append(append([]byte{}, timestamp...), body...))
	sigHex := hex.EncodeToString(sig)

	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	otherPubHex := hex.EncodeToString(otherPub)

	tests := []struct {
		name      string
		pubKeyHex string
		timestamp []byte
		body      []byte
		sigHex    string
		want      bool
	}{
		{"valid signature", pubHex, timestamp, body, sigHex, true},
		{"tampered body", pubHex, timestamp, []byte(`{"type":2}`), sigHex, false},
		{"wrong timestamp", pubHex, []byte("1700000001"), body, sigHex, false},
		{"garbage public key hex", "not-hex-!!!", timestamp, body, sigHex, false},
		{"garbage signature hex", pubHex, timestamp, body, "not-hex-!!!", false},
		{"wrong-length public key", hex.EncodeToString([]byte("too-short")), timestamp, body, sigHex, false},
		{"wrong-length signature", pubHex, timestamp, body, hex.EncodeToString([]byte("too-short")), false},
		{"signed with different key", otherPubHex, timestamp, body, sigHex, false},
		{"empty everything", "", nil, nil, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifySignature(tt.pubKeyHex, tt.timestamp, tt.body, tt.sigHex)
			if got != tt.want {
				t.Errorf("VerifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestVerifySignature_ConcatenationOrder guards the exact byte order Discord
// requires: timestamp || body, not body || timestamp. A signature built with
// the fields swapped must not verify against a check that uses the correct
// order.
func TestVerifySignature_ConcatenationOrder(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(pub)

	timestamp := []byte("1700000000")
	body := []byte(`{"type":1}`)

	swapped := ed25519.Sign(priv, append(append([]byte{}, body...), timestamp...))
	if VerifySignature(pubHex, timestamp, body, hex.EncodeToString(swapped)) {
		t.Error("VerifySignature accepted a signature computed over body||timestamp, want timestamp||body only")
	}

	correct := ed25519.Sign(priv, append(append([]byte{}, timestamp...), body...))
	if !VerifySignature(pubHex, timestamp, body, hex.EncodeToString(correct)) {
		t.Error("VerifySignature rejected a correctly-ordered timestamp||body signature")
	}
}
