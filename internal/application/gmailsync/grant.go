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
// The status lets callers tell 401 from 429, 500 or a decode failure, which decide whether to
// disconnect the candidate. internal/application/calsync uses this same type: one grant and
// one status flag cover both scopes, so one rule must read them.
type APIError struct {
	// Op names the call.
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
// Only a revoked grant may disconnect the candidate. The status is SHARED between mail and
// calendar, only a browser consent clears it, and the mail scope is restricted, so misreading a
// provider incident as revocation would disconnect every mailbox at once.
//
// Two carriers:
//
//   - the API refusing an access token with 401 or 403, except a 403 whose reason is a rate
//     limit or quota (Gmail sends that for a burst of reads);
//   - the token endpoint refusing the refresh token with `invalid_grant`, where a real
//     revocation lands before any API call. It arrives as a *url.Error wrapping
//     *oauth2.RetrieveError, hence the unwrapping.
//
// Nothing else from that endpoint counts: `invalid_client` is our own credential being wrong.
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
