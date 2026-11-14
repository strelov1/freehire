// Package changehook pushes the job change feed (job_changes, GET /api/v1/jobs/changes) to
// the HTTPS endpoints of API customers: signed, batched, at-least-once deliveries driven by a
// persisted per-subscription position in the feed. The subscription rows are change_webhooks
// (migration 0264); cmd/changes-webhook runs the delivery loop.
//
// It is separate from webhooknotify and notify, which deliver saved-search digests to one
// unsigned URL per account on that engine's own retry rules.
package changehook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// The headers of every delivery. The signature covers the timestamp and the exact body.
const (
	HeaderSignature = "X-Freehire-Signature"
	HeaderTimestamp = "X-Freehire-Timestamp"
	HeaderDelivery  = "X-Freehire-Delivery"
	HeaderEvent     = "X-Freehire-Event"

	signatureVersion = "v1="
)

// DefaultTolerance is how far a delivery's timestamp may be from the receiver's clock before
// Verify refuses it, so a captured request cannot be replayed later.
const DefaultTolerance = 5 * time.Minute

// Sign is the signature header value for a body sent at ts (unix seconds):
// "v1=" + hex(HMAC-SHA256(secret, ts + "." + body)).
func Sign(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return signatureVersion + hex.EncodeToString(mac.Sum(nil))
}

var (
	ErrBadSignature = errors.New("changehook: signature does not match")
	ErrStale        = errors.New("changehook: timestamp outside the tolerance")
)

// Verify is what a receiver does with a delivery: the timestamp header, the signature
// header and the raw body, as received. It is here so the receiving side is tested against
// the sending side and the documented example is real code.
func Verify(secret, timestampHeader, signatureHeader string, body []byte, now time.Time, tolerance time.Duration) error {
	ts, err := strconv.ParseInt(strings.TrimSpace(timestampHeader), 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return ErrStale
	}
	if !hmac.Equal([]byte(Sign(secret, ts, body)), []byte(strings.TrimSpace(signatureHeader))) {
		return ErrBadSignature
	}
	return nil
}
