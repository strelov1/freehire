package emailprefs

import (
	"fmt"
	"net/url"
	"strings"
)

// Path is where a preference link lands. The web app serves it publicly, outside
// the account shell, since it must open without a session.
const Path = "/unsubscribe"

// Links turns a (user, group) into the URL used in a mail's footer and its
// List-Unsubscribe header, so the two always read the same string.
type Links struct {
	signer *Signer
	base   string
}

// minSecretLen mirrors the floor config puts on JWT_SECRET. Without it a worker
// would sign links with the empty string and mail dead unsubscribe links.
const minSecretLen = 32

// NewLinks builds the link maker. secret is the session-signing secret (the salt
// keeps this key apart from it); origin is the frontend origin the mail links into.
//
// A too-short secret is kept, not rejected, but every For call then fails, so the
// mail is retried rather than sent with a dead link. A dead unsubscribe link is
// worse than a late mail.
func NewLinks(secret, origin string) *Links {
	l := &Links{base: strings.TrimRight(origin, "/")}
	if len(secret) >= minSecretLen {
		l.signer = NewSigner(secret)
	}
	return l
}

// TextFooter is the unsubscribe line a plain-text body ends with.
//
// One author for every sender: text-only readers have the same right to leave as
// HTML readers, and spam scorers read both. It also says who is writing and why,
// as the HTML shell's footer does, since SES Trust & Safety found our mail hard to
// attribute.
func TextFooter(unsubscribeURL string) string {
	return "\n--\nThis email is from freehire, the job search service. You get it because you have a freehire account and these notifications are on.\nUnsubscribe: " + unsubscribeURL + "\n"
}

// For returns the preference URL for one recipient and one group of mail. It fails
// for a group nobody may turn off, so an essential mail never gets a link it cannot
// honour.
//
// The token rides in the query, so nginx logs it; mitigations: write calls take it
// in a body and the page strips it from the address bar after the first read.
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
