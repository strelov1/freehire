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

// The header name lives with each provider (stripeSignatureHeader, revenuecatSignatureHeader).
// The scheme is shared: `t=<unix>,v1=<hex>` over "<timestamp>.<raw body>" with HMAC-SHA256.

// stripeSignatureWindow bounds how old a signed Stripe delivery may be (the provider's
// default tolerance). Retries are re-signed when sent, so the window bounds the signature's
// age, not the event's; widening it only lengthens the life of a captured delivery.
// It is a parameter because RevenueCat's retry signing is undocumented (see
// revenuecatSignatureWindow).
const stripeSignatureWindow = 5 * time.Minute

// ErrBadSignature marks every reason a delivery failed to authenticate: no header, a
// malformed one, a stale timestamp, a MAC mismatch. Exported so the handler can answer 401
// for it and 400 for a validly signed but unreadable body.
var ErrBadSignature = errors.New("billing: delivery is not signed by the provider")

var errNoSignature = fmt.Errorf("%w: delivery carries no signature", ErrBadSignature)

// verifySignature checks that a delivery was signed by the provider, recently.
//
// raw MUST be the body exactly as received: the HMAC covers those bytes, so re-marshalled
// JSON is rejected. Two checks, both required: the MAC proves the secret holder sent it, and
// the freshness window stops a captured delivery being replayed.
func verifySignature(raw []byte, header, secret string, window time.Duration, now time.Time) error {
	if strings.TrimSpace(header) == "" {
		return errNoSignature
	}

	ts, sigs, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	signedAt := time.Unix(ts, 0)
	if drift := now.Sub(signedAt); drift > window || drift < -window {
		return fmt.Errorf("%w: signature is %s away from now, outside the %s window", ErrBadSignature, drift.Round(time.Second), window)
	}

	// The payload is "<timestamp>.<raw body>", written in two parts to avoid copying the body.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(raw)
	expected := mac.Sum(nil)

	// Try every v1: while two endpoint secrets are active the provider sends two v1 elements,
	// and checking only one would reject about half the deliveries during a rotation.
	//
	// Constant-time compare, since this is the endpoint's whole authentication; it returns 0
	// for unequal lengths, so truncated signatures are handled too.
	for _, sig := range sigs {
		if subtle.ConstantTimeCompare(expected, sig) == 1 {
			return nil
		}
	}
	return fmt.Errorf("%w: no signature matches the body", ErrBadSignature)
}

// parseSignatureHeader reads `t=<unix seconds>,v1=<hex>[,v1=<hex>…]`.
//
// It returns every v1 as a candidate (two are sent during a secret rotation). Unknown
// elements are ignored so a future `v2` does not reject everything, but a missing t or v1 is
// an error. Ignoring schemes other than v1 is a downgrade defence: test deliveries carry a
// deliberately fake `v0`.
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
