package gmailsync

import (
	"errors"
	"fmt"
	"net/http"

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
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Op, e.Status) }

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
//   - the API refusing an access token with 401 or 403;
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
		return apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden
	}
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		return retrieve.ErrorCode == invalidGrant
	}
	return false
}
