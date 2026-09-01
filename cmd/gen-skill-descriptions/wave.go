package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// wave is the next batch to draft: the canonicals nobody has described yet, most
// demanded first, capped at limit (all of them when limit is not positive).
//
// Ordering by how many open postings name the skill is what makes the programme
// finishable. The vocabulary is hundreds of entries and every one of them costs a human
// read, so the first wave has to be the skills a reader actually meets — a glossary that
// defines `as400` before `kubernetes` is a glossary nobody has used yet.
//
// A skill the catalogue does not currently name still belongs in a later wave: the
// glossary covers the vocabulary, not this week's postings. It counts zero and sorts to
// the end rather than dropping out.
func wave(canonicals []string, described map[string]string, demand map[string]int, limit int) []string {
	out := make([]string, 0, len(canonicals))
	for _, c := range canonicals {
		if described[c] == "" {
			out = append(out, c)
		}
	}
	// Ties break on the slug so a re-run after an edit shows the same remainder in the
	// same order — the operator drafts, edits, and re-runs to see what is left.
	slices.SortFunc(out, func(a, b string) int {
		if d := cmp.Compare(demand[b], demand[a]); d != 0 {
			return d
		}
		return cmp.Compare(a, b)
	})
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out
}

// parseSkillDemand reads the skills distribution out of a GET /jobs/facets envelope:
// canonical slug → open postings naming it.
//
// The count comes from the search index rather than from Postgres because the question
// is only "which of these matters most", and a GROUP BY over unnest(skills) across the
// whole catalogue is minutes of database work to answer something the facets endpoint
// already publishes.
//
// An envelope carrying no skills distribution is an error, not an empty map. Ranking
// the whole vocabulary by zero would silently produce an alphabetical wave, which looks
// exactly like a wave and is not one.
func parseSkillDemand(body []byte) (map[string]int, error) {
	var envelope struct {
		Data struct {
			Facets map[string]map[string]int `json:"facets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decoding facets envelope: %w", err)
	}
	demand := envelope.Data.Facets["skills"]
	if len(demand) == 0 {
		return nil, errors.New("facets envelope carries no skills distribution")
	}
	return demand, nil
}
