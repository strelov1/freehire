package sources

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Shared Next.js App Router RSC-flight primitives. A Next.js server-rendered page inlines
// its data as a sequence of self.__next_f.push([1,"…"]) chunks; concatenating and
// JS-string-decoding the chunks yields one flight string that embeds the page's JSON.
// The deel and vouch adapters both read their postings out of this stream.

// nextFlightPush captures the JS-string body of one self.__next_f.push([1,"…"]) flight
// chunk (the init push, push([0]), carries no [1,"…"] payload and is ignored). The capture
// keeps the surrounding quotes so it decodes as a JSON string.
var nextFlightPush = regexp.MustCompile(`self\.__next_f\.push\(\[1,("(?:[^"\\]|\\.)*")\]\)`)

// decodeNextFlight concatenates and JS-string-decodes every flight chunk embedded in the
// page's <script> tags, returning the assembled flight stream (or an error when the page
// carries no flight payload — a markup change must surface loudly).
func decodeNextFlight(root *html.Node) (string, error) {
	var scripts strings.Builder
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "script" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					scripts.WriteString(c.Data)
				}
			}
		}
		return true
	})
	matches := nextFlightPush.FindAllStringSubmatch(scripts.String(), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no flight payload found")
	}
	var flight strings.Builder
	for _, m := range matches {
		var chunk string
		if err := json.Unmarshal([]byte(m[1]), &chunk); err != nil {
			return "", fmt.Errorf("decode flight chunk: %w", err)
		}
		flight.WriteString(chunk)
	}
	return flight.String(), nil
}

// bracketSlice returns the balanced open..close run that follows the first occurrence of
// key in s (e.g. the `[ … ]` array or `{ … }` object after a JSON field name), or ok=false
// when key is absent. It counts depth only outside JSON string literals, so a bracket inside
// a value — a tenant-controlled title like "[EMEA] Engineer" — does not unbalance the scan.
func bracketSlice(s, key string, open, closing byte) (string, bool) {
	at := strings.Index(s, key)
	if at < 0 {
		return "", false
	}
	start := strings.IndexByte(s[at:], open)
	if start < 0 {
		return "", false
	}
	start += at
	depth, inString := 0, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch c {
			case '\\':
				i++ // skip the escaped byte
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}
