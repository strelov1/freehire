package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// wave is the next batch to draft: the canonicals nobody has described yet, most
// demanded first, capped at limit (all of them when limit is not positive). Skills the
// catalogue does not currently name count zero and sort last rather than dropping out.
func wave(canonicals []string, described map[string]string, demand map[string]int, limit int) []string {
	out := make([]string, 0, len(canonicals))
	for _, c := range canonicals {
		if described[c] == "" {
			out = append(out, c)
		}
	}
	// Ties break on the slug so a re-run shows the same remainder in the same order.
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
// canonical slug → open postings naming it. It uses the search index because a GROUP BY over
// unnest(skills) is minutes of database work for a ranking the facets endpoint publishes.
//
// An envelope with no skills distribution is an error, not an empty map: ranking by zero
// would silently produce an alphabetical wave.
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
