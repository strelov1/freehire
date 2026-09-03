package suggest

import (
	"context"
	"fmt"
	"strings"
)

// Index is what the service needs from the search engine. Narrow on purpose: it names
// the two reads this package makes, so the engine's client can satisfy it without this
// package importing it.
type Index interface {
	// SearchSuggestions completes a fragment. Typo tolerance and relevance ranking are
	// the engine's, which is the whole reason the fragment goes here rather than
	// through a matcher of our own.
	SearchSuggestions(ctx context.Context, query, filter string, limit int) ([]Document, error)
	// AllSuggestions reads the dictionary whole, for the recognition set.
	AllSuggestions(ctx context.Context) ([]Document, error)
}

// Suggestion is one offered row: the whole phrase it names, and every facet part
// picking it applies. A row is the recognised prefix plus one completion, so it can
// carry more than one part — "Senior Software Engineer Google" sets the role and the
// company together, and applying one of the two would silently discard what the
// visitor typed.
type Suggestion struct {
	Text  string `json:"text"`
	Parts []Part `json:"parts"`
	Jobs  int    `json:"jobs"`
}

// Service answers the search box.
type Service struct {
	index   Index
	phrases *Phrases
}

// New builds the service with an empty recognition set. Refresh fills it; until then
// nothing is recognised and every query is one fragment, which degrades to the plain
// completion behaviour rather than to an error.
func New(index Index) *Service {
	return &Service{index: index, phrases: NewPhrases(nil)}
}

// Refresh reloads the recognition set from the dictionary. The builder rewrites that
// index wholesale on its own schedule, so this is how a long-lived process stops
// recognising phrases that no longer exist and starts recognising new ones.
func (s *Service) Refresh(ctx context.Context) error {
	docs, err := s.index.AllSuggestions(ctx)
	if err != nil {
		return fmt.Errorf("suggest: refresh: %w", err)
	}
	s.phrases.Replace(docs)
	return nil
}

// Suggest answers a query.
//
// An empty one returns NOTHING, and that is a boundary rather than a gap. What an
// empty box should offer is a curated answer — ranking the catalogue by size leads
// with Management (267,971), Sales (180,253) and Support (127,110), which reads as a
// different website to somebody who came for engineering work — and the curation that
// answers it is the filter modal's own category grouping, which lives in the web and
// is checked there for completeness against the category vocabulary at compile time.
// Serving it from here would mean a second copy of that order, and a second copy is
// two lists that agree until one of them is edited.
func (s *Service) Suggest(ctx context.Context, query string, limit int) ([]Suggestion, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	parsed := s.phrases.Parse(query)
	// Over-ask: withholding drained suggestions below can empty a page that the engine
	// filled, and asking for exactly the limit would leave the box short.
	hits, err := s.index.SearchSuggestions(ctx, parsed.Fragment, kindFilter(parsed.ExcludedKinds()), limit*2)
	if err != nil {
		return nil, err
	}
	return rows(parsed.Recognised, hits, limit), nil
}

// rows composes each completion with the prefix already recognised.
func rows(prefix []Part, hits []Document, limit int) []Suggestion {
	out := make([]Suggestion, 0, len(hits))
	for _, h := range hits {
		// A count that has fallen to zero between rebuilds means the postings are gone;
		// offering it sends the visitor to an empty page, which is worse than offering
		// nothing.
		if h.Jobs <= 0 {
			continue
		}
		parts := make([]Part, 0, len(prefix)+1)
		parts = append(parts, prefix...)
		parts = append(parts, Part{Kind: h.Kind, Slug: h.Slug, Text: h.Text})

		words := make([]string, 0, len(parts))
		for _, p := range parts {
			words = append(words, p.Text)
		}
		out = append(out, Suggestion{Text: strings.Join(words, " "), Parts: parts, Jobs: h.Jobs})
		if len(out) == limit {
			break
		}
	}
	return out
}

// kindFilter renders the engine filter that leaves out the kinds the prefix already
// filled. Empty when there is nothing to exclude — a filter of `true` would be a
// second thing to get wrong.
func kindFilter(kinds []Kind) string {
	if len(kinds) == 0 {
		return ""
	}
	clauses := make([]string, 0, len(kinds))
	for _, k := range kinds {
		clauses = append(clauses, fmt.Sprintf("kind != %q", string(k)))
	}
	return strings.Join(clauses, " AND ")
}
