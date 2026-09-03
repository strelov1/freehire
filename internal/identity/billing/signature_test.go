package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"
)

const testSecret = "whsec_test"

// signHeader builds the header the provider would send for this body at this instant.
func signHeader(body []byte, secret string, at time.Time) string {
	ts := at.Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifySignature(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"api_version":"1.0","event":{"id":"e1","type":"RENEWAL"}}`)

	cases := []struct {
		name    string
		body    []byte
		header  string
		secret  string
		wantErr bool
	}{
		{
			name:   "a delivery signed now",
			body:   body,
			header: signHeader(body, testSecret, now),
			secret: testSecret,
		},
		{
			name:   "a delivery signed a moment ago",
			body:   body,
			header: signHeader(body, testSecret, now.Add(-30*time.Second)),
			secret: testSecret,
		},
		{
			name:    "signed with the wrong secret",
			body:    body,
			header:  signHeader(body, "whsec_other", now),
			secret:  testSecret,
			wantErr: true,
		},
		{
			name:    "no header at all",
			body:    body,
			header:  "",
			secret:  testSecret,
			wantErr: true,
		},
		{
			name:    "header missing the signature",
			body:    body,
			header:  fmt.Sprintf("t=%d", now.Unix()),
			secret:  testSecret,
			wantErr: true,
		},
		{
			name:    "header missing the timestamp",
			body:    body,
			header:  "v1=deadbeef",
			secret:  testSecret,
			wantErr: true,
		},
		{
			name:    "signature is not hex",
			body:    body,
			header:  fmt.Sprintf("t=%d,v1=not-hex", now.Unix()),
			secret:  testSecret,
			wantErr: true,
		},
		{
			name:    "timestamp is not a number",
			body:    body,
			header:  "t=yesterday,v1=deadbeef",
			secret:  testSecret,
			wantErr: true,
		},
		{
			// A correctly signed delivery is a credential until it goes stale. Without the
			// window, anyone who ever captures one delivery can replay it forever.
			name:    "a valid delivery replayed after the window",
			body:    body,
			header:  signHeader(body, testSecret, now.Add(-2*signatureWindow)),
			secret:  testSecret,
			wantErr: true,
		},
		{
			name:    "a timestamp far in the future",
			body:    body,
			header:  signHeader(body, testSecret, now.Add(2*signatureWindow)),
			secret:  testSecret,
			wantErr: true,
		},
		{
			// The body was tampered with after signing.
			name:    "the body does not match the signature",
			body:    []byte(`{"api_version":"1.0","event":{"id":"e2","type":"RENEWAL"}}`),
			header:  signHeader(body, testSecret, now),
			secret:  testSecret,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifySignature(tc.body, tc.header, tc.secret, now)
			if tc.wantErr && err == nil {
				t.Fatal("want an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want no error, got %v", err)
			}
		})
	}
}

// TestVerifySignatureIsOverRawBytes is the trap this whole function exists to avoid. The
// HMAC covers the bytes as received; a handler that parses the JSON and re-marshals it
// before verifying produces different bytes for the same event and rejects deliveries that
// are perfectly valid. The two bodies below are the same event and must NOT verify against
// one signature.
func TestVerifySignatureIsOverRawBytes(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	asReceived := []byte(`{"api_version": "1.0", "event": {"id": "e1"}}`)
	reserialised := []byte(`{"api_version":"1.0","event":{"id":"e1"}}`)

	header := signHeader(asReceived, testSecret, now)

	if err := verifySignature(asReceived, header, testSecret, now); err != nil {
		t.Fatalf("the bytes as received must verify: %v", err)
	}
	if err := verifySignature(reserialised, header, testSecret, now); err == nil {
		t.Fatal("a re-serialised body must not verify — if it does, the test is not proving anything")
	}
}
