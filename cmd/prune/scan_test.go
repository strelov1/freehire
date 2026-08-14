package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeCandidates serves the catalogue one keyset page at a time, exactly as the query
// does, so a scan bug in paging shows up here rather than in production.
type fakeCandidates struct {
	rows  []db.PruneCandidatesRow
	pages int
}

func (f *fakeCandidates) PruneCandidates(_ context.Context, p db.PruneCandidatesParams) ([]db.PruneCandidatesRow, error) {
	f.pages++
	var out []db.PruneCandidatesRow
	for _, r := range f.rows {
		if r.ID > p.AfterID {
			out = append(out, r)
			if len(out) == int(p.PageSize) {
				break
			}
		}
	}
	return out, nil
}

type fakeDeleter struct {
	batches [][]int64
	rules   [][]string
	// extra is appended to each batch's result, standing in for the duplicate chain
	// the statement drags along.
	extra int64
	err   error
}

func (f *fakeDeleter) PruneJobs(_ context.Context, p db.PruneJobsParams) ([]int64, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.batches = append(f.batches, slices.Clone(p.Ids))
	f.rules = append(f.rules, slices.Clone(p.Rules))
	out := slices.Clone(p.Ids)
	if f.extra > 0 {
		out = append(out, f.extra)
	}
	return out, nil
}

type fakeIndex struct{ facet []int64 }

func (f *fakeIndex) SubmitJobDeletion(_ context.Context, ids []int64) error {
	f.facet = append(f.facet, ids...)
	return nil
}

func row(id int64, source, externalID, slug, title, category string) db.PruneCandidatesRow {
	return db.PruneCandidatesRow{
		ID: id, Source: source, ExternalID: externalID,
		CompanySlug: slug, Title: title, Category: category,
	}
}

func testRand() *rand.Rand { return rand.New(rand.NewPCG(1, 0)) }

