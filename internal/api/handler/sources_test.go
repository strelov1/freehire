package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

var sourcesNow = time.Date(2026, 9, 16, 6, 0, 0, 0, time.UTC)

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func i8(n int64) pgtype.Int8 { return pgtype.Int8{Int64: n, Valid: true} }

func entriesBySource(entries []sourceEntry) map[string]sourceEntry {
	out := make(map[string]sourceEntry, len(entries))
	for _, e := range entries {
		out[e.Source] = e
	}
	return out
}

func TestBuildSourceEntriesClassifiesByTheAdapterRegistry(t *testing.T) {
	snap := []db.SourceStat{
		{Source: "greenhouse", OpenJobs: 10, MeasuredAt: ts(sourcesNow)},
		{Source: "adzuna", OpenJobs: 10, MeasuredAt: ts(sourcesNow)},
	}

	got := entriesBySource(buildSourceEntries(sources.Taxonomy(), nil, snap, sourcesNow))

	if got["greenhouse"].Kind != sources.KindATS {
		t.Errorf("greenhouse kind = %q, want %q", got["greenhouse"].Kind, sources.KindATS)
	}
	if got["adzuna"].Kind != sources.KindAggregator {
		t.Errorf("adzuna kind = %q, want %q", got["adzuna"].Kind, sources.KindAggregator)
	}
}

func TestBuildSourceEntriesCarriesOverlapForAggregatorsOnly(t *testing.T) {
	snap := []db.SourceStat{
		{Source: "adzuna", OpenJobs: 100, AtsMatchedJobs: 70, MeasuredAt: ts(sourcesNow)},
		{Source: "greenhouse", OpenJobs: 100, AtsMatchedJobs: 0, MeasuredAt: ts(sourcesNow)},
	}

	got := entriesBySource(buildSourceEntries(sources.Taxonomy(), nil, snap, sourcesNow))

	adzuna := got["adzuna"].Jobs
	if adzuna == nil || adzuna.ATSMatched == nil || *adzuna.ATSMatched != 70 {
		t.Fatalf("adzuna overlap = %+v, want a matched count of 70", adzuna)
	}
	if adzuna.ATSUnmatched == nil || *adzuna.ATSUnmatched != 30 {
		t.Errorf("adzuna unmatched = %+v, want 30 (raw minus matched)", adzuna.ATSUnmatched)
	}

	// A first-party ATS source is never marked by the aggregator-suppression pass, so its
	// 0 is arithmetic rather than a finding. Publishing it would read as "fully exclusive".
	gh := got["greenhouse"].Jobs
	if gh == nil {
		t.Fatal("greenhouse has no jobs block")
	}
	if gh.ATSMatched != nil || gh.ATSUnmatched != nil {
		t.Errorf("greenhouse carries overlap figures %+v/%+v, want neither", gh.ATSMatched, gh.ATSUnmatched)
	}
}

func TestBuildSourceEntriesKeepsAnUnmeasuredCountAbsent(t *testing.T) {
	snap := []db.SourceStat{
		{Source: "greenhouse", OpenJobs: 10, MeasuredAt: ts(sourcesNow)}, // BrowsableJobs invalid
		{Source: "lever", OpenJobs: 10, BrowsableJobs: i8(0), MeasuredAt: ts(sourcesNow)},
	}

	got := entriesBySource(buildSourceEntries(sources.Taxonomy(), nil, snap, sourcesNow))

	if b := got["greenhouse"].Jobs.Browsable; b != nil {
		t.Errorf("greenhouse browsable = %d, want absent — Meilisearch was not measured", *b)
	}
	if b := got["lever"].Jobs.Browsable; b == nil || *b != 0 {
		t.Errorf("lever browsable = %v, want a measured 0", b)
	}
}

func TestBuildSourceEntriesHasNoJobsBlockWithoutASnapshot(t *testing.T) {
	got := entriesBySource(buildSourceEntries(sources.Taxonomy(), nil, nil, sourcesNow))

	if got["greenhouse"].Jobs != nil {
		t.Errorf("greenhouse carries a jobs block with no snapshot: %+v", got["greenhouse"].Jobs)
	}
	if len(got) == 0 {
		t.Fatal("no entries at all; every registered adapter must still be listed")
	}
}

