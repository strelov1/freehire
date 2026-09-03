package linkedinprofile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/flexjson"
)

// ErrNoProfile means the page did not yield a member profile: it carried no JSON-LD, the
// JSON-LD held no Person node, or every field of the Person it held was withheld. They are
// one sentinel because a caller does the same thing for all three — tell the user the
// profile could not be read. They carry different messages because an operator does not:
// "no JSON-LD block" on a 200 is what an authwall looks like, and a run of those is
// LinkedIn shutting us out, not users pasting private profiles.
var ErrNoProfile = errors.New("linkedinprofile: no readable profile on the page")

// Profile is what a public LinkedIn member page releases to an anonymous reader, after the
// withheld runs have been dropped. Every field may be empty — LinkedIn masks per field, not
// per page — and no field ever carries a masked run.
//
// There is deliberately nothing here about employment history. LinkedIn withholds every job
// title and every position description from an anonymous reader, so the absence is in the
// source, not in this parser.
type Profile struct {
	// Name is the member's display name.
	Name string
	// Headline is the one-line summary under the name, conventionally
	// "level + role + stack". LinkedIn serves it truncated with an ellipsis, and it is
	// passed on exactly as served.
	Headline string
	// Location is the profile's stated locality, as one human-readable line
	// ("Florianópolis, Santa Catarina, Brazil").
	Location string
	// Company is the current employer — the first employer entry LinkedIn left readable,
	// which in practice is the only one.
	Company string
}

// Nothing here carries the member's listed languages, though LinkedIn does release them.
// The wizard this feeds does not collect languages, so a field for them would be parsed,
// carried through the response, and read by nobody — while the API documentation promised
// it. When a surface actually wants them, lifting them back is four lines.

// empty reports whether nothing at all survived. A Person node with every field withheld is
// not a sparse profile, it is a profile we were not given, and saying so lets the caller tell
// the user that instead of showing them an empty form.
func (p Profile) empty() bool {
	return p.Name == "" && p.Headline == "" && p.Location == "" && p.Company == ""
}

// parse reads a fetched profile page and returns what LinkedIn released for the member with
// the given public id.
//
// wantID is how the owner is told apart from anyone else the page mentions. A profile page
// carries other Person nodes — a post's author today, a recommender or a "people also viewed"
// entry tomorrow — and returning one of those would show a stranger's name and headline to
// the user as their own. When no node claims the id (LinkedIn does not always carry a url on
// the node), the first readable Person is used, which is the shape observed in practice.
func parse(page []byte, wantID string) (Profile, error) {
	root, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return Profile{}, fmt.Errorf("%w: page is not parseable HTML", ErrNoProfile)
	}

	var people []ldNode
	blocks := 0
	forEachLDBlock(root, func(raw []byte) {
		blocks++
		for _, n := range nodesIn(raw) {
			if n.isType("Person") {
				people = append(people, n)
			}
		}
	})

	switch {
	case blocks == 0:
		// A 200 with no JSON-LD at all is what LinkedIn's sign-in wall looks like.
		return Profile{}, fmt.Errorf("%w: no JSON-LD block", ErrNoProfile)
	case len(people) == 0:
		return Profile{}, fmt.Errorf("%w: no Person node", ErrNoProfile)
	}

	if p, ok := profileOwnedBy(people, wantID); ok {
		return p, nil
	}
	return Profile{}, fmt.Errorf("%w: every field withheld", ErrNoProfile)
}

// profileOwnedBy prefers the node that names wantID, and falls back to the first readable
// one. A node that claims the id but released nothing is still the owner's, so the fallback
// does not then go and return a stranger's.
func profileOwnedBy(people []ldNode, wantID string) (Profile, bool) {
	for _, n := range people {
		if n.claims(wantID) {
			p := profileFrom(n)
			return p, !p.empty()
		}
	}
	for _, n := range people {
		if p := profileFrom(n); !p.empty() {
			return p, true
		}
	}
	return Profile{}, false
}

// profileFrom lifts the node's fields, every one of them through value, so a masked run
// cannot leave this package regardless of which field carried it.
func profileFrom(n ldNode) Profile {
	p := Profile{
		Name:     value(n.text("name")),
		Headline: value(n.text("description")),
		Location: value(n.locality()),
	}
	// The employer list arrives newest-first with everything past the current role
	// withheld, so this takes the first entry that says anything rather than assuming the
	// first entry is readable.
	for _, w := range n.list("worksFor") {
		if name := value(nameOf(w)); name != "" {
			p.Company = name
			break
		}
	}
	return p
}