// The change's central safety requirement, and the one the reviewer found untested:
// a company-scoped rule must not fire while the board is still in the source files.
// Nothing at crawl time knows a company's bucket, so what it removes is back within
// the hour — the deletion is pure loss.
func TestScanRefusesCompanyRulesWhileTheBoardIsListed(t *testing.T) {
	listedBoard := boards{
		listed:     map[boardKey]bool{{"greenhouse", "acme"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"acme": true}},
	}
	// A business role at a company that has never posted anything technical: the rule
	// matches, and only the guard stands between it and deletion.
	rows := []db.PruneCandidatesRow{
		row(1, "greenhouse", "acme:1", "acme", "Account Manager", "sales"),
	}
	ev := []db.CompanyTechEvidenceRow{{Source: "greenhouse", CompanySlug: "acme"}}

	p, err := scan(context.Background(), &fakeCandidates{rows: rows}, ev, listedBoard, 0, 10, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(p.targets) != 0 {
		t.Errorf("targeted %d rows, want 0 — the board is still crawled, so the deletion would undo itself", len(p.targets))
	}

	// Strike the board — the provider is still a crawled platform, this one board is
	// gone — and the same row becomes a target.
	retired := boards{
		listed:     map[boardKey]bool{{"greenhouse", "still-here"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"still-here": true}},
	}
	p, err = scan(context.Background(), &fakeCandidates{rows: rows}, ev, retired, 0, 10, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(p.targets) != 1 || p.targets[0].rule != ruleBusiness {
		t.Errorf("targets = %+v, want one %s — the board is gone, so the deletion sticks", p.targets, ruleBusiness)
	}
}

// The title rule needs the opposite: a posting whose board is not listed is not
// re-crawlable, so removing it cannot be undone and the rule must not apply.
func TestScanRefusesTitleRuleOnAnUnlistedBoard(t *testing.T) {
	brd := boards{
		listed:     map[boardKey]bool{{"greenhouse", "acme"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"acme": true}},
	}
	rows := []db.PruneCandidatesRow{
		row(1, "greenhouse", "acme:1", "acme", "Registered Nurse", ""),      // listed board
		row(2, "greenhouse", "imported:9", "other", "Registered Nurse", ""), // link-source import
	}

	p, err := scan(context.Background(), &fakeCandidates{rows: rows}, nil, brd, 0, 10, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(p.targets) != 1 || p.targets[0].id != 1 {
		t.Errorf("targets = %+v, want only the listed board's row — nothing re-crawls the import", p.targets)
	}
}

// The cap bounds what is collected, not what is counted: a capped run must still say
// how much work is left, or it reads as a finished campaign.
func TestScanCapsTargetsButKeepsCounting(t *testing.T) {
	brd := boards{
		listed:     map[boardKey]bool{{"greenhouse", "acme"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"acme": true}},
	}
	var rows []db.PruneCandidatesRow
	for i := int64(1); i <= 25; i++ {
		rows = append(rows, row(i, "greenhouse", "acme:x", "acme", "Line Cook", ""))
	}

	p, err := scan(context.Background(), &fakeCandidates{rows: rows}, nil, brd, 10, 5, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(p.targets) != 10 {
		t.Errorf("targets = %d, want 10 (the cap)", len(p.targets))
	}
	if p.matched != 25 {
		t.Errorf("matched = %d, want 25 — everything matching is counted, capped or not", p.matched)
	}
}

// The keyset has to advance past a page in which nothing matched, or the scan loops on
// the same page forever.
func TestScanWalksEveryPageAndTerminates(t *testing.T) {
	brd := boards{
		listed:     map[boardKey]bool{{"greenhouse", "acme"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"acme": true}},
	}
	var rows []db.PruneCandidatesRow
	for i := int64(1); i <= 12000; i++ {
		title := "Backend Engineer" // no rule matches
		if i == 11999 {
			title = "Line Cook" // the one match, on the last page
		}
		rows = append(rows, row(i, "greenhouse", "acme:x", "acme", title, ""))
	}

	src := &fakeCandidates{rows: rows}
	p, err := scan(context.Background(), src, nil, brd, 0, 5, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if p.matched != 1 {
		t.Errorf("matched = %d, want 1 — the match sits on the last page", p.matched)
	}
	// Three pages carry the rows and a fourth comes back empty, which is how the scan
	// learns it is done.
	if want := 12000/scanPage + 2; src.pages != want {
		t.Errorf("read %d pages, want %d", src.pages, want)
	}
}

// An unclassified job carries no is_tech at all; a job placed as non-technical carries
// false. The scan must preserve that distinction, because only the first is a target
// under the unknown rule.
func TestScanPreservesTheTriStateSignal(t *testing.T) {
	// The provider is a crawled platform; the rows' own board is retired.
	brd := boards{
		listed:     map[boardKey]bool{{"greenhouse", "still-here"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"still-here": true}},
	}
	rows := []db.PruneCandidatesRow{
		row(1, "greenhouse", "gone:1", "acme", "Team Member", ""),
		func() db.PruneCandidatesRow {
			r := row(2, "greenhouse", "gone:2", "acme", "Some Role", "")
			r.IsTech = pgtype.Bool{Bool: false, Valid: true}
			return r
		}(),
	}

	p, err := scan(context.Background(), &fakeCandidates{rows: rows}, nil, brd, 0, 10, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(p.targets) != 1 || p.targets[0].id != 1 {
		t.Errorf("targets = %+v, want only the unclassified row — a placed non-tech job matches no rule here", p.targets)
	}
}

// Every target must reach exactly one batch, including the last partial one. An
// off-by-one here silently under-deletes and nothing else would notice.
func TestDeleteTargetsBatchesEveryTargetOnce(t *testing.T) {
	p := newPlan(5, testRand())
	for i := range deleteBatch*2 + 7 {
		p.targets = append(p.targets, target{id: int64(i + 1), rule: ruleTitle})
	}
	del := &fakeDeleter{}
	idx := &fakeIndex{}

	if err := deleteTargets(context.Background(), del, idx, p); err != nil {
		t.Fatalf("deleteTargets: %v", err)
	}
	if len(del.batches) != 3 {
		t.Fatalf("batches = %d, want 3 (two full and one partial)", len(del.batches))
	}

	var seen []int64
	for i, b := range del.batches {
		if len(b) != len(del.rules[i]) {
			t.Errorf("batch %d has %d ids against %d rules — the query pairs them positionally", i, len(b), len(del.rules[i]))
		}
		seen = append(seen, b...)
	}
	if len(seen) != len(p.targets) {
		t.Errorf("sent %d ids, want %d", len(seen), len(p.targets))
	}
	slices.Sort(seen)
	if len(slices.Compact(seen)) != len(seen) {
		t.Error("an id was sent twice")
	}
}

// The index is cleaned with what the statement reports it actually deleted, not with
// what was asked for — the duplicate chain means those differ — since search is served
// straight from Meilisearch with no Postgres check.
func TestDeleteTargetsMirrorsWhatWasActuallyDeleted(t *testing.T) {
	p := newPlan(5, testRand())
	p.targets = []target{{id: 1, rule: ruleTitle}}
	del := &fakeDeleter{extra: 99} // the duplicate the chain walk pulled in
	idx := &fakeIndex{}

	if err := deleteTargets(context.Background(), del, idx, p); err != nil {
		t.Fatalf("deleteTargets: %v", err)
	}
	if !slices.Equal(idx.facet, []int64{1, 99}) {
		t.Errorf("facet index got %v, want [1 99] — the duplicate went too", idx.facet)
	}
	if p.deleted != 2 {
		t.Errorf("deleted = %d, want 2 — the report must count rows, not targets", p.deleted)
	}
}

// A failure partway leaves earlier batches committed, so the count of what did go has
// to survive the error for the report to be honest about it.
func TestDeleteTargetsKeepsTheCountOnFailure(t *testing.T) {
	p := newPlan(5, testRand())
	p.targets = []target{{id: 1, rule: ruleTitle}}
	del := &fakeDeleter{err: errors.New("connection lost")}

	if err := deleteTargets(context.Background(), del, &fakeIndex{}, p); err == nil {
		t.Fatal("a delete failure must be reported, not swallowed")
	}
	if p.deleted != 0 {
		t.Errorf("deleted = %d, want 0 — nothing committed in this run", p.deleted)
	}
}

// The gate the first prod dry run exposed: a source with no boards satisfies "the board
// is absent" for free, so the company-scoped rules fired on 2991 hand-curated Telegram
// vacancies that no crawl restores.
func TestScanNeverTouchesASourceWithNoBoards(t *testing.T) {
	brd := boards{
		listed:     map[boardKey]bool{{"greenhouse", "acme"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"acme": true}},
	}
	rows := []db.PruneCandidatesRow{
		row(1, "telegram", "jobnetworkng/34050/0", "acme", "Crane Operator", ""),
		row(2, "telegram", "huntmejob/33477/0", "acme", "Dispatcher", "management"),
		row(3, "telegram", "huggabletalents/2892/0", "acme", "Registered Nurse", ""),
	}

	p, err := scan(context.Background(), &fakeCandidates{rows: rows}, nil, brd, 0, 10, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(p.targets) != 0 {
		t.Fatalf("targeted %+v, want none — nothing re-crawls a Telegram vacancy", p.targets)
	}
	if len(p.refused) == 0 {
		t.Error("the source gate must be visible in the report, or the operator cannot tell it ran")
	}
}

// The database batch and the mirror batch answer different constraints and must not
// collapse into one number. Meilisearch runs one task per index at a time and the
// worker waits for each, so a mirror call per transaction serialises the whole run
// behind the search engine — measured on prod at 505 rows in eight minutes.
func TestDeleteTargetsMirrorsInLargeBatchesNotPerTransaction(t *testing.T) {
	p := newPlan(5, testRand())
	for i := range mirrorBatch + deleteBatch {
		p.targets = append(p.targets, target{id: int64(i + 1), rule: ruleTitle})
	}
	del := &fakeDeleter{}
	idx := &countingIndex{}

	if err := deleteTargets(context.Background(), del, idx, p); err != nil {
		t.Fatalf("deleteTargets: %v", err)
	}
	if want := len(p.targets) / deleteBatch; len(del.batches) != want {
		t.Errorf("database batches = %d, want %d — the transaction size is unchanged", len(del.batches), want)
	}
	// One flush at the threshold and one for the remainder.
	if idx.calls != 2 {
		t.Errorf("mirror calls = %d, want 2 — %d rows must not mean %d search tasks",
			idx.calls, len(p.targets), len(del.batches))
	}
	if idx.ids != len(p.targets) {
		t.Errorf("mirrored %d ids, want %d — buffering must not drop any", idx.ids, len(p.targets))
	}
}

// countingIndex records how many times the search engine was called, which is the
// quantity the fix is about.
type countingIndex struct{ calls, ids int }

func (c *countingIndex) SubmitJobDeletion(_ context.Context, ids []int64) error {
	c.calls++
	c.ids += len(ids)
	return nil
}

// The sample must describe what the run will delete. Feeding the reservoir the rows a
// cap excluded empties it of real titles precisely when a cap is in use — which is
// every first live run, and the one time the operator most needs to read it.
func TestScanSamplesOnlyWhatItWillDelete(t *testing.T) {
	brd := boards{
		listed:     map[boardKey]bool{{"greenhouse", "acme"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"acme": true}},
	}
	var rows []db.PruneCandidatesRow
	for i := int64(1); i <= 2000; i++ {
		rows = append(rows, row(i, "greenhouse", "acme:x", "acme", "Line Cook", ""))
	}

	p, err := scan(context.Background(), &fakeCandidates{rows: rows}, nil, brd, 10, 5, testRand())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(p.samples) != 5 {
		t.Fatalf("samples = %d, want 5", len(p.samples))
	}
	for _, s := range p.samples {
		if s == "" {
			t.Fatalf("the sample carries empty entries: %q — a capped run must still show real titles", p.samples)
		}
	}
}
