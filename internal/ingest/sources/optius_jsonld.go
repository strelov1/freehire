package sources

import (
	"encoding/json"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/flexjson"
)

// Optius inserts employer text into these JSON string fields without escaping quotation
// marks. Bound the repair to those fields, delimited by the following property, and require
// the entire JobPosting to decode before trusting any of it.
var optiusBodyField = regexp.MustCompile(`(?s)("(?:description|responsibilities|jobBenefits)"\s*:\s*")(.*?)("\s*(?:,\s*"(?:[^"\\]|\\.)*"\s*:|}))`)

func optiusJobPosting(root *html.Node, posting *optiusLD) bool {
	if ldJobPosting(root, posting) {
		return true
	}
	found := false
	walk(root, func(n *html.Node) bool {
		if found || n.Type != html.ElementNode || n.Data != "script" || Attr(n, "type") != "application/ld+json" {
			return !found
		}
		raw := textContent(n)
		var repaired strings.Builder
		offset := 0
		for {
			match := optiusBodyField.FindStringSubmatchIndex(raw[offset:])
			if match == nil {
				break
			}
			start, end := offset+match[4], offset+match[5]
			repaired.WriteString(raw[offset:start])
			escaped := false
			for _, c := range raw[start:end] {
				if c == '"' && !escaped {
					repaired.WriteByte('\\')
				}
				repaired.WriteRune(c)
				if c == '\\' {
					escaped = !escaped
				} else {
					escaped = false
				}
			}
			// Resume before the boundary: the following property can itself need repair.
			offset = end
		}
		repaired.WriteString(raw[offset:])
		msg, ok := jobPostingNode(flexjson.SanitizeControlChars([]byte(repaired.String())))
		if ok && json.Unmarshal(msg, posting) == nil {
			found = true
		}
		return !found
	})
	return found
}
