package suggest

import (
	"strings"
	"sync"
)

// Part is one piece of a query the dictionary recognised — a whole phrase, and the
// facet it applies. A suggestion the box offers is the recognised parts plus one
// candidate for the fragment, and picking it applies all of them.
type Part struct {
	Kind Kind   `json:"kind"`
	Slug string `json:"slug,omitempty"`
	Text string `json:"text"`
}

// Parsed splits a query into what the dictionary already knows and what is still
// being typed.
type Parsed struct {
	// Recognised are the whole phrases consumed from the front of the query, in order.
	Recognised []Part
	// Fragment is everything the phrases did not consume — what gets completed.
	Fragment string
}

// singular are the kinds a query may name only once. A posting has ONE role and ONE
// employer, so offering a second of either composes a filter that matches nothing.
// Skills are absent on purpose: several skills narrow a search sensibly, which is the
// whole reason `java` and `kubernetes` are separate facet values.
var singular = map[Kind]bool{
	KindRole:     true,
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

// kindRank orders the kinds a fully-typed phrase could resolve to, lowest wins.
//
// The reasoning is which reading of a WHOLE phrase is strongest, and it follows the
// same doctrine Build uses to drop a category that shares a role's slug:
//
//   - a role names a job, which is what somebody typing a whole phrase is naming;
//   - a title is what a posting is actually CALLED, so it beats anything merely
//     mentioned inside it;
//   - a company names an employer, and rarely collides with the rest at all;
//   - a skill is one technology inside a job, not the job;
//   - a category names a department, the weakest reading of a phrase.
var kindRank = map[Kind]int{
	KindRole:     0,
	KindTitle:    1,
	KindCompany:  2,
	KindSkill:    3,
	KindCategory: 4,
}

// rank is kindRank with a defined answer for a kind added later: last, so a new kind
// never silently outranks the ones whose precedence was reasoned about.
func rank(k Kind) int {
	if r, ok := kindRank[k]; ok {
		return r
	}
	return len(kindRank)
}

// Phrases is the in-process view of the dictionary used to RECOGNISE what a visitor
// has already typed. It is deliberately a second mechanism beside the index query,
// and each has one job:
//
//   - Recognition must be EXACT. A mistyped phrase has to fall through into the
//     fragment, where the index forgives it — silently consuming it as recognised
//     would mean the typo is never corrected and never completed.
//   - Completion must be typo-tolerant and relevance-ranked, which is what
//     Meilisearch already does. Reimplementing that here would be a second matcher,
//     and two matchers drift.
//
// It is a hash lookup over a few tens of thousands of phrases, so the parse costs
// nothing at query time and needs no round trip.
type Phrases struct {
	mu     sync.RWMutex
	byText map[string]Part
	// longest is the word count of the longest phrase, which bounds how far the parse
	// looks ahead. Without it every position would try every possible length.
	longest int
}

// NewPhrases builds the recognition set from the dictionary's documents.
func NewPhrases(docs []Document) *Phrases {
	p := &Phrases{}
	p.Replace(docs)
	return p
}

// Replace swaps the recognition set wholesale, which is how a periodic refresh keeps
// it current with the index the builder rewrites.
func (p *Phrases) Replace(docs []Document) {
	byText := make(map[string]Part, len(docs))
	longest := 0
	for _, d := range docs {
		text := Title(d.Text)
		if text == "" {
			continue
		}
		// Two kinds CAN spell the same phrase — `backend` is a role, a skill and a
		// category — and the parse must pick ONE, because that choice decides which
		// facet a fully-typed phrase applies.
		//
		// By PRECEDENCE, not by arrival. First-writer-wins looks equivalent and is not:
		// this set is loaded from the index, whose empty-query order is
		// `searches:desc, jobs:desc`, so the winner would move as demand and posting
		// counts move. The same query would apply a different filter on different days
		// and nothing would look broken.
		if cur, ok := byText[text]; ok && rank(cur.Kind) <= rank(d.Kind) {
			continue
		}
		byText[text] = Part{Kind: d.Kind, Slug: d.Slug, Text: d.Text}
		if n := len(strings.Fields(text)); n > longest {
			longest = n
		}
	}
	p.mu.Lock()
	p.byText, p.longest = byText, longest
	p.mu.Unlock()
}

// Len reports how many phrases are recognised, for the caller that logs a refresh.
func (p *Phrases) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.byText)
}

// Parse splits a query into recognised phrases and the fragment still being typed.
//
// Greedy longest-match, left to right: at each position it tries the longest run of
// words the dictionary could hold and shortens until one matches. Longest rather than
// first, because a short phrase that prefixes a longer one would otherwise be consumed
// and strand the rest of the longer one in the fragment — `data` eating the front of
// `data engineer`.
func (p *Phrases) Parse(query string) Parsed {
	p.mu.RLock()
	byText, longest := p.byText, p.longest
	p.mu.RUnlock()

	normalised := Title(query)
	words := strings.Fields(normalised)

	// The last word is still being TYPED unless a space follows it, and a word being
	// typed must never be recognised — even when it already spells a phrase on its own.
	// Observed on production: `senior software engineer go` consumed `go` as the skill
	// it is, leaving nothing to complete, so the box offered "Senior Software Engineer
	// Go Director". There `go` is the first two letters of `google`.
	//
	// The trailing space is the signal, and it is the one the visitor gives. Note it is
	// read from the RAW query: Title trims, so by here the space is gone.
	recognisable := len(words)
	if normalised != "" && !strings.HasSuffix(query, " ") {
		recognisable--
	}

	var out Parsed
	i := 0
	for i < recognisable {
		matched := false
		// Never look past the end, and never past the longest phrase there is.
		for n := min(longest, len(words)-i); n >= 1; n-- {
			if part, ok := byText[strings.Join(words[i:i+n], " ")]; ok {
				out.Recognised = append(out.Recognised, part)
				i += n
				matched = true
				break
			}
		}
		// The first word that begins no phrase ends recognition: everything from here
		// on is what the visitor is still typing, including any later word that would
		// have matched on its own. A phrase recognised AFTER a gap would mean the
		// fragment is not a suffix of the query, and the box could no longer render
		// "what you typed plus this completion".
		if !matched {
			break
		}
	}
	out.Fragment = strings.Join(words[i:], " ")
	return out
}
