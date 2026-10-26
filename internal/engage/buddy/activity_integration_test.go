//go:build integration

// Integration tests for a member's activity grid. Run with:
// go test -tags=integration ./internal/engage/buddy/ -run Activity
package buddy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

// eventDaysAgo adds a ledger event at noon UTC daysAgo calendar days before today.
func eventDaysAgo(t *testing.T, pool *pgxpool.Pool, u, appID int64, kind, signal string, daysAgo int) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO application_events
		(user_id, application_id, company_slug, kind, signal, occurred_at, source)
		VALUES ($1, $2, 'acme', $3, $4,
		        (date_trunc('day', now() AT TIME ZONE 'UTC') - make_interval(days => $5) + interval '12 hours') AT TIME ZONE 'UTC',
		        'user') RETURNING id`,
		u, appID, kind, signal, daysAgo).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func utcDaysAgo(n int) string {
	return time.Now().UTC().AddDate(0, 0, -n).Format(time.DateOnly)
}

func activityTotal(days []buddy.ActivityDay) int {
	n := 0
	for _, d := range days {
		n += d.Count
	}
	return n
}

func TestMemberActivityCountsPerDay(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, author := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	hA := joinWith(t, pool, svc, author, "", nil, nil)
	app := newApp(t, pool, author, "acme")
	eventDaysAgo(t, pool, author, app, "applied", "", 2)
	eventDaysAgo(t, pool, author, app, "applied", "", 2)
	eventDaysAgo(t, pool, author, app, "stage_set", "interview", 2)
	eventDaysAgo(t, pool, author, app, "stage_set", "offer", 10)
	eventDaysAgo(t, pool, author, app, "applied", "", buddy.ActivityDays-1) // the window's first day
	eventDaysAgo(t, pool, author, app, "applied", "", buddy.ActivityDays)   // a day too old

	got, err := svc.MemberActivity(ctx, viewer, hA)
	if err != nil {
		t.Fatal(err)
	}
	want := []buddy.ActivityDay{
		{Day: utcDaysAgo(buddy.ActivityDays - 1), Count: 1},
		{Day: utcDaysAgo(10), Count: 1},
		{Day: utcDaysAgo(2), Count: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("activity = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("activity = %+v, want %+v", got, want)
		}
	}
}

// The grid restates the feed's visibility predicate (sqlc cannot share a WHERE clause), so
// this is the guard against the two drifting: every exclusion the feed applies is seeded,
// and the grid must count exactly the entries the member's feed lists.
func TestMemberActivityMatchesMemberFeed(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, author, other := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test"), seedUser(t, pool, "o@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	hA := joinWith(t, pool, svc, author, "", nil, nil)
	joinWith(t, pool, svc, other, "", nil, nil)
	var privateJob int64
	if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, title, company, url, description, public_slug, is_private)
		VALUES ('test', 'p1', 'Secret Role', 'Hush', 'https://example.test/p1', 'd', 'secret-p1', true) RETURNING id`).Scan(&privateJob); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	app := newApp(t, pool, author, "acme")
	// Counted: an application and the two stage moves the feed shows.
	eventDaysAgo(t, pool, author, app, "applied", "", 3)
	eventDaysAgo(t, pool, author, app, "stage_set", "interview", 2)
	eventDaysAgo(t, pool, author, app, "stage_set", "offer", 1)
	// Not counted: every other stage, every other kind, a retraction, a hidden application,
	// a private posting, and another author.
	for _, sig := range []string{"accepted", "rejected", "withdrawn", "expired", "applied"} {
		eventDaysAgo(t, pool, author, app, "stage_set", sig, 1)
	}
	eventDaysAgo(t, pool, author, app, "follow_up_sent", "", 1)
	eventDaysAgo(t, pool, author, app, "employer_reply", "interview", 1)
	eventDaysAgo(t, pool, author, app, "interview_scheduled", "", 1)
	retracted := eventDaysAgo(t, pool, author, app, "applied", "", 1)
	if _, err := pool.Exec(ctx, `UPDATE application_events SET retracted_at = now() WHERE id = $1`, retracted); err != nil {
		t.Fatal(err)
	}
	hiddenApp := newApp(t, pool, author, "globex")
	eventDaysAgo(t, pool, author, hiddenApp, "applied", "", 1)
	if err := svc.HideApplication(ctx, author, hiddenApp); err != nil {
		t.Fatal(err)
	}
	private := eventDaysAgo(t, pool, author, newApp(t, pool, author, "hush"), "applied", "", 1)
	if _, err := pool.Exec(ctx, `UPDATE application_events SET job_id = $2 WHERE id = $1`, private, privateJob); err != nil {
		t.Fatal(err)
	}
	eventDaysAgo(t, pool, other, newApp(t, pool, other, "acme"), "applied", "", 1)

	days, err := svc.MemberActivity(ctx, viewer, hA)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := svc.MemberFeed(ctx, viewer, hA, "", buddy.MaxFeedLimit)
	if err != nil {
		t.Fatal(err)
	}
	if feed.NextCursor != "" {
		t.Fatal("the feed must fit one page for this comparison")
	}
	if total := activityTotal(days); total != 3 || total != len(feed.Entries) {
		t.Fatalf("grid total = %d, feed entries = %d, want both 3", total, len(feed.Entries))
	}
	// Day by day, too: the grid's day is the feed entry's day.
	perDay := map[string]int{}
	for _, e := range feed.Entries {
		perDay[e.Day]++
	}
	for _, d := range days {
		if perDay[d.Day] != d.Count {
			t.Fatalf("day %s: grid %d, feed %d", d.Day, d.Count, perDay[d.Day])
		}
	}
}

// A handle the viewer may not see answers exactly as Profile does: not found.
func TestMemberActivityUnavailableHandles(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, byMe, meBy, left := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "b1@example.test"),
		seedUser(t, pool, "b2@example.test"), seedUser(t, pool, "l@example.test")
	hV := joinWith(t, pool, svc, viewer, "", nil, nil)
	hByMe := joinWith(t, pool, svc, byMe, "", nil, nil)
	hMeBy := joinWith(t, pool, svc, meBy, "", nil, nil)
	hLeft := joinWith(t, pool, svc, left, "", nil, nil)
	for _, u := range []int64{viewer, byMe, meBy, left} {
		eventDaysAgo(t, pool, u, newApp(t, pool, u, "acme"), "applied", "", 1)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2), ($3, $1)`, viewer, byMe, meBy); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, left); err != nil {
		t.Fatal(err)
	}
	for name, h := range map[string]string{"own": hV, "blocked by viewer": hByMe, "blocked the viewer": hMeBy, "left": hLeft, "unknown": "nobody-here"} {
		if _, err := svc.MemberActivity(ctx, viewer, h); !errors.Is(err, buddy.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}
