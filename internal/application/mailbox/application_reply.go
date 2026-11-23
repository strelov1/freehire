package mailbox

import (
	"strings"

	"github.com/google/uuid"
)

// The full local part is longer than a hosted mailbox username.
const applicationReplyPrefix = "ats-reply-"

// ApplicationReplyAddress returns the hosted envelope address for one outgoing email.
func ApplicationReplyAddress(token uuid.UUID, domain string) string {
	return applicationReplyPrefix + token.String() + "@" + domain
}

// ApplicationReplyToken recognizes only the canonical alias on the hosted domain.
// The caller has already selected and normalized a recipient from the SMTP envelope.
func ApplicationReplyToken(address string) (uuid.UUID, bool) {
	local, _, hasDomain := strings.Cut(address, "@")
	value, matchesPrefix := strings.CutPrefix(local, applicationReplyPrefix)
	if !hasDomain || !matchesPrefix {
		return uuid.Nil, false
	}
	token, err := uuid.Parse(value)
	return token, err == nil && token != uuid.Nil && token.String() == value
}
