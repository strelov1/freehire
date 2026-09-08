package emailprefs

import (
	"fmt"
	"net/url"
	"strings"
)

// Path is where a preference link lands. The web app serves it publicly, outside
// the account shell, because the whole point is that it opens without a session.
const Path = "/unsubscribe"

// Links turns a (user, group) into the URL that appears in a mail's footer and in
// its List-Unsubscribe header. It exists so the two can never disagree: both read
// the same string from one call, rather than each building it from a base and a
// token and hoping they agree about the path.
type Links struct {
	signer *Signer
	base   string
}

// minSecretLen mirrors the floor config puts on JWT_SECRET. A worker started
// without it would otherwise sign links with the empty string and mail out URLs the
// API can never verify — every one of them a dead unsubscribe link, which is the
// exact failure this whole change exists to remove.
const minSecretLen = 32

// NewLinks builds the link maker. secret is the session-signing secret (the salt
// keeps this key apart from it); origin is the frontend origin the mail links into.
//
// A secret too short to be the real one is kept, not rejected — but every For call
// then fails, so the mail fails to send and is retried rather than going out with a
// link that leads nowhere. Failing closed is the point: a dead unsubscribe link is
// worse than a late mail.
func NewLinks(secret, origin string) *Links {
	l := &Links{base: strings.TrimRight(origin, "/")}
	if len(secret) >= minSecretLen {
		l.signer = NewSigner(secret)
	}
	return l
}

// For returns the preference URL for one recipient and one group of mail. It fails
// for a group nobody may turn off, which is what stops an essential mail acquiring a
// link it cannot honour.
//
// The token rides in the query because a link in an email has nowhere else to carry
// it. That is a real cost — nginx logs query strings, so the token reaches the
// access log — and the mitigations live with the endpoint and the page: the write
// calls take it in a body, and the page strips it from the address bar after the
// first read.
func (l *Links) For(userID int64, g Group) (string, error) {
	if l.signer == nil {
		return "", fmt.Errorf("%w: no usable signing secret (JWT_SECRET must be at least %d bytes)", ErrCannotMint, minSecretLen)
	}
	token, err := l.signer.Mint(userID, g)
	if err != nil {
		return "", err
	}
	return l.base + Path + "?t=" + url.QueryEscape(token), nil
}
