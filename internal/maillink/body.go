package maillink

import (
	"strings"

	"github.com/jaytaylor/html2text"
)

// ReadableBody picks the text a classifier reads for one email. The plain-text
// part is preferred; when it is absent (many ATS templates are HTML-only) the
// HTML part is stripped to readable text so the classifier sees the actual
// message body rather than only the subject. Length is bounded downstream.
//
// Exported because the agent inbox listing serves the same text to a caller's own
// agent, and the two must not drift: what our worker classifies and what a
// harness classifies has to be the same body.
func ReadableBody(text, html string) string {
	if strings.TrimSpace(text) != "" {
		return text
	}
	if html == "" {
		return ""
	}
	stripped, err := html2text.FromString(html, html2text.Options{OmitLinks: false})
	if err != nil {
		return html // last resort: text among tags beats an empty body
	}
	return stripped
}
