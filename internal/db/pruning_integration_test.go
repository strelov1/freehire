//go:build integration

// Integration tests for the catalogue-pruning queries. ResidualTitleGroups reports the
// most frequent word groups drawn from the titles of jobs that still carry no is_tech
// signal, so each pruning iteration can be aimed at the next real cluster and the
// remaining group measured. SQL behavior, verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The miner's vocabularies live in cmd/mine-titles and reach the query as parameters.
// These are the slices the cases below need, not the production lists.
var (
	testStop       = []string{"part", "time", "hiring", "urgent"}
	testConnectors = []string{"de", "of", "da"}
)

// residualJob upserts a job with the given external id, title and source, carrying the
// tri-state is_tech (nil = unclassified) that the miner selects on.
func residualJob(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext, title, source string, isTech *bool) Job {
	t.Helper()
	p := ingestParams(ext, title)
	p.Source = source
	if isTech != nil {
		p.IsTech = pgtype.Bool{Bool: *isTech, Valid: true}
	}
	j, err := ingestUpsert(ctx, New(pool), p)
	if err != nil {
		t.Fatalf("upsert %s: %v", ext, err)
	}
	return j
}

// groups runs the miner and returns the result keyed by group, so a case can assert on
// the one group it cares about without depending on the rest of the ranking.
func groups(ctx context.Context, t *testing.T, q *Queries, limit int32) map[string]ResidualTitleGroupsRow {
	t.Helper()
	rows, err := q.ResidualTitleGroups(ctx, ResidualTitleGroupsParams{
		StopWords:  testStop,
		Connectors: testConnectors,
		RowLimit:   limit,
	})
	if err != nil {
		t.Fatalf("ResidualTitleGroups: %v", err)
	}
	m := make(map[string]ResidualTitleGroupsRow, len(rows))
	for _, r := range rows {
		m[r.Grp] = r
	}
	return m
}

// The reason the miner groups by word group rather than whole title: boards append
// location and schedule, so one role splits into singleton titles that no whole-title
// grouping can reunite.
func TestResidualTitleGroupsClustersAcrossTitleNoise(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "a:1", "Personal Care Aide - on-call - Honolulu", "greenhouse", nil)
	residualJob(ctx, t, pool, "a:2", "personal care aide, Waipahu / Ewa Beach", "ukg", nil)
	residualJob(ctx, t, pool, "a:3", "Personal Care Aide (MWF 5am-8am)", "greenhouse", nil)

	g := groups(ctx, t, q, 50)

	got, ok := g["personal care"]
	if !ok {
		t.Fatalf("no cluster for %q; got %v", "personal care", slices.Sorted(maps(g)))
	}
	if got.Jobs != 3 {
		t.Errorf("jobs = %d, want 3 — the three titles are one role", got.Jobs)
	}
	sources := slices.Clone(got.Sources)
	slices.Sort(sources)
	if !slices.Equal(sources, []string{"greenhouse", "ukg"}) {
		t.Errorf("sources = %v, want [greenhouse ukg]", sources)
	}
	if _, ok := g["care aide"]; !ok {
		t.Error("the overlapping pair \"care aide\" must also be reported — both are usable anchors")
	}
}

// Romance titles carry the preposition inside the role name, so a three-word group
// bridging a connector is the only unit that can reproduce them — while the two-word
// fragments around that same connector are noise and must not be reported.
func TestResidualTitleGroupsBridgesConnectorsButNotEdges(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "b:1", "Operador de Caixa", "gupy", nil)
	residualJob(ctx, t, pool, "b:2", "operador de caixa - loja centro", "gupy", nil)
	residualJob(ctx, t, pool, "b:3", "Analista de Suporte", "gupy", nil)

	g := groups(ctx, t, q, 50)

	if got, ok := g["operador de caixa"]; !ok || got.Jobs != 2 {
		t.Errorf("operador de caixa = %+v (present=%v), want 2 jobs — a connector belongs mid-group", got, ok)
	}
	for _, fragment := range []string{"operador de", "de caixa", "analista de", "de suporte"} {
		if _, ok := g[fragment]; ok {
			t.Errorf("%q was reported — a connector at a group's edge makes it a fragment", fragment)
		}
	}
}

