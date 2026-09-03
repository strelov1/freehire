package suggest

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Kind says what a suggestion IS, which is the only thing that decides what picking it
// does: a title becomes the free-text query (no facet names it), everything else
// applies its own facet.
type Kind string

const (
	KindTitle    Kind = "title"
	KindSkill    Kind = "skill"
	KindCategory Kind = "category"
	KindCompany  Kind = "company"
)

// Document is one offerable suggestion, as it sits in the index.
type Document struct {
	// ID is namespaced by kind. `backend` is a plausible role slug AND a plausible
	// category slug AND a plausible skill, and Meilisearch dedupes on this — an
	// unnamespaced id would silently keep whichever kind was written last.
	ID   string `json:"id"`
	Text string `json:"text"`
	Kind Kind   `json:"kind"`
	// Slug is the facet value a pick applies. Empty for a title, which names no facet.
	Slug string `json:"slug,omitempty"`
	// Jobs is the open postings behind it. Never zero — see Build.
	Jobs int `json:"jobs"`
	// Searches is how often visitors have asked for it. Zero until the frequency
	// record exists; the endpoint ranks by it first, then by Jobs.
	Searches int `json:"searches"`
}

// Company is one employer as the builder receives it, before it becomes a Document.
type Company struct {
	Slug string
	Name string
	Jobs int
}

// Input is everything the dictionary is assembled from. The counts are the live facet
// distributions; the builder does not measure anything itself.
type Input struct {
	// Titles are RAW posting titles with their counts, straight from the catalogue.
	// Normalisation and merging happen here rather than in the query that produced
	// them, because Title is one function shared with the query path.
	Titles map[string]int
	// TitleFloor is the merged count a title must reach. Applied AFTER normalisation:
	// "Product Owner", "product owner" and "PRODUCT OWNER" are three rows and one
	// suggestion, so a floor applied before merging drops a title that clears it.
	TitleFloor int

	Categories map[string]int
	Skills     map[string]int
	Companies  []Company

	// Searches is how often each NORMALISED phrase has been searched for. Keyed by
	// Title(text), which is why a typed query and the title it names meet here at all.
	// Absent is zero rather than excluded: nobody having asked for a suggestion is not
	// evidence against it, and the posting count still orders it.
	Searches map[string]int
}

// Build assembles the suggestion dictionary.
//
// Two rules shape it beyond the obvious, and both come from measuring the live
// catalogue rather than from taste:
//
//   - A bare-category role and its category select the SAME postings — role `devops`
//     counts 53,250 against category `devops` at 53,251. Offering both puts one filter
//     in the dropdown twice, which is the confusion this whole feature exists to
//     remove. The role wins: "DevOps Engineer" names a job, "DevOps" names a
//     department.
//   - The catalogue carries every seniority grade as its own role slug, and graded
//     slugs outnumber ungraded ones roughly six to one, so offering each would spend a
//     whole dropdown on one role's grades.
//
// Nothing measured at zero is offered at all: a suggestion with no postings behind it
// leads to an empty page, which is worse than no suggestion.
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
		// Two gates, and they refuse different things. `Recordable` asks whether this is
		// a search PHRASE at all — the same question the demand path asks of typed text,
		// and the answer that keeps a runaway title out of an id (hex doubles the value,
		// so the engine's 511-byte ceiling is a ~255-byte one here). `Offerable` asks
		// whether the phrase names a craft.
		if n < in.TitleFloor || !Recordable(text) || !Offerable(text) {
			continue
		}
		// A title names no facet, so its slug is empty and its id is the normalised
		// text — the text IS its identity. The DISPLAY form is title-cased: the
		// lowercase is what merges the spellings, and "product owner" sitting in a
		// dropdown between "Backend Engineer" and "Google" reads as a bug.
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

	// Stable order so a rebuild that changed nothing writes the same documents in the
	// same sequence — which is what makes a diff of two builds readable.
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	return docs
}

// documentID builds the identifier Meilisearch dedupes on.
//
// The engine accepts letters, digits, hyphens and underscores and nothing else, up to
// 511 bytes, and two production builds failed here in turn:
//
//   - `company:01-tech` — the readable separator was an illegal character, and so are
//     characters the VALUES legitimately hold (`node.js`, `c++`, `at&t`), so no
//     prefix-plus-value scheme survives this dictionary's own vocabulary;
//   - then a 340-byte company slug — a transliterated college name — which the hex
//     encoding that fixed the first failure doubled past the ceiling.
//
// So: a fixed-width hash. Legal by construction, and its length does not follow the
// value at all — which is the property the second failure was really about. A slug
// arrives from a feed and nobody promised its length; bounding the values we mine
// ourselves could never have covered that.
//
// The kind stays in front, unencoded, because it is a closed vocabulary of plain
// words: `backend` is a plausible role, skill AND category, and the whole reason an id
// is namespaced is that those three must not collide.
//
// Readability in the engine's own UI is the cost. The document carries `kind`, `slug`
// and `text` as fields, so nothing that has to be looked up is only in the id.
func documentID(kind Kind, value string) string {
	sum := sha256.Sum256([]byte(value))
	return string(kind) + "_" + hex.EncodeToString(sum[:])
}

// categoryText and skillText render a slug as a person would write it. Neither
// dictionary ships a display label to Go — the web contracts carry those — so the
// slug's own shape is the honest fallback rather than a second label table that would
// drift from the one the filter modal renders.
func categoryText(slug string) string { return titleWords(slug) }
func skillText(slug string) string    { return titleWords(slug) }

func titleWords(slug string) string {
	return capitalise(strings.FieldsFunc(slug, func(r rune) bool { return r == '_' || r == '-' }))
}

// displayTitle renders a normalised posting title the way it is written. It splits on
// WHITESPACE only, unlike titleWords: a hyphen inside a mined title is part of the
// name, so splitting there turns "front-end developer" into "Front End Developer".
func displayTitle(title string) string { return capitalise(strings.Fields(title)) }

// capitalise upper-cases each word's first rune and leaves the rest alone. Leaving the
// rest alone is the point: "c#" becomes "C#" and "node.js" becomes "Node.js", where
// title-casing the whole word would produce "Node.Js".
func capitalise(words []string) string {
	for i, w := range words {
		r := []rune(w)
		words[i] = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return strings.Join(words, " ")
}
