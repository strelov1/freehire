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

// ErrBadSignature marks every reason a delivery failed to authenticate: no header, a
// malformed one, a stale timestamp, a MAC that does not match.
//
// It is exported so the handler can tell "not from the provider" (401, and they stop) from
// "from the provider but unreadable" (400, and they also stop). Both are refusals; only one
// is worth retrying, and neither is.
var ErrBadSignature = errors.New("billing: delivery is not signed by the provider")

var errNoSignature = fmt.Errorf("%w: delivery carries no signature", ErrBadSignature)

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

	ts, sigs, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	signedAt := time.Unix(ts, 0)
	if drift := now.Sub(signedAt); drift > signatureWindow || drift < -signatureWindow {
		return fmt.Errorf("%w: signature is %s away from now, outside the %s window", ErrBadSignature, drift.Round(time.Second), signatureWindow)
	}

	// The signed payload is "<timestamp>.<raw body>", written in two goes rather than
	// concatenated, so the body is never copied to be hashed. Once, then compared against
	// every candidate: the MAC does not depend on which signature we are checking.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(raw)
	expected := mac.Sum(nil)

	// EVERY v1 is tried, not just one, and that is a rotation requirement rather than
	// belt-and-braces. While two endpoint secrets are active the provider signs each
	// delivery with BOTH and sends two v1 elements in one header — so a verifier that keeps
	// only one of them rejects roughly half of all deliveries, for the whole rollover.
	// Which is precisely when a secret is being rotated because it leaked.
	//
	// subtle.ConstantTimeCompare, not bytes.Equal: this comparison is the whole of the
	// endpoint's authentication, so how long it takes to fail is the only thing it could
	// leak. It returns 0 for unequal lengths, so a truncated signature is handled here too.
	for _, sig := range sigs {
		if subtle.ConstantTimeCompare(expected, sig) == 1 {
			return nil
		}
	}
	return fmt.Errorf("%w: no signature matches the body", ErrBadSignature)
}

// parseSignatureHeader reads `t=<unix seconds>,v1=<hex>[,v1=<hex>…]`.
//
// It returns EVERY v1, because the provider sends more than one while an endpoint has two
// active secrets — see the note at the comparison. They are candidates, not a sequence:
// exactly one of them is expected to match, and which is not knowable here.
//
// Unknown elements are ignored rather than rejected: the scheme is versioned in the `v1`
// key, so the provider adding a `v2` beside it must not turn every delivery into a
// rejection. A missing t or v1 is still an error — that is not a new scheme, it is a
// malformed one.
//
// Ignoring every scheme but v1 is the provider's own instruction, and it is a DOWNGRADE
// defence: they send a deliberately fake `v0` on test deliveries, so a verifier that
// accepted any scheme would accept a signature it never checked.
func parseSignatureHeader(header string) (ts int64, sigs [][]byte, err error) {
	var sawTS bool
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			ts, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, nil, fmt.Errorf("%w: signature timestamp %q is not a number", ErrBadSignature, value)
			}
			sawTS = true
		case "v1":
			sig, decodeErr := hex.DecodeString(value)
			if decodeErr != nil {
				return 0, nil, fmt.Errorf("%w: signature %q is not hex", ErrBadSignature, value)
			}
			sigs = append(sigs, sig)
		}
	}
	if !sawTS || len(sigs) == 0 {
		return 0, nil, fmt.Errorf("%w: signature header %q is missing t or v1", ErrBadSignature, header)
	}
	return ts, sigs, nil
}