// Employment type and posting boilerplate dominated the ranking before the stop list;
// no group may be built from them at all.
func TestResidualTitleGroupsDropsStopWords(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "c:1", "Part Time Line Cook - urgent hiring", "greenhouse", nil)
	residualJob(ctx, t, pool, "c:2", "part time line cook", "greenhouse", nil)

	g := groups(ctx, t, q, 50)

	if _, ok := g["line cook"]; !ok {
		t.Error("the role pair \"line cook\" must survive")
	}
	for _, noise := range []string{"part time", "time line", "urgent hiring", "cook urgent"} {
		if _, ok := g[noise]; ok {
			t.Errorf("%q was reported — no group may contain a stop word", noise)
		}
	}
}

// Requisition numbers and shredded schedule notation are the noise class that made
// the ranking useless before the length and numeric rules. Neither rule is covered by
// the stop list, so without this case both predicates could be deleted unnoticed.
func TestResidualTitleGroupsDropsShortAndNumericTokens(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "i:1", "Line Helper 2024 req 12345 m w", "greenhouse", nil)

	g := groups(ctx, t, q, 50)

	if _, ok := g["line helper"]; !ok {
		t.Error("the role pair \"line helper\" must survive")
	}
	for _, noise := range []string{"helper 2024", "2024 req", "req 12345", "12345 m", "m w"} {
		if _, ok := g[noise]; ok {
			t.Errorf("%q was reported — numeric and under-three-character tokens cannot edge a group", noise)
		}
	}
}

// Adjacency must not span punctuation, or the report invents phrases that never
// occur: a term copied from "aide honolulu" would match none of the jobs that
// produced it, because the dictionary matches contiguous text.
func TestResidualTitleGroupsDoesNotBridgePunctuation(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "j:1", "Personal Care Aide - Honolulu", "greenhouse", nil)
	residualJob(ctx, t, pool, "j:2", "Driver, Nurse Aide", "greenhouse", nil)

	g := groups(ctx, t, q, 50)

	if _, ok := g["care aide"]; !ok {
		t.Error("a group within one run must still form")
	}
	for _, phantom := range []string{"aide honolulu", "driver nurse"} {
		if _, ok := g[phantom]; ok {
			t.Errorf("%q was reported — it spans a separator and occurs in no title", phantom)
		}
	}
}

// A missing vocabulary must not read as an exhausted catalogue. Passing NULL for a
// stop list makes every <> ALL(...) comparison NULL, which would filter every row and
// print the campaign's success message — the one wrong answer this report can give.
func TestResidualTitleGroupsSurvivesNilVocabularies(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "k:1", "Line Cook", "greenhouse", nil)

	rows, err := q.ResidualTitleGroups(ctx, ResidualTitleGroupsParams{RowLimit: 50})
	if err != nil {
		t.Fatalf("ResidualTitleGroups: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("nil vocabularies returned nothing — an empty report is the campaign's stop signal and must not be produced by a missing parameter")
	}
}

// A large share of the catalogue is Portuguese, Spanish and Russian. An ASCII-only
// tokenizer would split "Técnico" into "t" and "cnico" and lose the cluster entirely.
func TestResidualTitleGroupsTokenizesAccentedAndCyrillic(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "d:1", "Técnico de Manutenção", "gupy", nil)
	residualJob(ctx, t, pool, "d:2", "técnico de manutenção — noturno", "gupy", nil)
	residualJob(ctx, t, pool, "d:3", "Подсобный рабочий", "trudvsem", nil)

	g := groups(ctx, t, q, 50)

	if got, ok := g["técnico de manutenção"]; !ok || got.Jobs != 2 {
		t.Errorf("técnico de manutenção = %+v (present=%v), want 2 jobs — accents must survive tokenizing", got, ok)
	}
	if _, ok := g["подсобный рабочий"]; !ok {
		t.Error("Cyrillic titles must tokenize whole")
	}
}

