// Package oauth2server implements freehire's side of OAuth 2.1 as an
// authorization server: PKCE verification and the opaque token/code primitives
// the MCP sign-in flow (freehire#3114) is built from.
//
// Named distinctly from the sibling internal/identity/auth/oauth package, which
// is the opposite direction — a client resolving Google/GitHub identities
// inbound. The two share no code.
package oauth2server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// VerifyPKCE reports whether verifier is the preimage of challenge under S256 —
// the only code_challenge_method OAuth 2.1 permits here (plain is refused at the
// authorize step, never reaching this check). Constant-time: the challenge is
// public (it rode the authorize redirect), but the comparison habit is cheap and
// the alternative invites a timing-leak finding on every future touch of this file.
func VerifyPKCE(challenge, verifier string) bool {
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}
