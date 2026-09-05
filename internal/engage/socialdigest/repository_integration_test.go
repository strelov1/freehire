//go:build integration

// Integration tests for the digest's SQL. Everything here is a property of the
// queries themselves — which postings are eligible, what the ranking column is, how
// the ledger deduplicates — and none of it is observable from a unit test: sqlc
// catches a misspelled column, but `NOT j.is_private` written as `j.is_private`
// compiles, generates, and publishes private postings to a public channel.
//
//	go test -tags=integration ./internal/engage/socialdigest/
//
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package socialdigest

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// seedJob inserts one posting. The zero value of every flag is the eligible case, so
// a test names only the column it is about.
type seed struct {
	slug        string
	title       string
	company     string
	companySlug string
	location    string
	remote      bool

	closed      bool
	duplicateOf *int64
	private     bool
	atsAbsent   bool

	// notTech and techUnknown are separate flags rather than one *bool, because the
	// three states are three different decisions and the query treats the last two the
	// same on purpose: `is_tech IS TRUE` excludes NULL as well as false.
	notTech     bool
	techUnknown bool
}

func seedJob(t *testing.T, pool *pgxpool.Pool, s seed) int64 {
	t.Helper()
	if s.title == "" {
		s.title = "Engineer"
	}
	if s.company == "" {
		s.company = "Acme"
	}
	if s.companySlug == "" {
		s.companySlug = "acme"
	}

	isTech := new(bool)
	*isTech = !s.notTech
	if s.techUnknown {
		isTech = nil
	}

	now := time.Now().UTC()
	var closedAt, atsAbsentAt *time.Time
	if s.closed {
		closedAt = &now
	}
	if s.atsAbsent {
		atsAbsentAt = &now
	}

	// duplicate_of goes in through duplicate_of_role, NOT directly: migration 0115
	// installed a trigger that DERIVES duplicate_of from the three owned columns and
	// overwrites whatever a writer put in it. Setting the column here would look like
	// it worked and leave the row non-duplicate.
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, location, remote,
		                   public_slug, closed_at, duplicate_of_role, is_private, ats_absent_at, is_tech)
		 VALUES ('test', $1, 'http://example.test', $2, $3, $4, $5, $6, $1, $7, $8, $9, $10, $11)
		 RETURNING id`,
		s.slug, s.title, s.company, s.companySlug, s.location, s.remote,
		closedAt, s.duplicateOf, s.private, atsAbsentAt, isTech).Scan(&id)
	if err != nil {
		t.Fatalf("seed job %q: %v", s.slug, err)
	}
	return id
}

func seedViews(t *testing.T, pool *pgxpool.Pool, jobID int64, d time.Time, uniques, pageUniques int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO job_daily_views (day, job_id, uniques, page_uniques) VALUES ($1, $2, $3, $4)`,
		d, jobID, uniques, pageUniques); err != nil {
		t.Fatalf("seed views: %v", err)
	}
}

func repo(pool *pgxpool.Pool) *PostgresRepository { return NewPostgresRepository(db.New(pool)) }

func TestLatestViewDayOnAnEmptyTable(t *testing.T) {
	pool := testdb.Pool(t)

	// max() over no rows is SQL NULL. That is "the rollup has produced nothing", which
	// the service turns into ErrNoViewData — NOT "the newest day is the zero time",
	// which would build a digest for year 1.
	got, ok, err := repo(pool).LatestViewDay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("got (%s, true), want no data", got)
	}
}

func TestLatestViewDayPicksTheNewest(t *testing.T) {
	pool := testdb.Pool(t)
	id := seedJob(t, pool, seed{slug: "acme-1"})
	seedViews(t, pool, id, day("2026-09-01"), 5, 5)
	seedViews(t, pool, id, day("2026-09-03"), 5, 5)
	seedViews(t, pool, id, day("2026-09-02"), 5, 5)

	got, ok, err := repo(pool).LatestViewDay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !got.Equal(day("2026-09-03")) {
		t.Errorf("got (%s, %v), want 2026-09-03", got.Format(DayLayout), ok)
	}
}

// The headline requirement of the whole change, and a pure SQL property: a posting
// with heavy API traffic must not outrank one with more page opens.
func TestTopPageViewedRanksOnPageUniquesNotUniques(t *testing.T) {
	pool := testdb.Pool(t)
	d := day("2026-09-03")

	crawled := seedJob(t, pool, seed{slug: "crawled", companySlug: "crawled-co"})
	read := seedJob(t, pool, seed{slug: "read", companySlug: "read-co"})
	seedViews(t, pool, crawled, d, 905, 5) // 900 API reads, 5 people
	seedViews(t, pool, read, d, 40, 40)    // 40 people

	got, err := repo(pool).TopPageViewed(context.Background(), d, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d postings, want 2", len(got))
	}
	if got[0].JobID != read {
		t.Errorf("the posting with more PAGE views must lead; got job %d first", got[0].JobID)
	}
	if got[0].PageUniques != 40 {
		t.Errorf("page uniques = %d, want 40", got[0].PageUniques)
	}
}