// A group repeated inside one title is still one job. Counting occurrences instead of
// jobs would rank a single verbose posting above a real cluster.
func TestResidualTitleGroupsCountsJobsNotOccurrences(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	residualJob(ctx, t, pool, "e:1", "Line Cook / Line Cook Assistant", "greenhouse", nil)

	if got := groups(ctx, t, q, 50)["line cook"]; got.Jobs != 1 {
		t.Errorf("jobs = %d, want 1 — the pair occurs twice in one title", got.Jobs)
	}
}

// Only live, canonical, unclassified rows are worth a dictionary term. Every excluded
// row here shares the "line cook" title and sits on a second source, so a filter that
// leaked would inflate the count or add "ukg" to the sources rather than pass unnoticed.
func TestResidualTitleGroupsExcludesClassifiedClosedAndDuplicates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	yes, no := true, false
	live := residualJob(ctx, t, pool, "f:1", "Line Cook", "greenhouse", nil)
	residualJob(ctx, t, pool, "f:2", "Line Cook", "ukg", &no)
	residualJob(ctx, t, pool, "f:3", "Line Cook", "ukg", &yes)
	closed := residualJob(ctx, t, pool, "f:4", "Line Cook", "ukg", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closed.ID); err != nil {
		t.Fatalf("close f:4: %v", err)
	}
	dup := residualJob(ctx, t, pool, "f:5", "Line Cook", "ukg", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", live.ID, dup.ID); err != nil {
		t.Fatalf("mark f:5 duplicate: %v", err)
	}

	got := groups(ctx, t, q, 50)["line cook"]
	if got.Jobs != 1 {
		t.Errorf("jobs = %d, want 1 — classified, closed and duplicate rows must not count", got.Jobs)
	}
	if !slices.Equal(got.Sources, []string{"greenhouse"}) {
		t.Errorf("sources = %v, want [greenhouse] — every excluded row is on ukg, so its presence means one leaked", got.Sources)
	}
}

func TestResidualTitleGroupsHonoursLimit(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"g:1", "g:2", "g:3"} {
		residualJob(ctx, t, pool, ext, "Caregiver Assistant", "greenhouse", nil)
	}
	residualJob(ctx, t, pool, "h:1", "Dishwasher Helper", "greenhouse", nil)

	rows, err := q.ResidualTitleGroups(ctx, ResidualTitleGroupsParams{
		StopWords:  testStop,
		Connectors: testConnectors,
		RowLimit:   1,
	})
	if err != nil {
		t.Fatalf("ResidualTitleGroups: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 — the limit caps the report", len(rows))
	}
	if rows[0].Grp != "caregiver assistant" {
		t.Errorf("row = %q, want the busiest group %q", rows[0].Grp, "caregiver assistant")
	}
}

