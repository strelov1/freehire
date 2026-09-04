package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader is the header the provider signs each delivery with.
const SignatureHeader = "Stripe-Signature"

// signatureWindow bounds how old a signed delivery may be. Five minutes is the provider's
// own default tolerance.
//
// The provider retries for up to three days, and this is deliberately NOT wide enough to
// admit a late retry — a retry is re-signed when it is sent, so the window bounds the age of
// the SIGNATURE rather than the age of the event. Widening it to accommodate retries would
// only lengthen the life of a captured delivery.
const signatureWindow = 5 * time.Minute

var errNoSignature = errors.New("billing: delivery carries no signature")

// verifySignature checks that a delivery was signed by the provider, recently.
//
// raw MUST be the request body exactly as received. The HMAC covers those bytes, so a
// caller that parses the JSON and re-marshals it before verifying will reject valid
// deliveries — the same event, serialised differently, is different bytes.
//
// Two independent checks, and both are load-bearing. The MAC says the body came from
// someone holding the secret. The freshness window says it is not a delivery captured
// last month and sent again: a signature with no time bound is a bearer credential, which
// is exactly the property that made the shared Authorization header worth replacing.
func verifySignature(raw []byte, header, secret string, now time.Time) error {
	if strings.TrimSpace(header) == "" {
		return errNoSignature
	}

	ts, sig, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	signedAt := time.Unix(ts, 0)
	if drift := now.Sub(signedAt); drift > signatureWindow || drift < -signatureWindow {
		return fmt.Errorf("billing: signature is %s away from now, outside the %s window", drift.Round(time.Second), signatureWindow)
	}

	// The signed payload is "<timestamp>.<raw body>", written in two goes rather than
	// concatenated, so the body is never copied to be hashed.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(raw)

	// subtle.ConstantTimeCompare, not bytes.Equal: this comparison is the whole of the
	// endpoint's authentication, so how long it takes to fail is the only thing it could
	// leak. It returns 0 for unequal lengths, so a truncated signature is handled here too.
	if subtle.ConstantTimeCompare(mac.Sum(nil), sig) != 1 {
		return errors.New("billing: signature does not match the body")
	}
	return nil
}

// parseSignatureHeader reads `t=<unix seconds>,v1=<hex>`.
//
// Unknown elements are ignored rather than rejected: the scheme is versioned in the `v1`
// key, so the provider adding a `v2` beside it must not turn every delivery into a
// rejection. A missing t or v1 is still an error — that is not a new scheme, it is a
// malformed one.
//
// Ignoring every scheme but v1 is the provider's own instruction, and it is a DOWNGRADE
// defence: they send a deliberately fake `v0` on test deliveries, so a verifier that
// accepted any scheme would accept a signature it never checked.
func parseSignatureHeader(header string) (ts int64, sig []byte, err error) {
	var sawTS, sawSig bool
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			ts, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, nil, fmt.Errorf("billing: signature timestamp %q is not a number", value)
			}
			sawTS = true
		case "v1":
			sig, err = hex.DecodeString(value)
			if err != nil {
				return 0, nil, fmt.Errorf("billing: signature %q is not hex", value)
			}
			sawSig = true
		}
	}
	if !sawTS || !sawSig {
		return 0, nil, fmt.Errorf("billing: signature header %q is missing t or v1", header)
	}
	return ts, sig, nil
}