func TestTopPageViewedEligibility(t *testing.T) {
	pool := testdb.Pool(t)
	d := day("2026-09-03")

	canonical := seedJob(t, pool, seed{slug: "canonical", companySlug: "canon"})
	eligible := seedJob(t, pool, seed{slug: "eligible", companySlug: "ok-co"})

	// Each of these differs from `eligible` in exactly one column, so a predicate that
	// stopped working names itself.
	excluded := map[string]int64{
		"closed":          seedJob(t, pool, seed{slug: "closed", companySlug: "a", closed: true}),
		"duplicate":       seedJob(t, pool, seed{slug: "duplicate", companySlug: "b", duplicateOf: &canonical}),
		"private":         seedJob(t, pool, seed{slug: "private", companySlug: "c", private: true}),
		"ats-absent":      seedJob(t, pool, seed{slug: "ats-absent", companySlug: "d", atsAbsent: true}),
		"not tech":        seedJob(t, pool, seed{slug: "not-tech", companySlug: "e", notTech: true}),
		"tech unknown":    seedJob(t, pool, seed{slug: "tech-unknown", companySlug: "f", techUnknown: true}),
		"zero page views": seedJob(t, pool, seed{slug: "zero-page", companySlug: "g"}),
	}

	seedViews(t, pool, eligible, d, 100, 100)
	for name, id := range excluded {
		if name == "zero page views" {
			seedViews(t, pool, id, d, 900, 0) // crawler-only: 900 API reads, no people
			continue
		}
		// Seeded HIGHER than the eligible one, so a dropped predicate puts them on top
		// rather than hiding at the bottom of the list.
		seedViews(t, pool, id, d, 500, 500)
	}

	got, err := repo(pool).TopPageViewed(context.Background(), d, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].JobID != eligible {
		names := map[int64]string{eligible: "eligible"}
		for name, id := range excluded {
			names[id] = name
		}
		var leaked []string
		for _, p := range got {
			if p.JobID != eligible {
				leaked = append(leaked, names[p.JobID])
			}
		}
		t.Fatalf("only the eligible posting should be returned; leaked: %v", leaked)
	}
}

// Every field the post is rendered from comes out of this one query, so a column
// silently dropped from the SELECT would show up as a blank line in public.
func TestTopPageViewedCarriesTheRowThrough(t *testing.T) {
	pool := testdb.Pool(t)
	d := day("2026-09-03")
	id := seedJob(t, pool, seed{
		slug: "acme-go-7", title: "Senior Go Engineer",
		company: "Acme Ltd", companySlug: "acme", location: "Berlin", remote: true,
	})
	seedViews(t, pool, id, d, 60, 42)

	got, err := repo(pool).TopPageViewed(context.Background(), d, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d postings, want 1", len(got))
	}
	want := Posting{
		JobID: id, Slug: "acme-go-7", Title: "Senior Go Engineer",
		Company: "Acme Ltd", CompanySlug: "acme", Location: "Berlin", Remote: true,
		PageUniques: 42,
	}
	if got[0] != want {
		t.Errorf("got %+v,\nwant %+v", got[0], want)
	}
}

func TestTopPageViewedIsScopedToItsDay(t *testing.T) {
	pool := testdb.Pool(t)
	id := seedJob(t, pool, seed{slug: "acme-1"})
	seedViews(t, pool, id, day("2026-09-02"), 100, 100)

	got, err := repo(pool).TopPageViewed(context.Background(), day("2026-09-03"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d postings for a day with no rows, want 0", len(got))
	}
}

func TestLedgerPublishOnceAndQuarantine(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	r := repo(pool)

	d := day("2026-09-03")
	a := seedJob(t, pool, seed{slug: "a", companySlug: "a-co"})
	b := seedJob(t, pool, seed{slug: "b", companySlug: "b-co"})
	items := []Posting{{JobID: a}, {JobID: b}}

	published, err := r.PublishedForChannel(ctx, d, ChannelDiscord)
	if err != nil {
		t.Fatal(err)
	}
	if published {
		t.Fatal("nothing has been published yet")
	}

	if err := r.RecordPublished(ctx, d, ChannelDiscord, items); err != nil {
		t.Fatal(err)
	}

	published, err = r.PublishedForChannel(ctx, d, ChannelDiscord)
	if err != nil {
		t.Fatal(err)
	}
	if !published {
		t.Error("the day should now read as published on discord")
	}

	// Keyed on the channel, not on the day alone: a run that posted to one channel and
	// failed on another must retry only the second.
	other, err := r.PublishedForChannel(ctx, d, "other")
	if err != nil {
		t.Fatal(err)
	}
	if other {
		t.Error("a channel that has not published must not read as published")
	}

	// ON CONFLICT DO NOTHING: a retry that races itself must not fail the run over a
	// row that already says what we were about to say.
	if err := r.RecordPublished(ctx, d, ChannelDiscord, items); err != nil {
		t.Errorf("re-recording the same rows should be a no-op, got %v", err)
	}
}

func TestRecentlyDigestedWindowExcludesItsOwnDay(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	r := repo(pool)

	digestDay := day("2026-09-03")
	own := seedJob(t, pool, seed{slug: "own", companySlug: "own-co"})
	inside := seedJob(t, pool, seed{slug: "inside", companySlug: "in-co"})
	outside := seedJob(t, pool, seed{slug: "outside", companySlug: "out-co"})

	if err := r.RecordPublished(ctx, digestDay, ChannelDiscord, []Posting{{JobID: own}}); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordPublished(ctx, day("2026-09-01"), ChannelDiscord, []Posting{{JobID: inside}}); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordPublished(ctx, day("2026-08-26"), ChannelDiscord, []Posting{{JobID: outside}}); err != nil {
		t.Fatal(err)
	}

	got, err := r.RecentlyDigested(ctx, QuarantineSince(digestDay), digestDay)
	if err != nil {
		t.Fatal(err)
	}
	// The digest must not quarantine itself, or a second channel building the day a
	// first one already published reads back its own list and drops every item.
	if got[own] {
		t.Error("the digest's own day must not be in its quarantine set")
	}
	if !got[inside] {
		t.Error("a posting published two days ago should be quarantined")
	}
	// Eight days back is outside a seven-day window.
	if got[outside] {
		t.Error("a posting published before the window should be free to return")
	}
}
