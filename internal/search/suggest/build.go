package suggest

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Kind says what a suggestion is, and so what picking it does: a title becomes the
// free-text query, everything else applies its own facet.
type Kind string

const (
	KindTitle    Kind = "title"
	KindSkill    Kind = "skill"
	KindCategory Kind = "category"
	KindCompany  Kind = "company"
)

// Document is one offerable suggestion, as it sits in the index.
type Document struct {
	// ID is namespaced by kind: Meilisearch dedupes on it, and `backend` is a plausible
	// role, category and skill slug.
	ID   string `json:"id"`
	Text string `json:"text"`
	Kind Kind   `json:"kind"`
	// Slug is the facet value a pick applies. Empty for a title, which names no facet.
	Slug string `json:"slug,omitempty"`
	// Jobs is the open postings behind it. Never zero — see Build.
	Jobs int `json:"jobs"`
	// Searches is how often visitors have asked for it; the endpoint ranks by it, then Jobs.
	Searches int `json:"searches"`
}

// Company is one employer as the builder receives it, before it becomes a Document.
type Company struct {
	Slug string
	Name string
	Jobs int
}

// Input is everything the dictionary is assembled from: live facet counts, nothing
// measured by the builder.
type Input struct {
	// Titles are raw posting titles with counts; Build normalises and merges them with
	// Title, which the query path shares.
	Titles map[string]int
	// TitleFloor is the merged count a title must reach, applied after normalisation so
	// spelling variants add up.
	TitleFloor int

	Categories map[string]int
	Skills     map[string]int
	Companies  []Company

	// Searches is how often each normalised phrase was searched for, keyed by Title(text).
	// Absent counts as zero, not excluded.
	Searches map[string]int
}

// Build assembles the suggestion dictionary. Nothing with zero postings is offered.
func Build(in Input) []Document {
	var docs []Document
	add := func(kind Kind, slug, text string, jobs int) {
		if jobs <= 0 || text == "" {
			return
		}
		docs = append(docs, Document{
			ID: documentID(kind, slug), Text: text, Kind: kind, Slug: slug, Jobs: jobs,
			Searches: in.Searches[Title(text)],
		})
	}

	// Titles: normalise, merge the spellings, then apply the floor and the craft test.
	merged := make(map[string]int, len(in.Titles))
	for raw, n := range in.Titles {
		if t := Title(raw); t != "" {
			merged[t] += n
		}
	}
	for text, n := range merged {
		// Recordable: is this a search phrase at all (also keeps runaway titles out of
		// ids). Offerable: does it name a craft.
		if n < in.TitleFloor || !Recordable(text) || !Offerable(text) {
			continue
		}
		// A title names no facet: empty slug, id from the normalised text. Display is
		// title-cased.
		docs = append(docs, Document{
			ID: documentID(KindTitle, text), Text: displayTitle(text), Kind: KindTitle, Jobs: n,
			Searches: in.Searches[text],
		})
	}

	for slug, n := range in.Categories {
		add(KindCategory, slug, categoryText(slug), n)
	}

	for slug, n := range in.Skills {
		add(KindSkill, slug, skillText(slug), n)
	}

	for _, c := range in.Companies {
		add(KindCompany, c.Slug, c.Name, c.Jobs)
	}

	// Stable order, so builds diff cleanly.
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	return docs
}

// documentID builds the identifier Meilisearch dedupes on. The engine accepts only
// letters, digits, hyphens and underscores, up to 511 bytes. Values can hold other
// characters (`node.js`, `c++`) and arrive from feeds with unbounded length, so the value
// is hashed to a fixed width. The kind stays in front, unencoded, to namespace the id.
func documentID(kind Kind, value string) string {
	sum := sha256.Sum256([]byte(value))
	return string(kind) + "_" + hex.EncodeToString(sum[:])
}

// categoryText and skillText render a slug as words. Display labels live in the web
// contracts, so the slug's shape is the fallback instead of a second label table.
func categoryText(slug string) string { return titleWords(slug) }
func skillText(slug string) string    { return titleWords(slug) }

func titleWords(slug string) string {
	return capitalise(strings.FieldsFunc(slug, func(r rune) bool { return r == '_' || r == '-' }))
}

// displayTitle renders a normalised posting title. Unlike titleWords it splits on
// whitespace only, so "front-end developer" does not become "Front End Developer".
func displayTitle(title string) string { return capitalise(strings.Fields(title)) }

// capitalise upper-cases each word's first rune only: "node.js" becomes "Node.js", not
// "Node.Js".
func capitalise(words []string) string {
	for i, w := range words {
		r := []rune(w)
		words[i] = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return strings.Join(words, " ")
}
