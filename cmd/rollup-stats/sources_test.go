package main

import (
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sourcestats"
	"github.com/strelov1/freehire/internal/search/search"
)

// browsableFor reads the de-duplicated count each named source would be published with.
// BrowsableCounts keeps its two states private on purpose, so the assertion goes through
// the same Rows call the worker makes — nil meaning "absent", exactly as the snapshot
// column does.
func browsableFor(t *testing.T, counts sourcestats.BrowsableCounts, sources ...string) map[string]*int64 {
	t.Helper()
	out := make(map[string]*int64, len(sources))
	for _, row := range sourcestats.Rows(sources, nil, nil, counts, time.Now()) {
		if row.BrowsableJobs.Valid {
			n := row.BrowsableJobs.Int64
			out[row.Source] = &n
			continue
		}
		out[row.Source] = nil
	}
	return out
}

func TestResolveBrowsableCountsMeasuresWhatMeilisearchAnswered(t *testing.T) {
	res := search.FacetResult{Facets: map[string]map[string]int64{
		sourceFacetAttr: {"greenhouse": 800, "adzuna": 42},
	}}

	got := resolveBrowsableCounts(res, nil)

	rows := browsableFor(t, got, "greenhouse", "adzuna", "lever")
	if rows["greenhouse"] == nil || *rows["greenhouse"] != 800 {
		t.Errorf("greenhouse = %v, want 800", rows["greenhouse"])
	}
	// Meilisearch answered and did not mention lever: a real, measured zero.
	if rows["lever"] == nil || *rows["lever"] != 0 {
		t.Errorf("lever = %v, want a measured 0", rows["lever"])
	}
}

func TestResolveBrowsableCountsLeavesEverythingAbsentOnError(t *testing.T) {
	got := resolveBrowsableCounts(search.FacetResult{}, errors.New("meili unreachable"))

	if rows := browsableFor(t, got, "greenhouse"); rows["greenhouse"] != nil {
		t.Errorf("greenhouse = %v after a failed facet request, want absent — a 0 would publish 'this source has no jobs'", *rows["greenhouse"])
	}
}

// A response that carries no distribution for the source attribute at all is a failed
// measurement too, not an empty catalogue: every source would come out as zero and the
// page would report the whole fleet dead.
func TestResolveBrowsableCountsTreatsAMissingAttrAsUnmeasured(t *testing.T) {
	res := search.FacetResult{Facets: map[string]map[string]int64{"countries": {"us": 10}}}

	got := resolveBrowsableCounts(res, nil)

	if rows := browsableFor(t, got, "greenhouse"); rows["greenhouse"] != nil {
		t.Errorf("greenhouse = %v when the response carried no source distribution, want absent", *rows["greenhouse"])
	}
}
