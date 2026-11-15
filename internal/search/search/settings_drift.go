package search

import (
	"context"
	"fmt"

	"github.com/meilisearch/meilisearch-go"
)

// SettingsDrift reports how the LIVE jobs and companies indexes lag this binary's
// facetSettings()/companySettings(): a sortable or filterable attribute or an embedder the
// binary may use that the live index lacks. Each entry names the index and the missing
// item; nil, nil means both are current.
//
// A binary rolled out before its settings patch turns every request for the new attribute
// or embedder into a Meili 400 and then a 500 (see AGENTS.md). Meant to run on a schedule.
//
// Only this direction is reported: a live index declaring more than the binary asks for is
// harmless.
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

// diffSettings is the pure comparison behind SettingsDrift: every sortable attribute,
// filterable attribute and embedder in want that live lacks.
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
