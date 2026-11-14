package gmailsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

// APIError is a Google API answer this service could not use, carrying the HTTP status.
//
// The status is the whole point. An untyped error makes 401, 429, 500, a connection reset
// and a decode failure indistinguishable, and the one decision this failure feeds — whether
// to disconnect the candidate — has opposite answers for the first and for all the rest.
//
// internal/application/calsync marks its own answers with this same type rather than a
// second one of its own: one Google grant covers both scopes and one status flag records
// its health, so the rule that reads them has to be one rule.
type APIError struct {
	// Op names the call, so the message still reads the way the untyped fmt.Errorf did.
	Op         string
	StatusCode int
	Status     string
	// Reason is Google's error reason from the body (rateLimitExceeded, insufficientPermissions…),
	// empty when the body carried none.
	Reason string
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Op, e.Status, e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Status)
}

// NewAPIError builds the error for a non-200 Google answer, reading the reason out of its
// body: Gmail answers a per-user rate limit with 403, not 429, and only the reason tells
// that apart from a refused grant.
func NewAPIError(op string, resp *http.Response) *APIError {
	var body struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&body)
	e := &APIError{Op: op, StatusCode: resp.StatusCode, Status: resp.Status}
	if len(body.Error.Errors) > 0 {
		e.Reason = body.Error.Errors[0].Reason
	}
	return e
}

// LimitHit reports whether err is Google refusing the pace rather than the grant: a 429, or
// a 403 whose reason is a rate limit or quota.
func LimitHit(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusTooManyRequests ||
		(apiErr.StatusCode == http.StatusForbidden && limitReason(apiErr.Reason))
}

// limitReason reports a reason meaning a quota or rate limit (rateLimitExceeded,
// userRateLimitExceeded, quotaExceeded, dailyLimitExceeded): the grant is fine, the pace is not.
func limitReason(reason string) bool {
	r := strings.ToLower(reason)
	return strings.Contains(r, "limit") || strings.Contains(r, "quota")
}

// invalidGrant is RFC 6749's code for a refresh token the provider will not honour again:
// revoked at Google, expired through disuse, or invalidated by a password change.
const invalidGrant = "invalid_grant"

// RevokedGrant reports whether err means the candidate's Google grant itself is gone, as
// against the provider being unwell, a quota being spent, or this run being cancelled.
//
// Only the first may cost them their connection. The status it sets is SHARED between mail
// and calendar, nothing but a browser consent round trip clears it, and the mail half is a
// restricted scope Google makes them re-approve — so one provider incident read as a
// revocation disconnects every mailbox we hold, all at once, and each candidate has to go
// through that consent again.
//
// TWO carriers, because a revocation reaches these workers on either leg:
//
//   - the API refusing an access token with 401 or 403 — except a 403 whose reason is a
//     rate limit or quota, which Gmail sends for a burst of reads (it disconnected 4 of 18
//     mailboxes on 2026-11-14, #4610);
//   - the TOKEN endpoint refusing the refresh token, which is where a real revocation
//     actually lands. The readers' client is built from a stored refresh token
//     (Connector.HTTPClient), so oauth2 exchanges it before any API call is made: Google
//     answers `invalid_grant` and the API is never reached at all. That error arrives as a
//     *url.Error wrapping *oauth2.RetrieveError, which is why this unwraps rather than
//     inspecting one concrete type.
//
// `invalid_grant` and nothing else from that endpoint. `invalid_client` is OUR credential
// being wrong, and disconnecting every candidate over a mis-deployed client secret is
// exactly the failure this function exists to prevent.
func RevokedGrant(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusUnauthorized ||
			(apiErr.StatusCode == http.StatusForbidden && !limitReason(apiErr.Reason))
	}
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		return retrieve.ErrorCode == invalidGrant
	}
	return false
}