// maps yields a map's keys, for a readable failure message when a lookup misses.
func maps(m map[string]ResidualTitleGroupsRow) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// Archiving and deleting are one statement, so a deletion cannot happen without its
// audit row. These cases pin the parts that would otherwise drift: the duplicate
// cluster the restricting foreign key forces us to take along, the per-row rule, and
// the returned ids the caller needs to mirror the removal into the search index.
func TestPruneJobsArchivesWhatItDeletes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	cook := residualJob(ctx, t, pool, "p:1", "Line Cook", "greenhouse", nil)
	nurse := residualJob(ctx, t, pool, "p:2", "Registered Nurse", "ukg", nil)
	keep := residualJob(ctx, t, pool, "p:3", "Backend Engineer", "greenhouse", nil)

	got, err := q.PruneJobs(ctx, PruneJobsParams{
		Ids:   []int64{cook.ID, nurse.ID},
		Rules: []string{"title", "company"},
	})
	if err != nil {
		t.Fatalf("PruneJobs: %v", err)
	}

	slices.Sort(got)
	want := []int64{cook.ID, nurse.ID}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("returned ids = %v, want %v — the caller mirrors exactly these into the search index", got, want)
	}

	var live int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&live); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if live != 1 {
		t.Errorf("jobs remaining = %d, want 1 (the engineer)", live)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE id = $1", keep.ID).Scan(&live); err != nil {
		t.Fatalf("count kept: %v", err)
	}
	if live != 1 {
		t.Error("the untargeted job must survive")
	}

	var (
		archID                             int64
		src, ext, title, companySlug, rule string
		prunedAt                           time.Time
	)
	if err := pool.QueryRow(ctx,
		"SELECT id, source, external_id, title, company_slug, rule, pruned_at FROM pruned_jobs WHERE id = $1",
		nurse.ID).Scan(&archID, &src, &ext, &title, &companySlug, &rule, &prunedAt); err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if archID != nurse.ID || src != "ukg" || ext != "p:2" || title != "Registered Nurse" ||
		companySlug != "acme" || rule != "company" || prunedAt.IsZero() {
		t.Errorf("archive row = %d/%q/%q/%q/%q/%q/%v — every identifying field must be recorded",
			archID, src, ext, title, companySlug, rule, prunedAt)
	}
}

// jobs.duplicate_of is the one foreign key to jobs that restricts, so deleting a
// canonical row with a live duplicate would fail. The cluster goes together — the
// duplicates of a cook posting are cook postings — and the duplicate is archived too.
func TestPruneJobsTakesTheDuplicateCluster(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	canon := residualJob(ctx, t, pool, "d:1", "Line Cook", "greenhouse", nil)
	dup := residualJob(ctx, t, pool, "d:2", "Line Cook", "ukg", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", canon.ID, dup.ID); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}

	got, err := q.PruneJobs(ctx, PruneJobsParams{Ids: []int64{canon.ID}, Rules: []string{"title"}})
	if err != nil {
		t.Fatalf("PruneJobs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("deleted %d rows, want 2 — the duplicate must go with its canonical", len(got))
	}

	var archived int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pruned_jobs").Scan(&archived); err != nil {
		t.Fatalf("count archive: %v", err)
	}
	if archived != 2 {
		t.Errorf("archived = %d, want 2 — the duplicate is deleted, so it is audited too", archived)
	}
}

// A row named directly AND reachable as another target's duplicate must archive once,
// or the insert violates the archive's primary key and takes the whole batch down.
func TestPruneJobsArchivesAnOverlappingRowOnce(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	canon := residualJob(ctx, t, pool, "o:1", "Line Cook", "greenhouse", nil)
	dup := residualJob(ctx, t, pool, "o:2", "Line Cook", "ukg", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", canon.ID, dup.ID); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}

	got, err := q.PruneJobs(ctx, PruneJobsParams{
		Ids:   []int64{canon.ID, dup.ID},
		Rules: []string{"title", "title"},
	})
	if err != nil {
		t.Fatalf("PruneJobs: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("deleted %d rows, want 2 with no duplicate archive row", len(got))
	}
}

// Every other foreign key to jobs cascades or nulls, which is what lets the delete run
// at all. A user's saved job going with it is an accepted cost, but it must be the
// cascade doing it rather than the statement failing.
func TestPruneJobsCascadesUserInteractions(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	j := residualJob(ctx, t, pool, "u:1", "Line Cook", "greenhouse", nil)
	var userID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash) VALUES ('a@b.test', 'x') RETURNING id").Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"INSERT INTO user_jobs (user_id, job_id, saved_at) VALUES ($1, $2, now())", userID, j.ID); err != nil {
		t.Fatalf("save job: %v", err)
	}

	if _, err := q.PruneJobs(ctx, PruneJobsParams{Ids: []int64{j.ID}, Rules: []string{"title"}}); err != nil {
		t.Fatalf("PruneJobs must not be blocked by a user interaction: %v", err)
	}
	var left int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_jobs").Scan(&left); err != nil {
		t.Fatalf("count user_jobs: %v", err)
	}
	if left != 0 {
		t.Errorf("user_jobs rows = %d, want 0 (cascaded)", left)
	}
}

