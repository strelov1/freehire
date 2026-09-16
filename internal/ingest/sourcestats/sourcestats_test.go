package sourcestats

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

var measuredAt = time.Date(2026, 9, 16, 4, 0, 0, 0, time.UTC)

// bySource re-keys the rows for assertion convenience.
func bySource(rows []db.InsertSourceStatParams) map[string]db.InsertSourceStatParams {
	out := make(map[string]db.InsertSourceStatParams, len(rows))
	for _, r := range rows {
		out[r.Source] = r
	}
	return out
}

func TestRowsCoversEveryRegisteredSource(t *testing.T) {
	// greenhouse was scanned; adzuna is registered but every posting of it has closed.
	agg := []db.AggregateOpenJobsBySourceRow{
		{Source: "greenhouse", OpenJobs: 10, AtsMatchedJobs: 0},
	}

	rows := Rows([]string{"greenhouse", "adzuna"}, agg, nil, Measured(map[string]int64{"greenhouse": 8}), measuredAt)

	got := bySource(rows)
	if len(rows) != 2 {
		t.Fatalf("Rows() returned %d rows, want one per registered source: %v", len(rows), got)
	}
	// A registered source the scan did not return has been MEASURED as empty, so it must
	// carry a zero — a missing row would be read as "never measured".
	adzuna, ok := got["adzuna"]
	if !ok {
		t.Fatalf("Rows() dropped a registered source with no open postings: %v", got)
	}
	if adzuna.OpenJobs != 0 || adzuna.AtsMatchedJobs != 0 {
		t.Errorf("empty source = %+v, want zero counts", adzuna)
	}
}

func TestRowsCoversASourceWithPostingsButNoAdapter(t *testing.T) {
	// `telegram` is the real case: an extraction pipeline, not a registered adapter, so
	// it is absent from Taxonomy while carrying real postings and its own display label.
	// A registry-only spine would omit it from a page whose whole claim is completeness,
	// and the omission would be invisible.
	agg := []db.AggregateOpenJobsBySourceRow{
		{Source: "greenhouse", OpenJobs: 10},
		{Source: "telegram", OpenJobs: 3},
	}

	rows := Rows([]string{"greenhouse"}, agg, nil, Unmeasured(), measuredAt)

	got := bySource(rows)
	tg, ok := got["telegram"]
	if !ok {
		t.Fatalf("Rows() dropped a source that has postings but no adapter: %v", got)
	}
	if tg.OpenJobs != 3 {
		t.Errorf("telegram OpenJobs = %d, want 3", tg.OpenJobs)
	}
}

func TestRowsKeepsASourceThatHasGoneQuiet(t *testing.T) {
	// telegram has no adapter and, this run, no open postings either — so it is in neither
	// the registry nor the scan. Without the previous snapshot in the union it disappears
	// from the page the moment its last posting closes, which is the same silent drop the
	// union exists to prevent, one closure later.
	previous := []db.SourceStat{
		{Source: "greenhouse", OpenJobs: 10},
		{Source: "telegram", OpenJobs: 3},
	}
	agg := []db.AggregateOpenJobsBySourceRow{{Source: "greenhouse", OpenJobs: 10}}

	rows := Rows([]string{"greenhouse"}, agg, previous, Unmeasured(), measuredAt)

	got := bySource(rows)
	tg, ok := got["telegram"]
	if !ok {
		t.Fatalf("Rows() dropped a source that has gone quiet: %v", got)
	}
	// Re-measured, not carried over: the row says "we looked and there is nothing now".
	if tg.OpenJobs != 0 {
		t.Errorf("telegram OpenJobs = %d, want a freshly measured 0 rather than the stored 3", tg.OpenJobs)
	}
}

func TestRowsCountsAUnionedSourceOnce(t *testing.T) {
	agg := []db.AggregateOpenJobsBySourceRow{{Source: "greenhouse", OpenJobs: 10}}

	rows := Rows([]string{"greenhouse", "adzuna"}, agg, nil, Unmeasured(), measuredAt)

	if len(rows) != 2 {
		t.Errorf("Rows() returned %d rows for 2 distinct sources: %v", len(rows), bySource(rows))
	}
}

func TestRowsCarriesTheScannedFigures(t *testing.T) {
	agg := []db.AggregateOpenJobsBySourceRow{
		{Source: "adzuna", OpenJobs: 100, AtsMatchedJobs: 70},
	}

	rows := Rows([]string{"adzuna"}, agg, nil, Measured(map[string]int64{"adzuna": 42}), measuredAt)

	got := bySource(rows)["adzuna"]
	if got.OpenJobs != 100 || got.AtsMatchedJobs != 70 {
		t.Errorf("counts = %d/%d, want 100/70", got.OpenJobs, got.AtsMatchedJobs)
	}
	if !got.BrowsableJobs.Valid || got.BrowsableJobs.Int64 != 42 {
		t.Errorf("BrowsableJobs = %+v, want 42", got.BrowsableJobs)
	}
	if !got.MeasuredAt.Valid || !got.MeasuredAt.Time.Equal(measuredAt) {
		t.Errorf("MeasuredAt = %+v, want %v", got.MeasuredAt, measuredAt)
	}
}

// The whole point of the two constructors: an unmeasured distribution must not look
// like a measured-empty one.
func TestUnmeasuredLeavesBrowsableAbsentNotZero(t *testing.T) {
	agg := []db.AggregateOpenJobsBySourceRow{{Source: "greenhouse", OpenJobs: 10}}

	rows := Rows([]string{"greenhouse"}, agg, nil, Unmeasured(), measuredAt)

	if got := bySource(rows)["greenhouse"].BrowsableJobs; got.Valid {
		t.Errorf("BrowsableJobs = %+v after an unmeasured run, want absent — a 0 here would publish 'this source has no jobs'", got)
	}
}

func TestMeasuredButAbsentFromTheDistributionIsZero(t *testing.T) {
	// Meilisearch answered; this source simply has nothing in the de-duplicated index —
	// every posting of it was suppressed as a duplicate. That IS zero, and saying so is
	// the finding.
	agg := []db.AggregateOpenJobsBySourceRow{{Source: "whatjobs", OpenJobs: 500, AtsMatchedJobs: 500}}

	rows := Rows([]string{"whatjobs"}, agg, nil, Measured(map[string]int64{"greenhouse": 8}), measuredAt)

	got := bySource(rows)["whatjobs"].BrowsableJobs
	if !got.Valid || got.Int64 != 0 {
		t.Errorf("BrowsableJobs = %+v, want a measured 0", got)
	}
}

func TestRowsIsOrderedBySource(t *testing.T) {
	rows := Rows([]string{"lever", "ashby", "greenhouse"}, nil, nil, Unmeasured(), measuredAt)

	var order []string
	for _, r := range rows {
		order = append(order, r.Source)
	}
	want := []string{"ashby", "greenhouse", "lever"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}
