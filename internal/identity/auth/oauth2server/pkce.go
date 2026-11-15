// Package oauth2server is freehire's OAuth 2.1 authorization-server side: PKCE and the
// opaque token/code primitives of the MCP sign-in flow. It is the opposite direction from
// the sibling auth/oauth package (a client of Google/GitHub) and shares no code.
package oauth2server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// VerifyPKCE reports whether verifier is the S256 preimage of challenge (plain is refused
// at the authorize step). The comparison is constant-time.
func VerifyPKCE(challenge, verifier string) bool {
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}
