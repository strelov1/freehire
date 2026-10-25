//go:build integration

// Integration tests for the buddy feed and per-application visibility.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

// event adds a ledger event to an application at a given age.
func event(t *testing.T, pool *pgxpool.Pool, u, appID int64, company, kind, signal string, hoursAgo int) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO application_events
		(user_id, application_id, company_slug, kind, signal, occurred_at, source)
		VALUES ($1, $2, $3, $4, $5, now() - make_interval(hours => $6), 'user') RETURNING id`,
		u, appID, company, kind, signal, hoursAgo).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newApp(t *testing.T, pool *pgxpool.Pool, u int64, company string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO applications (user_id, company_slug, applied_at) VALUES ($1, $2, now()) RETURNING id`, u, company).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func feedIDs(p buddy.FeedPage) []int64 {
	out := make([]int64, len(p.Entries))
	for i, e := range p.Entries {
		out[i] = e.ID
	}
	return out
}

func TestFeedIncludesOnlyAllowedEventsAtDayPrecision(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme Corp')`); err != nil {
		t.Fatal(err)
	}
	viewer, author := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test")
	joinWith(t, pool, svc, viewer, "backend", nil, nil)
	hA := joinWith(t, pool, svc, author, "backend", []string{"de"}, nil)
	// The author's events pre-date their joining: they still count.
	app := newApp(t, pool, author, "acme")
	applied := event(t, pool, author, app, "acme", "applied", "", 50)
	interview := event(t, pool, author, app, "acme", "stage_set", "interview", 40)
	offer := event(t, pool, author, app, "acme", "stage_set", "offer", 30)
	for _, sig := range []string{"accepted", "rejected", "withdrawn", "expired", "applied"} {
		event(t, pool, author, app, "acme", "stage_set", sig, 20)
	}
	event(t, pool, author, app, "acme", "employer_reply", "interview", 19)
	event(t, pool, author, app, "acme", "interview_scheduled", "", 18)
	retracted := event(t, pool, author, app, "acme", "applied", "", 17)
	if _, err := pool.Exec(ctx, `UPDATE application_events SET retracted_at = now() WHERE id = $1`, retracted); err != nil {
		t.Fatal(err)
	}
	// The viewer's own event never shows to the viewer.
	event(t, pool, viewer, newApp(t, pool, viewer, "acme"), "acme", "applied", "", 10)

	got, err := svc.Feed(ctx, viewer, buddy.FeedAll, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{offer, interview, applied}; !slices.Equal(feedIDs(got), want) {
		t.Fatalf("feed = %v, want %v", feedIDs(got), want)
	}
	e := got.Entries[2]
	if e.Handle != hA || e.Type != buddy.FeedApplied || e.Company != (buddy.Company{Slug: "acme", Name: "Acme Corp"}) ||
		e.Card.Country != "de" || e.JobTitle != nil {
		t.Fatalf("entry = %+v", e)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(e.Day) {
		t.Fatalf("day = %q", e.Day)
	}
	if got.Entries[0].Type != buddy.FeedOffer || got.Entries[1].Type != buddy.FeedInterview {
		t.Fatalf("types = %q %q", got.Entries[0].Type, got.Entries[1].Type)
	}
}

func TestFeedCarriesJobTitle(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, author := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	joinWith(t, pool, svc, author, "", nil, nil)
	var jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, title, company, url, description, public_slug)
		VALUES ('test', 'x1', 'Staff Go Engineer', 'Acme', 'https://example.test/x1', 'd', 'staff-go-x1') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	app := newApp(t, pool, author, "acme")
	ev := event(t, pool, author, app, "acme", "applied", "", 1)
	if _, err := pool.Exec(ctx, `UPDATE application_events SET job_id = $2 WHERE id = $1`, ev, jobID); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Feed(ctx, viewer, buddy.FeedAll, "", 10)
	if err != nil || len(got.Entries) != 1 || got.Entries[0].JobTitle == nil || *got.Entries[0].JobTitle != "Staff Go Engineer" {
		t.Fatalf("feed = %+v, %v", got, err)
	}
	// A pruned job leaves the event with a null title.
	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Feed(ctx, viewer, buddy.FeedAll, "", 10)
	if err != nil || len(got.Entries) != 1 || got.Entries[0].JobTitle != nil {
		t.Fatalf("after prune = %+v, %v", got, err)
	}
}

