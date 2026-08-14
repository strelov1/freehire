package discordbot

import (
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/linktoken"
)

// ErrInvalidToken is returned for a link token that is malformed, has a bad
// signature, or has expired.
var ErrInvalidToken = errors.New("discordbot: invalid or expired link token")

// DiscordLinkTTL bounds how long a /link deep-link token is valid — long
// enough for a user to copy it from the web app into Discord, short enough
// that a leaked token is not useful for long.
const DiscordLinkTTL = 10 * time.Minute

// linkTokenPurpose is mixed into the MAC for domain separation, so a token
// signed for another purpose with the same secret cannot pass here.
const linkTokenPurpose = "discord-link"

// DiscordLinkTokens mints and verifies the short, stateless token a user
// carries from the web app into the bot's /link slash command: a
// base64url(payload‖MAC) blob, not a JWT — no server-side store needed, and
// it stays short enough to paste as a command argument. The encode/sign/verify
// logic itself lives in internal/linktoken, shared with
// telegramnotify.LinkTokens — only the purpose tag and sentinel error differ.
type DiscordLinkTokens struct {
	tokens *linktoken.Tokens
}

// NewDiscordLinkTokens returns a DiscordLinkTokens signing with secret (reuse
// JWT_SECRET) and expiring each token after ttl.
func NewDiscordLinkTokens(secret string, ttl time.Duration) *DiscordLinkTokens {
	return &DiscordLinkTokens{tokens: linktoken.New(secret, linkTokenPurpose, ttl)}
}

// Issue returns a /link token for userID, expiring after the configured TTL.
func (l *DiscordLinkTokens) Issue(userID int64) (string, error) {
	return l.tokens.Issue(userID), nil
}

// Parse verifies a token's signature and expiry and returns its user id.
func (l *DiscordLinkTokens) Parse(token string) (int64, error) {
	userID, ok := l.tokens.Parse(token)
	if !ok {
		return 0, ErrInvalidToken
	}
	return userID, nil
}
