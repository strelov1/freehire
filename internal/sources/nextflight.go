package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Shared Next.js App Router RSC-flight primitives. A Next.js server-rendered page inlines
// its data as a sequence of self.__next_f.push([1,"…"]) chunks; concatenating and
// JS-string-decoding the chunks yields one flight string that embeds the page's JSON.
// The deel, vouch, and topco adapters read their postings out of this stream.

// fetchFlight fetches a Next.js page and returns its decoded RSC-flight stream — the shared
// opener for the flight adapters (they wrap the error with their own board context).
func fetchFlight(ctx context.Context, c HTMLGetter, url string) (string, error) {
	root, err := c.GetHTML(ctx, url)
	if err != nil {
		return "", err
	}
	return decodeNextFlight(root)
}

// flightArray decodes the JSON array that follows key in the flight stream into a []T. A
// missing array is an error — a markup change must surface loudly rather than silently empty
// the catalogue; an empty array is valid and yields no elements. key includes the trailing
// colon (e.g. `"listings":`).
func flightArray[T any](flight, key string) ([]T, error) {
	raw, ok := bracketSlice(flight, key, '[', ']')
	if !ok {
		return nil, fmt.Errorf("flight array %s not found", key)
	}
	var out []T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode flight array %s: %w", key, err)
	}
	return out, nil
}

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

// nextFlightTextRowMarker matches the start of an RSC text row, "<id>:T<hexlen>,". RSC numbers
// both the row id and the byte length in lowercase HEX (refs run …$29, $2a, $2b, $30…), so the
// id is hex, not decimal. The leading non-hex byte anchors the id's left edge (so "a2a:T" is not
// read as "2a:T") without a lookbehind; a text row is never at offset 0, so requiring one
// preceding byte is safe.
var nextFlightTextRowMarker = regexp.MustCompile(`[^0-9a-f]([0-9a-f]+):T([0-9a-f]+),`)

// nextFlightTextRows returns the flight stream's text rows keyed by id. A text row is
// "<id>:T<hexlen>,<bytes>" whose content is exactly hexlen BYTES (it may itself contain
// newlines or commas), so each marker's content is sliced by its declared byte length. A
// posting's "$N" reference (e.g. a richtext description) resolves to row N's content.
func nextFlightTextRows(flight string) map[string]string {
	b := []byte(flight)
	rows := make(map[string]string)
	for _, m := range nextFlightTextRowMarker.FindAllSubmatchIndex(b, -1) {
		id := string(b[m[2]:m[3]])
		n, err := strconv.ParseInt(string(b[m[4]:m[5]]), 16, 64)
		// Bounding n against math.MaxInt before narrowing keeps int(n) from wrapping on
		// a 32-bit build; end < start below catches start+int(n) itself overflowing.
		if err != nil || n < 0 || n > math.MaxInt {
			continue
		}
		start := m[1] // end of the full match = first content byte after the comma
		end := start + int(n)
		if end < start || end > len(b) {
			end = len(b)
		}
		rows[id] = string(b[start:end])
	}
	return rows
}