func TestFeedExcludesLeftBlockedAndNonMembers(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, left, byMe, meBy, outsider, ok := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "l@example.test"),
		seedUser(t, pool, "b1@example.test"), seedUser(t, pool, "b2@example.test"), seedUser(t, pool, "o@example.test"), seedUser(t, pool, "ok@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	for _, u := range []int64{left, byMe, meBy, ok} {
		joinWith(t, pool, svc, u, "", nil, nil)
	}
	var want int64
	for _, u := range []int64{left, byMe, meBy, outsider, ok} {
		id := event(t, pool, u, newApp(t, pool, u, "acme"), "acme", "applied", "", 5)
		if u == ok {
			want = id
		}
	}
	if err := svc.Leave(ctx, left); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buddy_blocks (blocker_id, blocked_id) VALUES ($1, $2), ($3, $1)`, viewer, byMe, meBy); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Feed(ctx, viewer, buddy.FeedAll, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(feedIDs(got), []int64{want}) {
		t.Fatalf("feed = %v, want [%d]", feedIDs(got), want)
	}
	// Seen from the blocked side, the viewer's events are gone too.
	viewerEv := event(t, pool, viewer, newApp(t, pool, viewer, "acme"), "acme", "applied", "", 1)
	other, err := svc.Feed(ctx, byMe, buddy.FeedAll, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(feedIDs(other), viewerEv) {
		t.Fatal("blocked member still sees the viewer's event")
	}
}

func TestFeedKeysetPaging(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, author := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	joinWith(t, pool, svc, author, "", nil, nil)
	app := newApp(t, pool, author, "acme")
	var all []int64
	for h := 1; h <= 5; h++ { // newest first
		all = append(all, event(t, pool, author, app, "acme", "applied", "", h))
	}
	// Two events share one instant: the id breaks the tie.
	tie := event(t, pool, author, app, "acme", "applied", "", 3)
	if _, err := pool.Exec(ctx, `UPDATE application_events SET occurred_at = (SELECT occurred_at FROM application_events WHERE id = $1) WHERE id = $2`, all[2], tie); err != nil {
		t.Fatal(err)
	}
	all = []int64{all[0], all[1], tie, all[2], all[3], all[4]}

	var seen []int64
	cursor := ""
	for range 10 {
		p, err := svc.Feed(ctx, viewer, buddy.FeedAll, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, feedIDs(p)...)
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
	}
	if !slices.Equal(seen, all) {
		t.Fatalf("paged = %v, want %v", seen, all)
	}
	if _, err := svc.Feed(ctx, viewer, buddy.FeedAll, "garbage", 2); !errors.Is(err, buddy.ErrBadFeedRequest) {
		t.Fatalf("bad cursor: %v", err)
	}
}

func TestFeedSimilarModeRestrictsToTopMembers(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, near := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "n@example.test")
	joinWith(t, pool, svc, viewer, "backend", nil, nil)
	hNear := joinWith(t, pool, svc, near, "backend", nil, nil)
	nearEv := event(t, pool, near, newApp(t, pool, near, "acme"), "acme", "applied", "", 2)

	// A viewer's own similar list is the author set: a member the list leaves out
	// contributes nothing, whatever the all feed shows. Remove the only candidate by
	// blocking and the similar feed empties.
	sim, err := svc.Feed(ctx, viewer, buddy.FeedSimilar, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sim.Entries) != 1 || sim.Entries[0].ID != nearEv || sim.Entries[0].Handle != hNear {
		t.Fatalf("similar = %+v", sim.Entries)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buddy_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, viewer, near); err != nil {
		t.Fatal(err)
	}
	sim, err = svc.Feed(ctx, viewer, buddy.FeedSimilar, "", 10)
	if err != nil || len(sim.Entries) != 0 {
		t.Fatalf("similar after block = %+v, %v", sim.Entries, err)
	}
}

func TestFeedHideShowApplication(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, author, stranger := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test"), seedUser(t, pool, "s@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	joinWith(t, pool, svc, author, "", nil, nil)
	app := newApp(t, pool, author, "acme")
	other := newApp(t, pool, author, "globex")
	keep := event(t, pool, author, other, "globex", "applied", "", 2)
	hidden := event(t, pool, author, app, "acme", "applied", "", 3)
	event(t, pool, author, app, "acme", "stage_set", "offer", 1)

	if err := svc.HideApplication(ctx, author, app); err != nil {
		t.Fatal(err)
	}
	if err := svc.HideApplication(ctx, author, app); err != nil {
		t.Fatalf("hiding twice: %v", err)
	}
	got, _ := svc.Feed(ctx, viewer, buddy.FeedAll, "", 10)
	if !slices.Equal(feedIDs(got), []int64{keep}) {
		t.Fatalf("after hide feed = %v, want [%d]", feedIDs(got), keep)
	}
	ids, err := svc.HiddenApplications(ctx, author)
	if err != nil || !slices.Equal(ids, []int64{app}) {
		t.Fatalf("hidden = %v, %v", ids, err)
	}
	if err := svc.ShowApplication(ctx, author, app); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Feed(ctx, viewer, buddy.FeedAll, "", 10)
	if len(got.Entries) != 3 || !slices.Contains(feedIDs(got), hidden) {
		t.Fatalf("after show feed = %v", feedIDs(got))
	}
	if ids, _ := svc.HiddenApplications(ctx, author); len(ids) != 0 {
		t.Fatalf("hidden after show = %v", ids)
	}

	// Another member's application is not yours to hide; a non-member cannot at all.
	if err := svc.HideApplication(ctx, viewer, app); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("hide foreign = %v", err)
	}
	if err := svc.HideApplication(ctx, author, 999999); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("hide unknown = %v", err)
	}
	if err := svc.HideApplication(ctx, stranger, app); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("hide by non-member = %v", err)
	}
	if _, err := svc.Feed(ctx, stranger, buddy.FeedAll, "", 10); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("feed by non-member = %v", err)
	}
}

// The feed query must be able to use application_events_feed_idx: its WHERE has to imply
// the index predicate. With sequential scans disabled the planner picks it when it can.
func TestFeedQueryUsesTheFeedIndex(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(ctx, `EXPLAIN SELECT e.id FROM application_events e
		WHERE e.kind IN ('applied', 'stage_set') AND e.retracted_at IS NULL
		  AND (e.kind = 'applied' OR e.signal IN ('interview', 'offer'))
		ORDER BY e.occurred_at DESC, e.id DESC LIMIT 21`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if !strings.Contains(plan.String(), "application_events_feed_idx") {
		t.Fatalf("plan does not use the feed index:\n%s", plan.String())
	}
}