// ldNode is one JSON-LD node held as raw members rather than decoded into a struct.
//
// That is the whole point. encoding/json abandons a struct decode on the first member whose
// shape it dislikes, and schema.org lets a publisher write "@type" as a string or a
// one-element array, an entity as a bare name or an object, and a list as a single item.
// Decoding into a struct means one such choice in knowsLanguage — a field nothing downstream
// reads — takes the name, the headline and the location down with it, and reports the profile
// as unreadable. Holding the members raw makes the promise in Profile's doc ("LinkedIn masks
// per field, not per page") true of our parsing as well as of LinkedIn's serving.
type ldNode map[string]json.RawMessage

// isType reports whether the node declares the given @type, which schema.org allows to be a
// single string or a list of them.
func (n ldNode) isType(want string) bool {
	for _, t := range n.list("@type") {
		if strings.EqualFold(textOf(t), want) {
			return true
		}
	}
	return false
}

// claims reports whether the node names the given public id in the URLs it carries.
func (n ldNode) claims(id string) bool {
	if id == "" {
		return false
	}
	suffix := "/in/" + strings.ToLower(id)
	for _, key := range []string{"url", "sameAs"} {
		for _, raw := range n.list(key) {
			if u := strings.ToLower(strings.TrimSuffix(textOf(raw), "/")); strings.HasSuffix(u, suffix) {
				return true
			}
		}
	}
	return false
}

// text reads a member as a plain string.
func (n ldNode) text(key string) string { return textOf(n[key]) }

// list reads a member as a sequence, whether it was written as one or as a single item.
func (n ldNode) list(key string) []json.RawMessage { return itemsOf(n[key]) }

// locality reads the postal address, which schema.org permits as an object or as a plain
// string.
func (n ldNode) locality() string {
	raw, ok := n["address"]
	if !ok {
		return ""
	}
	if addr, ok := decodeNode(raw); ok {
		return addr.text("addressLocality")
	}
	return textOf(raw)
}

// textOf reads a value that may be a string or a list of them, taking the first.
func textOf(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return textOf(list[0])
	}
	return ""
}

// itemsOf normalises a value that may be a list or a single item into a list.
func itemsOf(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	return []json.RawMessage{raw}
}

// nameOf reads an entity written either as a bare name or as an object carrying one.
func nameOf(raw json.RawMessage) string {
	if n, ok := decodeNode(raw); ok {
		return n.text("name")
	}
	return textOf(raw)
}

func decodeNode(raw json.RawMessage) (ldNode, bool) {
	var n ldNode
	if len(raw) == 0 || json.Unmarshal(raw, &n) != nil {
		return nil, false
	}
	return n, true
}

// nodesIn returns the top-level nodes of one JSON-LD block: the members of an @graph, the
// elements of a bare array, or the document itself. Nothing nested inside a node is a
// candidate — a post listing carries its author as a Person, and descending into one would
// return a stranger.
func nodesIn(raw []byte) []ldNode {
	// Raw control bytes inside a string literal are invalid JSON that Go's decoder
	// rejects outright, where a lenient one would not. Escaping them first is what keeps
	// a stray newline in a headline from costing the whole profile.
	block := flexjson.SanitizeControlChars(raw)

	doc, ok := decodeNode(block)
	if !ok {
		// A block that is not an object may still be an array of nodes.
		var list []json.RawMessage
		if json.Unmarshal(block, &list) != nil {
			return nil
		}
		return decodeNodes(list)
	}
	if graph, ok := doc["@graph"]; ok {
		return decodeNodes(itemsOf(graph))
	}
	return []ldNode{doc}
}

func decodeNodes(list []json.RawMessage) []ldNode {
	out := make([]ldNode, 0, len(list))
	for _, raw := range list {
		if n, ok := decodeNode(raw); ok {
			out = append(out, n)
		}
	}
	return out
}

// forEachLDBlock calls fn with the contents of every application/ld+json script element.
// The type is compared after dropping any parameters and without regard to case or quoting,
// so `type=application/ld+json` and `type="application/ld+json; charset=utf-8"` are both
// read — both are ordinary HTML that a parser handles for free.
//
// internal/ingest/sources walks for the same element, and this is deliberately not shared
// with it. SanitizeControlChars was worth hoisting because it carries KNOWLEDGE — a
// live-verified fact about how real pages break — and two copies of that would drift into
// two answers. A DOM walk carries none: it is standard-library idiom with nothing to
// disagree about, and hoisting it would mean editing four working adapters to gain
// twenty shared lines.
func forEachLDBlock(root *html.Node, fn func(raw []byte)) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" && isLDType(attr(n, "type")) {
			fn([]byte(textContent(n)))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
}

func isLDType(v string) bool {
	mediaType, _, _ := strings.Cut(v, ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), "application/ld+json")
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
