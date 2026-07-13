// Package gmailsync holds the pure, I/O-free helpers for the Gmail ATS inbox:
// subject normalization (the inbox grouping key), the curated ATS sender-domain
// list, and the Gmail search-query builder. Kept free of Google/DB dependencies
// so it is fully table-testable.
package gmailsync

import (
	"regexp"
	"strings"
)

// replyPrefix matches one or more leading reply/forward markers (English +
// common locale forms), case-insensitively, so "Re: Fwd: X" and "AW: X"
// normalize to the same base subject.
var replyPrefix = regexp.MustCompile(`(?i)^\s*((re|fwd|fw|aw|antw|sv|vs)\s*:\s*)+`)

// whitespace collapses any run of spaces/tabs/newlines to a single space.
var whitespace = regexp.MustCompile(`\s+`)

// NormalizeSubject reduces an email subject to its grouping key: leading
// reply/forward prefixes stripped, whitespace collapsed, trimmed, lowercased.
// The result groups a conversation ("X" and "Re: X") under one key.
func NormalizeSubject(subject string) string {
	s := replyPrefix.ReplaceAllString(subject, "")
	s = whitespace.ReplaceAllString(s, " ")
	return strings.ToLower(strings.TrimSpace(s))
}
