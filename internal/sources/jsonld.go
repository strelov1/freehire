package sources

import (
	"bytes"
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// LDJobPosting is the exported form of ldJobPosting, for sibling packages that parse a
// server-rendered detail page into a sources.Job (e.g. internal/linksource).
func LDJobPosting(root *html.Node, v any) bool { return ldJobPosting(root, v) }

// ldJobPosting decodes the first application/ld+json JobPosting block on the page into v,
// returning false when the page carries no such block. Shared by the HTML detail adapters
// (teamtailor, breezy) whose job pages server-render a schema.org JobPosting; each passes
// a struct selecting just the fields it needs. The JobPosting may sit at the block's top
// level, inside an array of nodes, or under a @graph wrapper (see jobPostingNode).
func ldJobPosting(root *html.Node, v any) bool {
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "script" &&
			attr(n, "type") == "application/ld+json" {
			if msg, ok := jobPostingNode(sanitizeJSONControlChars([]byte(textContent(n)))); ok &&
				json.Unmarshal(msg, v) == nil {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// LDJobPostings decodes EVERY schema.org JobPosting block on the page, for listing pages that
// inline multiple postings as separate ld+json blocks (e.g. a static careers page that renders
// each vacancy's JobPosting alongside it). Unlike ldJobPosting (first block only) it walks all
// ld+json scripts and returns each posting's raw JSON so a multi-job adapter maps every one.
func LDJobPostings(root *html.Node) []json.RawMessage {
	var out []json.RawMessage
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "script" &&
			attr(n, "type") == "application/ld+json" {
			out = append(out, jobPostingNodes(sanitizeJSONControlChars([]byte(textContent(n))))...)
		}
		return true
	})
	return out
}

// jobPostingNode finds the first schema.org JobPosting node inside one ld+json block, whatever
// its top-level shape (see jobPostingNodes). It returns that node's raw JSON so the caller
// decodes just the fields it needs, or ok=false when the block carries no JobPosting.
func jobPostingNode(raw []byte) (json.RawMessage, bool) {
	nodes := jobPostingNodes(raw)
	if len(nodes) == 0 {
		return nil, false
	}
	return nodes[0], true
}

// jobPostingNodes returns every schema.org JobPosting node inside one ld+json block, whatever its
// top-level shape: a bare JobPosting object, an array of nodes ([{...}, ...], as Langford emits),
// or a graph wrapper ({"@graph":[...]}). Most pages carry one, but a listing block may inline
// several — so this returns all and jobPostingNode takes the first.
func jobPostingNodes(raw []byte) []json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '[' {
		var nodes []json.RawMessage
		if json.Unmarshal(trimmed, &nodes) != nil {
			return nil
		}
		return filterJobPostings(nodes)
	}
	if isJobPosting(trimmed) {
		return []json.RawMessage{trimmed}
	}
	// Not a bare JobPosting object → it may be a @graph wrapper listing the page's nodes.
	var wrapper struct {
		Graph []json.RawMessage `json:"@graph"`
	}
	if json.Unmarshal(trimmed, &wrapper) == nil {
		return filterJobPostings(wrapper.Graph)
	}
	return nil
}

// filterJobPostings returns the JobPosting nodes among a list of ld+json nodes, in order.
func filterJobPostings(nodes []json.RawMessage) []json.RawMessage {
	var out []json.RawMessage
	for _, n := range nodes {
		if isJobPosting(n) {
			out = append(out, n)
		}
	}
	return out
}

// isJobPosting reports whether a raw ld+json node is a schema.org JobPosting.
func isJobPosting(raw json.RawMessage) bool {
	var probe struct {
		Type string `json:"@type"`
	}
	return json.Unmarshal(raw, &probe) == nil && strings.EqualFold(probe.Type, "JobPosting")
}

// jsonControlCharEscapes maps a raw control byte to the two-byte escape json.Unmarshal
// accepts in its place; a byte absent here (rare outside \n/\r/\t) falls back to \u00XX.
var jsonControlCharEscapes = map[byte]string{'\n': `\n`, '\r': `\r`, '\t': `\t`}

const hexDigits = "0123456789abcdef"

// sanitizeJSONControlChars escapes raw control bytes (0x00-0x1F) found INSIDE a JSON string
// literal. Some sites template-render their JobPosting ld+json by interpolating raw HTML into
// a string without escaping its newlines, which is invalid JSON — Go's decoder rejects it
// ("invalid character '\n' in string literal") where a lenient decoder (e.g. Python's
// json.loads(strict=False)) would accept it; live-verified on cryptocurrencyjobs.co. Control
// bytes outside a string (insignificant whitespace between tokens) are left untouched.
func sanitizeJSONControlChars(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString, escaped := false, false
	for _, b := range raw {
		switch {
		case !inString:
			if b == '"' {
				inString = true
			}
			out = append(out, b)
		case escaped:
			escaped = false
			out = append(out, b)
		case b == '\\':
			escaped = true
			out = append(out, b)
		case b == '"':
			inString = false
			out = append(out, b)
		case b < 0x20:
			if esc, ok := jsonControlCharEscapes[b]; ok {
				out = append(out, esc...)
			} else {
				out = append(out, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
			}
		default:
			out = append(out, b)
		}
	}
	return out
}
