package suggest

import (
	"context"
	"fmt"
	"strings"
)

// Index is the two reads the service needs from the search engine.
type Index interface {
	// SearchSuggestions completes a fragment; typo tolerance and ranking are the engine's.
	SearchSuggestions(ctx context.Context, query, filter string, limit int) ([]Document, error)
	// AllSuggestions reads the dictionary whole, for the recognition set.
	AllSuggestions(ctx context.Context) ([]Document, error)
}

// Suggestion is one offered row: the whole phrase and every facet part picking it applies.
// A row is the recognised prefix plus one completion, so it can carry several parts
// ("Senior Software Engineer Google" sets role and company together).
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

// New builds the service with an empty recognition set. Until Refresh fills it, every
// query is one plain fragment.
func New(index Index) *Service {
	return &Service{index: index, phrases: NewPhrases(nil)}
}

// Refresh reloads the recognition set from the dictionary, which the builder rewrites
// wholesale on its own schedule.
func (s *Service) Refresh(ctx context.Context) error {
	docs, err := s.index.AllSuggestions(ctx)
	if err != nil {
		return fmt.Errorf("suggest: refresh: %w", err)
	}
	s.phrases.Replace(docs)
	return nil
}

// Suggest answers a query. An empty query returns nothing on purpose: the empty-box
// answer is the web filter modal's curated category grouping (ranking by size would lead
// with Management and Sales), and serving it here would duplicate that order.
func (s *Service) Suggest(ctx context.Context, query string, limit int) ([]Suggestion, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	parsed := s.phrases.Parse(query)
	// Over-ask: rows drops zero-job hits, which could leave the page short.
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
		// A count that fell to zero between rebuilds would lead to an empty page.
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
// filled. Empty when there is nothing to exclude.
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
