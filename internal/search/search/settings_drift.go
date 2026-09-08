package search

import (
	"context"
	"fmt"

	"github.com/meilisearch/meilisearch-go"
)

// SettingsDrift reports every way the LIVE jobs and companies indexes lag what this
// binary's own facetSettings()/companySettings() expect: a sortable or filterable
// attribute this binary may request that the live index has not declared, or an
// embedder this binary may query that the live index does not carry. Each entry is a
// human-readable line naming the index and the missing attribute or embedder; a nil,
// nil result means both indexes are current.
//
// This exists because nothing checked it. AGENTS.md documents the hazard by hand for
// both indexes ("Adding a filterable/sortable attribute", "Skill vectors and the match
// sort"): a binary rolled out before its settings patch reaches the live index turns
// every request for the new attribute or embedder into a Meili 400, which this
// package's error mapping turns into a 500 for every caller — not just the one who
// asked for the new sort, filter, or the match ranking. There was no operator script
// for it; this is that script's read side, meant to run on a schedule and publish what
// it finds rather than wait for someone to notice a broken page.
//
// It only reports THIS direction. A live index still declaring an attribute or
// embedder this binary no longer asks for is the other, harmless direction — see
// AGENTS.md: only a binary that asks for what the live index lacks breaks.
func (c *Client) SettingsDrift(ctx context.Context) ([]string, error) {
	var drift []string

	live, err := c.facet.GetSettingsWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("search: get live settings for %s: %w", facetIndexUID, err)
	}
	drift = append(drift, diffSettings(facetIndexUID, live, facetSettings())...)

	live, err = c.manager.Index(companyIndexUID).GetSettingsWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("search: get live settings for %s: %w", companyIndexUID, err)
	}
	drift = append(drift, diffSettings(companyIndexUID, live, companySettings())...)

	return drift, nil
}

// diffSettings is the pure comparison SettingsDrift's I/O wraps: given what the live
// index actually declares and what this binary expects, it names every sortable
// attribute, filterable attribute, and embedder the binary may ask for that the live
// index does not yet carry. Kept separate from the network call so the interesting
// cases (a missing sort, a missing filter, a missing embedder, nothing missing) are
// unit-testable against fabricated settings instead of a running engine.
func diffSettings(indexUID string, live, want *meilisearch.Settings) []string {
	var drift []string

	liveSortable := toSet(live.SortableAttributes)
	for _, attr := range want.SortableAttributes {
		if !liveSortable[attr] {
			drift = append(drift, fmt.Sprintf("%s: sortable attribute %q not yet live", indexUID, attr))
		}
	}

	liveFilterable := toSet(live.FilterableAttributes)
	for _, attr := range want.FilterableAttributes {
		if !liveFilterable[attr] {
			drift = append(drift, fmt.Sprintf("%s: filterable attribute %q not yet live", indexUID, attr))
		}
	}

	for name := range want.Embedders {
		if _, ok := live.Embedders[name]; !ok {
			drift = append(drift, fmt.Sprintf("%s: embedder %q not yet live", indexUID, name))
		}
	}

	return drift
}

func toSet(xs []string) map[string]bool {
	set := make(map[string]bool, len(xs))
	for _, x := range xs {
		set[x] = true
	}
	return set
}