func TestBuildSourceEntriesHasNoHealthBlockWithoutAHealthRecord(t *testing.T) {
	// telegram is an extraction pipeline, not a crawl adapter: no board_health row. A
	// derived "down" here would be a verdict about a source nothing measured.
	snap := []db.SourceStat{{Source: "telegram", OpenJobs: 5, MeasuredAt: ts(sourcesNow)}}

	got := entriesBySource(buildSourceEntries(sources.Taxonomy(), nil, snap, sourcesNow))

	tg, ok := got["telegram"]
	if !ok {
		t.Fatal("a source with postings but no adapter was dropped")
	}
	if tg.Kind != sources.KindOther {
		t.Errorf("telegram kind = %q, want %q", tg.Kind, sources.KindOther)
	}
	if tg.Health != nil {
		t.Errorf("telegram carries a health verdict %+v, want none", tg.Health)
	}
}

func TestBuildSourceEntriesCarriesHealthWhenThereIsARecord(t *testing.T) {
	health := []db.ProviderHealthRollupRow{{
		Provider: "greenhouse", TotalBoards: 900, HealthyBoards: 890, CooledBoards: 10,
		LastRunAt: ts(sourcesNow.Add(-time.Hour)), LastSuccessAt: ts(sourcesNow.Add(-time.Hour)),
		IngestedTotal: 4321,
	}}

	got := entriesBySource(buildSourceEntries(sources.Taxonomy(), health, nil, sourcesNow))["greenhouse"]

	if got.Health == nil {
		t.Fatal("greenhouse has no health block")
	}
	if got.Health.IngestedTotal != 4321 || got.Health.TotalBoards != 900 {
		t.Errorf("health = %+v, want the rollup's figures", got.Health)
	}
	if got.Health.Status != statusOperational {
		t.Errorf("status = %q, want operational", got.Health.Status)
	}
}

// A posting URL never reaches the wire at all now. The first version published its host so
// the page could resolve a logo from it; production served the WRONG brand, because an ATS
// posting's URL usually lives on the employer's own domain. Logos come from the display
// name client-side, and this endpoint has no field that could carry a link to somebody's job.
func TestSourceEntryCarriesNoPostingURL(t *testing.T) {
	snap := []db.SourceStat{{Source: "greenhouse", OpenJobs: 1, MeasuredAt: ts(sourcesNow)}}

	blob, err := json.Marshal(buildSourceEntries(sources.Taxonomy(), nil, snap, sourcesNow))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"http", "logo_host", "sample_url"} {
		if strings.Contains(string(blob), banned) {
			t.Errorf("wire shape carries %q:\n%s", banned, blob)
		}
	}
}

// Sanitization by construction, the rule /status already holds itself to: an entry has no
// field that could carry a board identifier or a crawl error, so one cannot leak by
// somebody forgetting to omit it.
func TestSourceEntryCarriesNoBoardOrErrorField(t *testing.T) {
	health := []db.ProviderHealthRollupRow{{Provider: "greenhouse", TotalBoards: 1, LastSuccessAt: ts(sourcesNow)}}
	snap := []db.SourceStat{{Source: "greenhouse", OpenJobs: 1, MeasuredAt: ts(sourcesNow)}}

	blob, err := json.Marshal(buildSourceEntries(sources.Taxonomy(), health, snap, sourcesNow))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"board"`, `"boards"`, `"last_error"`, `"error"`} {
		if strings.Contains(string(blob), banned) {
			t.Errorf("wire shape carries %s: an internal detail must not be renderable", banned)
		}
	}
}

// The unmatched count is the absence of evidence, not evidence of absence. The page has its
// own guard on the rendered words; this one holds the WIRE, because a field named
// `exclusive` would put the claim in every client that ever reads this endpoint — including
// ones we do not write — and no amount of careful page copy would reach them.
func TestSourceEntryNeverNamesAFieldExclusive(t *testing.T) {
	snap := []db.SourceStat{{Source: "adzuna", OpenJobs: 100, AtsMatchedJobs: 70, MeasuredAt: ts(sourcesNow)}}

	blob, err := json.Marshal(buildSourceEntries(sources.Taxonomy(), nil, snap, sourcesNow))
	if err != nil {
		t.Fatal(err)
	}
	// Anchor: the overlap figures must actually be in this payload, or the check below
	// passes because there is nothing to find.
	if !strings.Contains(string(blob), `"ats_unmatched"`) {
		t.Fatalf("no overlap figures in the payload; the guard would pass vacuously:\n%s", blob)
	}
	if strings.Contains(strings.ToLower(string(blob)), "exclusive") {
		t.Errorf("the wire shape says \"exclusive\":\n%s", blob)
	}
}
