package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
			header:  signHeader(body, testSecret, now.Add(-2*stripeSignatureWindow)),
			secret:  testSecret,
			wantErr: true,
		},
		{
			name:    "a timestamp far in the future",
			body:    body,
			header:  signHeader(body, testSecret, now.Add(2*stripeSignatureWindow)),
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
			err := verifySignature(tc.body, tc.header, tc.secret, stripeSignatureWindow, now)
			if tc.wantErr && err == nil {
				t.Fatal("want an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want no error, got %v", err)
			}
		})
	}
}

// TestVerifySignatureAcceptsAnyMatchingV1 is the endpoint-secret ROTATION case, and it is
// the one a verifier that keeps a single v1 fails.
//
// While two endpoint secrets are active the provider signs each delivery with both and
// sends two v1 elements in one header. It does not say which is which, and the order is not
// ours to rely on — so both orderings are exercised, and both must verify against the one
// secret this deployment holds. Keeping only the last element passes the first case and
// fails the second, which is what "roughly half of all deliveries rejected" looks like from
// inside a test.
func TestVerifySignatureAcceptsAnyMatchingV1(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"id":"evt_1","type":"invoice.paid","data":{"object":{}}}`)

	// The header the provider builds during a rollover: one signature per active secret.
	ours := signHeader(body, testSecret, now)
	theirs := signHeader(body, "whsec_rotating_in", now)
	// signHeader emits "t=<unix>,v1=<hex>"; this is the v1 element on its own, to append.
	sigOf := func(header string) string { _, v1, _ := strings.Cut(header, ","); return v1 }

	cases := map[string]string{
		"ours first":  ours + "," + sigOf(theirs),
		"ours second": theirs + "," + sigOf(ours),
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			if err := verifySignature(body, header, testSecret, stripeSignatureWindow, now); err != nil {
				t.Fatalf("a header carrying our signature beside another must verify: %v", err)
			}
		})
	}

	// The converse, or the test above proves only that we accept things. A header of
	// signatures none of which is ours stays a rejection however many there are.
	none := theirs + "," + sigOf(signHeader(body, "whsec_third", now))
	if err := verifySignature(body, none, testSecret, stripeSignatureWindow, now); err == nil {
		t.Fatal("a header carrying only OTHER secrets' signatures must not verify")
	}
}

// TestVerifySignatureErrorsAreBadSignature pins what the handler branches on: every refusal
// from this function carries ErrBadSignature, because the handler answers 401 to those and
// 400 to a body that verified and then failed to parse. A refusal that lost the sentinel
// would be answered 400 — a status the provider treats as permanent, on a delivery it should
// have been told to stop retrying for a different reason.
func TestVerifySignatureErrorsAreBadSignature(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"id":"evt_1"}`)

	headers := map[string]string{
		"no header":         "",
		"missing v1":        fmt.Sprintf("t=%d", now.Unix()),
		"missing t":         "v1=deadbeef",
		"unparseable t":     "t=yesterday,v1=deadbeef",
		"signature not hex": fmt.Sprintf("t=%d,v1=not-hex", now.Unix()),
		"stale":             signHeader(body, testSecret, now.Add(-2*stripeSignatureWindow)),
		"wrong secret":      signHeader(body, "whsec_other", now),
	}
	for name, header := range headers {
		t.Run(name, func(t *testing.T) {
			err := verifySignature(body, header, testSecret, stripeSignatureWindow, now)
			if err == nil {
				t.Fatal("want a refusal, got nil")
			}
			if !errors.Is(err, ErrBadSignature) {
				t.Fatalf("every refusal must carry ErrBadSignature, got %v", err)
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

	if err := verifySignature(asReceived, header, testSecret, stripeSignatureWindow, now); err != nil {
		t.Fatalf("the bytes as received must verify: %v", err)
	}
	if err := verifySignature(reserialised, header, testSecret, stripeSignatureWindow, now); err == nil {
		t.Fatal("a re-serialised body must not verify — if it does, the test is not proving anything")
	}
}
