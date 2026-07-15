package maillink

import (
	"strings"

	"github.com/jaytaylor/html2text"
)

// readableBody picks the text the classifier reads for one email. The plain-text
// part is preferred; when it is absent (many ATS templates are HTML-only) the
// HTML part is stripped to readable text so the LLM sees the actual message body
// rather than only the subject. Length is bounded downstream by the classifier.
func readableBody(text, html string) string {
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
