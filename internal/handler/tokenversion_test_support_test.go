package handler

import "context"

// stubVersions is the session-generation loader the handler tests mount their routes
// with. Accounts start at generation 1 and none of these tests revoke a session, so a
// constant is exactly right here — revocation itself is covered where it lives, in
// internal/auth (middleware) and internal/db (the counter's SQL).
type stubVersions struct{ version int32 }

func (s stubVersions) GetUserTokenVersion(context.Context, int64) (int32, error) {
	return s.version, nil
}

// testVersions is the shared instance; testTokenVersion is the generation its tokens
// must be minted with to match.
var testVersions = stubVersions{version: testTokenVersion}

const testTokenVersion = 1
