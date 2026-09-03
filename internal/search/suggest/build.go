package suggest

import (
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/dict/roletag"
)

// Kind says what a suggestion IS, which is the only thing that decides what picking it
// does: a title becomes the free-text query (no facet names it), everything else
// applies its own facet.
type Kind string

const (
	KindTitle    Kind = "title"
	KindRole     Kind = "role"
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

	Roles      map[string]int
	RoleLabels map[string]string
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
			ID: string(kind) + ":" + slug, Text: text, Kind: kind, Slug: slug, Jobs: jobs,
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
		if n < in.TitleFloor || !Offerable(text) {
			continue
		}
		// A title names no facet, so its slug is empty and its id is the normalised
		// text — the text IS its identity. The DISPLAY form is title-cased: the
		// lowercase is what merges the spellings, and "product owner" sitting in a
		// dropdown between "Backend Engineer" and "Google" reads as a bug.
		docs = append(docs, Document{
			ID: string(KindTitle) + ":" + text, Text: displayTitle(text), Kind: KindTitle, Jobs: n,
			Searches: in.Searches[text],
		})
	}

	// Roles: one row per base role, keeping the busiest variant of each.
	best := map[string]string{}
	for slug, n := range in.Roles {
		if n <= 0 {
			continue
		}
		base := roletag.BaseRole(slug)
		if cur, ok := best[base]; !ok || in.Roles[cur] < n {
			best[base] = slug
		}
	}
	roleSlugs := map[string]bool{}
	for base, slug := range best {
		roleSlugs[base] = true
		add(KindRole, slug, in.RoleLabels[slug], in.Roles[slug])
	}

	for slug, n := range in.Categories {
		// The de-duplication. A category whose slug a role already carries is those same
		// postings under a department's name.
		if roleSlugs[slug] {
			continue
		}
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