// duplicate_of can chain: RecomputeRoleDuplicatesForCompanies and the aggregator
// suppression both scope to open jobs, so a closed row's pointer is never repointed
// when its parent later becomes a duplicate itself. The prune scan reaches closed rows
// by design, so it meets those chains — and a one-level extension leaves the tail
// pointing at a deleted row, which the restricting foreign key rejects, taking the
// whole batch down.
func TestPruneJobsFollowsDuplicateChains(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	a := residualJob(ctx, t, pool, "c:1", "Line Cook", "greenhouse", nil)
	b := residualJob(ctx, t, pool, "c:2", "Line Cook", "ukg", nil)
	c := residualJob(ctx, t, pool, "c:3", "Line Cook", "workday", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", c.ID, b.ID); err != nil {
		t.Fatalf("link b -> c: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", b.ID, a.ID); err != nil {
		t.Fatalf("link a -> b: %v", err)
	}

	got, err := q.PruneJobs(ctx, PruneJobsParams{Ids: []int64{c.ID}, Rules: []string{"title"}})
	if err != nil {
		t.Fatalf("PruneJobs on a chain: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("deleted %d rows, want 3 — the whole chain goes with its root", len(got))
	}
}

// The guarantee the applications table exists for, asserted against the pruner itself
// rather than against a hand-written DELETE: removing a posting must leave the
// candidate's own record standing and must not move what the aggregates say about the
// employer — while the marks on the posting still go with it.
func TestPruneJobs_LeavesTheApplicationAndTheAggregates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "prune-guard@example.test", true)
	job := seedResponseJob(t, q, "prune-guard-1", "guardco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	seedReply(t, q, user, job, "prune-guard-reply")
	if _, err := q.SaveJob(ctx, SaveJobParams{UserID: user, JobID: job}); err != nil {
		t.Fatalf("save: %v", err)
	}
	appsBefore, answeredBefore := rebuildAnswered(t, q, "guardco")

	if _, err := q.PruneJobs(ctx, PruneJobsParams{Ids: []int64{job}, Rules: []string{"title"}}); err != nil {
		t.Fatalf("PruneJobs: %v", err)
	}

	var stage, notes *string
	var jobID *int64
	if err := pool.QueryRow(ctx,
		`SELECT stage, notes, job_id FROM applications WHERE user_id = $1`, user).
		Scan(&stage, &notes, &jobID); err != nil {
		t.Fatalf("the application did not survive the pruner: %v", err)
	}
	if jobID != nil {
		t.Errorf("job_id = %v after the prune, want NULL", *jobID)
	}
	if stage == nil || *stage != "applied" {
		t.Errorf("stage = %v, want applied — the candidate's record is not the campaign's to edit", stage)
	}

	var marks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_jobs WHERE user_id = $1`, user).Scan(&marks); err != nil {
		t.Fatalf("count marks: %v", err)
	}
	if marks != 0 {
		t.Errorf("%d interaction rows survived, want 0 — a bookmark goes with its posting", marks)
	}

	apps, answered := rebuildAnswered(t, q, "guardco")
	if apps != appsBefore || answered != answeredBefore {
		t.Errorf("aggregates moved: %d/%d before, %d/%d after — a removal must not change what is said about the employer",
			appsBefore, answeredBefore, apps, answered)
	}
}
