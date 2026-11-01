package serveprobe_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/serveprobe"
)

// shown withholds nothing, the case of a row search should hold a document for.
func shown(db.Job) search.WithholdReason { return "" }

func row(id int64) db.Job {
	return db.Job{
		ID: id, Source: "ashby", ExternalID: "acme:x", Title: "Web Developer",
		Company: "Acme", CompanySlug: "acme", PublicSlug: "web-developer-acme",
		CreatedAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	}
}

// The case the probe exists for: a source salary the row stores and the index
// document lacks. The projection serves it (so the code is right) while the stale
// document does not (so a reindex is owed) — the two layers must say so separately.
func TestCheck_SeparatesAStaleIndexFromTheProjection(t *testing.T) {
	r := row(1)
	r.SalaryMinSource = pgtype.Int4{Int32: 180000, Valid: true}
	r.SalaryCurrencySource, r.SalaryPeriodSource = "CAD", "year"
	r.Countries = []string{"ca"}

	stale := jobview.Job{Countries: []string{"ca"}}
	rep := serveprobe.Check([]db.Job{r}, map[int64]jobview.Job{1: stale}, shown)

	if got := rep.Counts[serveprobe.LayerProjection]["salary"]; got != (serveprobe.Count{Stored: 1, Gaps: 0}) {
		t.Errorf("projection salary = %+v, want stored 1 / gaps 0", got)
	}
	if got := rep.Counts[serveprobe.LayerIndex]["salary"]; got != (serveprobe.Count{Stored: 1, Gaps: 1}) {
		t.Errorf("index salary = %+v, want stored 1 / gaps 1", got)
	}
	if got := rep.Counts[serveprobe.LayerIndex]["countries"]; got != (serveprobe.Count{Stored: 1, Gaps: 0}) {
		t.Errorf("index countries = %+v, want stored 1 / gaps 0", got)
	}
}

// A row with no document is not a field gap: it is counted apart, by why it has none,
// and checked in the projection only.
func TestCheck_AMissingDocumentIsUnindexedNotAGap(t *testing.T) {
	r := row(2)
	r.WorkMode = "remote"

	rep := serveprobe.Check([]db.Job{r}, map[int64]jobview.Job{}, shown)

	if rep.Sampled != 1 || rep.Unindexed[serveprobe.Missing] != 1 {
		t.Errorf("sampled/missing = %d/%d, want 1/1", rep.Sampled, rep.Unindexed[serveprobe.Missing])
	}
	if got := rep.Counts[serveprobe.LayerIndex]["work_mode"]; got != (serveprobe.Count{}) {
		t.Errorf("index work_mode = %+v, want nothing counted", got)
	}
	if got := rep.Counts[serveprobe.LayerProjection]["work_mode"]; got != (serveprobe.Count{Stored: 1}) {
		t.Errorf("projection work_mode = %+v, want stored 1 / gaps 0", got)
	}
}

// A field the row does not store is never counted, so an empty catalogue column cannot
// read as a served one.
func TestCheck_AnUnstoredFieldIsNotCounted(t *testing.T) {
	rep := serveprobe.Check([]db.Job{row(3)}, map[int64]jobview.Job{3: {}}, shown)
	for _, l := range serveprobe.Layers {
		for _, f := range serveprobe.Fields() {
			if c := rep.Counts[l][f]; c != (serveprobe.Count{}) {
				t.Errorf("%s %s = %+v, want nothing counted", l, f, c)
			}
		}
	}
}

// An unreadable derived list is no list — the projection serves nothing for it, so
// counting it as stored would report a gap the projection is right to have.
func TestCheck_UnreadableDerivedRequirementsAreNotStored(t *testing.T) {
	r := row(4)
	r.RequirementsDerived = []byte(`not json`)
	rep := serveprobe.Check([]db.Job{r}, map[int64]jobview.Job{4: {}}, shown)
	if got := rep.Counts[serveprobe.LayerProjection]["requirements"]; got != (serveprobe.Count{}) {
		t.Errorf("requirements = %+v, want nothing counted", got)
	}
}

// Most absent documents are search's own rules at work (measured 2026-10-31: 797 of
// 798, mostly no category). Only a row no rule withholds is Missing, so that one count
// is a real indexing loss instead of a share hidden among the deliberate ones.
func TestCheck_NamesWhyARowHasNoDocument(t *testing.T) {
	rows := []db.Job{row(5), row(6), row(7)}
	reasons := map[int64]search.WithholdReason{5: search.WithheldNoCategory, 6: search.WithheldUnseen}
	rep := serveprobe.Check(rows, map[int64]jobview.Job{}, func(j db.Job) search.WithholdReason { return reasons[j.ID] })

	want := map[string]int{string(search.WithheldNoCategory): 1, string(search.WithheldUnseen): 1, serveprobe.Missing: 1}
	for _, k := range serveprobe.UnindexedReasons {
		if rep.Unindexed[k] != want[k] {
			t.Errorf("unindexed[%s] = %d, want %d", k, rep.Unindexed[k], want[k])
		}
	}
}
