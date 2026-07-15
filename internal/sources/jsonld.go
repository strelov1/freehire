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
			if msg, ok := jobPostingNode([]byte(textContent(n))); ok &&
				json.Unmarshal(msg, v) == nil {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// jobPostingNode finds the schema.org JobPosting node inside one ld+json block, whatever its
// top-level shape: a bare JobPosting object, an array of nodes ([{...}, ...], as Langford
// emits), or a graph wrapper ({"@graph":[...]}). It returns that node's raw JSON so the caller
// decodes just the fields it needs, or ok=false when the block carries no JobPosting.
func jobPostingNode(raw []byte) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false
	}
	if trimmed[0] == '[' {
		var nodes []json.RawMessage
		if json.Unmarshal(trimmed, &nodes) != nil {
			return nil, false
		}
		return firstJobPosting(nodes)
	}
	if isJobPosting(trimmed) {
		return trimmed, true
	}
	// Not a bare JobPosting object → it may be a @graph wrapper listing the page's nodes.
	var wrapper struct {
		Graph []json.RawMessage `json:"@graph"`
	}
	if json.Unmarshal(trimmed, &wrapper) == nil {
		return firstJobPosting(wrapper.Graph)
	}
	return nil, false
}

// firstJobPosting returns the first JobPosting node among a list of ld+json nodes.
func firstJobPosting(nodes []json.RawMessage) (json.RawMessage, bool) {
	for _, n := range nodes {
		if isJobPosting(n) {
			return n, true
		}
	}
	return nil, false
}

// isJobPosting reports whether a raw ld+json node is a schema.org JobPosting.
func isJobPosting(raw json.RawMessage) bool {
	var probe struct {
		Type string `json:"@type"`
	}
	return json.Unmarshal(raw, &probe) == nil && strings.EqualFold(probe.Type, "JobPosting")
}
