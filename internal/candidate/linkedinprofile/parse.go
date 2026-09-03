package linkedinprofile

import (
	"encoding/json"
	"errors"
	"regexp"
)

// ErrNoProfile means the page did not yield a member profile: it carried no JSON-LD,
// the JSON-LD did not parse, it held no Person node, or every field of the Person it
// held was withheld. These are one outcome on purpose — from the reader's side they
// are all "LinkedIn did not give us this profile", and there is nothing a caller
// could usefully do differently for each.
var ErrNoProfile = errors.New("linkedinprofile: no readable profile on the page")

// Profile is what a public LinkedIn member page releases to an anonymous reader,
// after the withheld runs have been dropped. Every field may be empty — LinkedIn
// masks per field, not per page — and no field ever carries a masked run.
//
// There is deliberately nothing here about employment history. LinkedIn withholds
// every job title and every position description from an anonymous reader, so the
// absence is in the source, not in this parser.
type Profile struct {
	// Name is the member's display name.
	Name string
	// Headline is the one-line summary under the name, conventionally
	// "level + role + stack". LinkedIn serves it truncated with an ellipsis, and it
	// is passed on exactly as served.
	Headline string
	// Location is the profile's stated locality, as one human-readable line
	// ("Florianópolis, Santa Catarina, Brazil").
	Location string
	// Company is the current employer — the first employer entry LinkedIn left
	// readable, which in practice is the only one.
	Company string
	// Languages are the languages the member lists.
	Languages []string
}

// empty reports whether nothing at all survived. A Person node with every field
// withheld is not a sparse profile, it is a profile we were not given, and saying
// so lets the caller tell the user that instead of showing them an empty form.
func (p Profile) empty() bool {
	return p.Name == "" && p.Headline == "" && p.Location == "" && p.Company == "" && len(p.Languages) == 0
}

// ldBlock matches a JSON-LD script element. A profile page carries one today; every
// match is read so that a page which grows a second block does not start failing.
var ldBlock = regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)

// ldNode is the slice of schema.org/Person this reader needs. Unknown members are
// ignored; the page carries a great deal more (post listings, follower counts, image
// URLs) and none of it is this package's business.
type ldNode struct {
	Type    string `json:"@type"`
	Name    string `json:"name"`
	Address struct {
		AddressLocality string `json:"addressLocality"`
	} `json:"address"`
	KnowsLanguage []struct {
		Name string `json:"name"`
	} `json:"knowsLanguage"`
	WorksFor []struct {
		Name string `json:"name"`
	} `json:"worksFor"`
	Description string `json:"description"`
}

// ldEnvelope is the outer document. LinkedIn serves the Person inside an @graph
// alongside decoy nodes (post listings, the WebPage), but a bare Person document is
// also valid JSON-LD, so both shapes are read.
type ldEnvelope struct {
	Graph []json.RawMessage `json:"@graph"`
}

// Parse reads a fetched profile page and returns what LinkedIn released.
//
// The Person is taken only from the top level of the document or its @graph, never
// from inside another node: the decoy nodes on a real page include posts whose
// author is itself a Person, and descending into them would return a stranger.
func Parse(page []byte) (Profile, error) {
	for _, block := range ldBlock.FindAllSubmatch(page, -1) {
		doc := block[1]

		var env ldEnvelope
		if err := json.Unmarshal(doc, &env); err != nil {
			continue
		}

		// The Person is either one of the @graph's top-level nodes or, on a bare
		// Person document, the document itself. Nothing deeper is a candidate.
		candidates := append(env.Graph, json.RawMessage(doc))

		for _, raw := range candidates {
			var n ldNode
			if err := json.Unmarshal(raw, &n); err != nil || n.Type != "Person" {
				continue
			}
			if p := profileFrom(n); !p.empty() {
				return p, nil
			}
		}
	}
	return Profile{}, ErrNoProfile
}

// profileFrom lifts the node's fields, every one of them through value, so a masked
// run cannot leave this package regardless of which field carried it.
func profileFrom(n ldNode) Profile {
	p := Profile{
		Name:     value(n.Name),
		Headline: value(n.Description),
		Location: value(n.Address.AddressLocality),
	}
	// The employer list arrives newest-first with everything past the current role
	// withheld, so this takes the first entry that says anything rather than assuming
	// the first entry is readable.
	for _, w := range n.WorksFor {
		if name := value(w.Name); name != "" {
			p.Company = name
			break
		}
	}
	for _, l := range n.KnowsLanguage {
		if name := value(l.Name); name != "" {
			p.Languages = append(p.Languages, name)
		}
	}
	return p
}
