// Package telegramnotify is the outbound Telegram channel for filter-subscription
// notifications — the sibling of the inbound internal/telegram crawl. It mints the
// deep-link token that links a user's chat, talks to the Bot API
// (sendMessage/setWebhook), parses inbound webhook updates, and implements
// notify.Notifier by rendering a digest into a Telegram message.
package telegramnotify

import (
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/linktoken"
)

// ErrInvalidToken is returned for a link token that is malformed, has a bad
// signature, or has expired.
var ErrInvalidToken = errors.New("telegramnotify: invalid or expired link token")

// linkTokenPurpose is mixed into the MAC for domain separation, so a token signed
// for another purpose with the same secret cannot pass here.
const linkTokenPurpose = "tg-link"

// LinkTokens mints and verifies the short, stateless token a user carries into the
// bot via the t.me deep link. The encoding is deliberately NOT a JWT: Telegram's
// deep-link `start` parameter allows only 1–64 chars from [A-Za-z0-9_-], which a
// JWT (dotted, ~200 chars) violates. This token is a base64url(payload‖MAC) blob
// of ~43 chars using exactly that alphabet, so it survives the deep link intact.
// The encode/sign/verify logic itself lives in internal/linktoken, shared with
// discordbot.DiscordLinkTokens — only the purpose tag and sentinel error differ.
type LinkTokens struct {
	tokens *linktoken.Tokens
}

// NewLinkTokens returns a LinkTokens signing with secret (reuse JWT_SECRET) and
// expiring each token after ttl (a short window, e.g. 10 minutes).
func NewLinkTokens(secret string, ttl time.Duration) *LinkTokens {
	return &LinkTokens{tokens: linktoken.New(secret, linkTokenPurpose, ttl)}
}

// Issue returns a deep-link token for userID, expiring after the configured TTL.
// The result is base64url (no padding) so it is safe as a Telegram start param.
func (l *LinkTokens) Issue(userID int64) (string, error) {
	return l.tokens.Issue(userID), nil
}

// Parse verifies a token's signature and expiry and returns its user id.
func (l *LinkTokens) Parse(token string) (int64, error) {
	userID, ok := l.tokens.Parse(token)
	if !ok {
		return 0, ErrInvalidToken
	}
	return userID, nil
}
