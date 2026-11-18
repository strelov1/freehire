package suggest

import (
	"sort"
	"strings"
	"sync"
)

// Part is one recognised piece of a query: a whole phrase and the facet it applies. A
// suggestion is the recognised parts plus one candidate for the fragment.
type Part struct {
	Kind Kind   `json:"kind"`
	Slug string `json:"slug,omitempty"`
	Text string `json:"text"`
}

// Parsed splits a query into what the dictionary knows and what is still being typed.
type Parsed struct {
	// Recognised are the whole phrases consumed from the front of the query, in order.
	Recognised []Part
	// Fragment is what the phrases did not consume, the part to complete.
	Fragment string
}

// singular are the kinds a query may name only once: a second role or employer would
// match nothing. Skills are absent because several skills narrow a search sensibly.
var singular = map[Kind]bool{
	KindCompany:  true,
	KindCategory: true,
	KindTitle:    true,
}

// ExcludedKinds reports the kinds the recognised prefix has already filled, so the
// completion query can leave them out.
func (p Parsed) ExcludedKinds() []Kind {
	var out []Kind
	seen := map[Kind]bool{}
	for _, part := range p.Recognised {
		if singular[part.Kind] && !seen[part.Kind] {
			seen[part.Kind] = true
			out = append(out, part.Kind)
		}
	}
	return out
}

// kindRank orders the kinds a fully-typed phrase could resolve to, lowest wins: title
// (what a posting is called), then company, then skill (one technology inside a job),
// then category (a department).
var kindRank = map[Kind]int{
	KindTitle:    1,
	KindCompany:  2,
	KindSkill:    3,
	KindCategory: 4,
}

// rank is kindRank with unknown kinds last, so a new kind never silently outranks the rest.
func rank(k Kind) int {
	if r, ok := kindRank[k]; ok {
		return r
	}
	return len(kindRank)
}

// Phrases is the in-process view of the dictionary used to recognise what a visitor
// has already typed. It sits beside the index query, and each has one job:
//
//   - Recognition is exact, so a mistyped phrase falls into the fragment, where the
//     index forgives it.
//   - Completion is typo-tolerant and relevance-ranked, which is Meilisearch's job.
//
// It is a hash lookup, so parsing needs no round trip.
type Phrases struct {
	mu     sync.RWMutex
	byText map[string]Part
	// sorted holds the phrases in order, for a binary search of "does any phrase start
	// with this?".
	sorted []string
	// longest is the word count of the longest phrase, bounding the parse lookahead.
	longest int
}

// NewPhrases builds the recognition set from the dictionary's documents.
func NewPhrases(docs []Document) *Phrases {
	p := &Phrases{}
	p.Replace(docs)
	return p
}

// Replace swaps the recognition set wholesale.
func (p *Phrases) Replace(docs []Document) {
	byText := make(map[string]Part, len(docs))
	longest := 0
	for _, d := range docs {
		text := Title(d.Text)
		if text == "" {
			continue
		}
		// Two kinds can spell the same phrase (`backend`); pick one by precedence, not
		// arrival. The index returns documents ordered by `searches:desc, jobs:desc`,
		// so first-wins would change the applied filter as counts move.
		if cur, ok := byText[text]; ok && rank(cur.Kind) <= rank(d.Kind) {
			continue
		}
		byText[text] = Part{Kind: d.Kind, Slug: d.Slug, Text: d.Text}
		if n := len(strings.Fields(text)); n > longest {
			longest = n
		}
	}
	sorted := make([]string, 0, len(byText))
	for text := range byText {
		sorted = append(sorted, text)
	}
	sort.Strings(sorted)

	p.mu.Lock()
	p.byText, p.sorted, p.longest = byText, sorted, longest
	p.mu.Unlock()
}

// beginsAPhrase reports whether any known phrase starts with this text, meaning the
// visitor may still be typing it.
func beginsAPhrase(sorted []string, text string) bool {
	i := sort.SearchStrings(sorted, text)
	return i < len(sorted) && strings.HasPrefix(sorted[i], text)
}

// Len reports how many phrases are recognised.
func (p *Phrases) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.byText)
}

// Parse splits a query into recognised phrases and the fragment still being typed.
//
// Greedy longest-match, left to right, so a short phrase does not eat the front of a
// longer one (`data` in `data engineer`).
func (p *Phrases) Parse(query string) Parsed {
	p.mu.RLock()
	byText, sorted, longest := p.byText, p.sorted, p.longest
	p.mu.RUnlock()

	normalised := Title(query)
	words := strings.Fields(normalised)

	// The last word is still being typed unless a space follows it, and is never
	// recognised even if it spells a phrase (`go` may be the start of `google`). The
	// space is read from the raw query because Title trims it.
	recognisable := len(words)
	if normalised != "" && !strings.HasSuffix(query, " ") {
		recognisable--

		// If everything typed so far begins a longer phrase, the visitor is still typing
		// that one, so nothing is recognised (`java dev` must reach "Java Developer").
		// Once a space follows, `java ` is the phrase Java.
		if beginsAPhrase(sorted, normalised) {
			recognisable = 0
		}
	}

	var out Parsed
	i := 0
	for i < recognisable {
		matched := false
		// Bounded by recognisable, not len(words): a match may not run over the unfinished
		// word, or `google java developer` would swallow `java developer` and leave
		// nothing to complete.
		for n := min(longest, recognisable-i); n >= 1; n-- {
			if part, ok := byText[strings.Join(words[i:i+n], " ")]; ok {
				out.Recognised = append(out.Recognised, part)
				i += n
				matched = true
				break
			}
		}
		// The first word that begins no phrase ends recognition, so the fragment stays a
		// suffix of the query.
		if !matched {
			break
		}
	}
	out.Fragment = strings.Join(words[i:], " ")
	return out
}
