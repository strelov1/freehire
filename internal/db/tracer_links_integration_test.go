//go:build integration

// Integration tests for the cv-tracer-links SQL semantics: the idempotent mint, whose whole point
// is that re-rendering an unchanged CV reuses its tokens, and the owner scoping that keeps a
// stranger from minting against — or reading — someone else's CV. Both are ON CONFLICT / JOIN
// behaviour and can only be verified against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func hashOf(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

// mint is the call the renderer makes for one link on one download.
func mint(t *testing.T, q *Queries, user int64, cv uuid.UUID, token, path, url string) (string, error) {
	t.Helper()
	return q.UpsertTracerLink(context.Background(), UpsertTracerLinkParams{
		CvID:            cv,
		UserID:          user,
		Token:           token,
		SourcePath:      path,
		DestinationUrl:  url,
		DestinationHash: hashOf(url),
	})
}

func seedTracerCV(t *testing.T, pool *pgxpool.Pool, email string) (int64, uuid.UUID) {
	t.Helper()
	user := seedCVUser(t, pool, email)
	cv, err := New(pool).CreateCV(context.Background(), CreateCVParams{
		UserID: user, Title: "General", TemplateID: "classic-ats", Data: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("seed cv: %v", err)
	}
	return user, cv.ID
}

// The PDF is re-rendered on every download, so this runs on every download. If it were not
// idempotent, three downloads would leave three tokens for one link and the counts would scatter
// across them — the feature would report a third of the truth and look merely unpopular.
func TestReMintingAnUnchangedLinkReusesItsToken(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	user, cv := seedTracerCV(t, pool, "remint@example.com")

	first, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("first mint: %v", err)
	}
	// A second download offers a freshly generated token; the row already there must win.
	second, err := mint(t, q, user, cv, "acme-bbbbb", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("second mint: %v", err)
	}
	if first != second {
		t.Errorf("re-mint produced %q, want the existing %q", second, first)
	}

	var rows int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM cv_tracer_links WHERE cv_id = $1`, cv).Scan(&rows); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if rows != 1 {
		t.Errorf("cv_tracer_links has %d rows, want 1", rows)
	}
}

// A candidate who edits a link has changed where the CV points, but the PDFs already sent still
// point at the old destination — and the recruiter holding one is entitled to arrive somewhere.
func TestChangingADestinationMintsAgainAndLeavesTheOldTokenWorking(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	user, cv := seedTracerCV(t, pool, "changed@example.com")
	ctx := context.Background()

	old, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("mint old: %v", err)
	}
	fresh, err := mint(t, q, user, cv, "acme-bbbbb", "header.links[0]", "https://github.com/ada-new")
	if err != nil {
		t.Fatalf("mint new: %v", err)
	}
	if old == fresh {
		t.Fatalf("a changed destination reused token %q", old)
	}

	resolved, err := q.TracerLinkByToken(ctx, old)
	if err != nil {
		t.Fatalf("resolve the superseded token: %v", err)
	}
	if resolved.DestinationUrl != "https://github.com/ada" {
		t.Errorf("superseded token now resolves to %q, want the destination it was minted for",
			resolved.DestinationUrl)
	}
	if resolved.OwnerID != user {
		t.Errorf("resolved owner = %d, want %d", resolved.OwnerID, user)
	}
}

// "Clicked the link in the header" and "clicked through to that project" are different events.
// Merging them would erase the only distinction the per-link panel exists to show.
func TestOneDestinationAtTwoPositionsGetsTwoTokens(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	user, cv := seedTracerCV(t, pool, "twice@example.com")

	header, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("mint header: %v", err)
	}
	project, err := mint(t, q, user, cv, "acme-bbbbb", "projects[0].link", "https://github.com/ada")
	if err != nil {
		t.Fatalf("mint project: %v", err)
	}
	if header == project {
		t.Errorf("both positions share token %q", header)
	}
}

func TestMintingAgainstAForeignCVWritesNothing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	_, cv := seedTracerCV(t, pool, "owner@example.com")
	stranger := seedCVUser(t, pool, "stranger@example.com")

	_, err := mint(t, q, stranger, cv, "acme-aaaaa", "header.links[0]", "https://evil.example/x")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("mint against a foreign CV: err = %v, want pgx.ErrNoRows", err)
	}

	var rows int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM cv_tracer_links`).Scan(&rows); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if rows != 0 {
		t.Errorf("a stranger minted %d rows against someone else's CV", rows)
	}
}

// Deleting a CV is the right to erase one's own data, and the schema is where that lives: no
// delete path has to remember to sweep the tokens or the clicks.
func TestDeletingACVTakesItsTokensAndClicks(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()
	user, cv := seedTracerCV(t, pool, "erase@example.com")

	token, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	link, err := q.TracerLinkByToken(ctx, token)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := q.RecordTracerClick(ctx, RecordTracerClickParams{TracerLinkID: link.ID}); err != nil {
		t.Fatalf("record click: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM cvs WHERE id = $1`, cv); err != nil {
		t.Fatalf("delete cv: %v", err)
	}

	if _, err := q.TracerLinkByToken(ctx, token); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("token still resolves after the CV was deleted: err = %v", err)
	}
	var clicks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cv_link_clicks`).Scan(&clicks); err != nil {
		t.Fatalf("count clicks: %v", err)
	}
	if clicks != 0 {
		t.Errorf("%d clicks survived the CV they belonged to", clicks)
	}
}

// The panel reports what somebody else did with the CV. A click the owner made while checking their
// own PDF is kept — the history stays complete — but must not be reported back to them as interest.
func TestStatsSeparateAutomatedAndOwnerClicksFromTheRest(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()
	user, cv := seedTracerCV(t, pool, "stats@example.com")

	token, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	link, err := q.TracerLinkByToken(ctx, token)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, c := range []RecordTracerClickParams{
		{TracerLinkID: link.ID, VisitorHash: "reader-one"},
		{TracerLinkID: link.ID, VisitorHash: "reader-one"}, // same person, twice
		{TracerLinkID: link.ID, VisitorHash: "reader-two"},
		{TracerLinkID: link.ID, VisitorHash: "", IsLikelyBot: true},
		{TracerLinkID: link.ID, VisitorHash: "the-owner", IsOwner: true},
		// No salt configured: identifiable as a click, not as a visitor.
		{TracerLinkID: link.ID, VisitorHash: ""},
	} {
		if err := q.RecordTracerClick(ctx, c); err != nil {
			t.Fatalf("record click: %v", err)
		}
	}

	stats, err := q.ListTracerLinkStats(ctx, ListTracerLinkStatsParams{CvID: cv, UserID: user})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("stats returned %d rows, want 1", len(stats))
	}
	got := stats[0]
	if got.Clicks != 4 {
		t.Errorf("clicks = %d, want 4 (three identified plus the unsalted one; not the bot, not the owner)", got.Clicks)
	}
	if got.BotClicks != 1 {
		t.Errorf("bot_clicks = %d, want 1", got.BotClicks)
	}
	if got.DistinctVisitors != 2 {
		t.Errorf("distinct_visitors = %d, want 2 — an empty hash is not a visitor", got.DistinctVisitors)
	}
}

func TestStatsAreEmptyForACVWithNoTracing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	user, cv := seedTracerCV(t, pool, "untraced@example.com")

	stats, err := q.ListTracerLinkStats(context.Background(), ListTracerLinkStatsParams{CvID: cv, UserID: user})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("stats = %+v, want none", stats)
	}
}

func TestStatsOfAForeignCVAreNotReadable(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	user, cv := seedTracerCV(t, pool, "mine@example.com")
	stranger := seedCVUser(t, pool, "nosy@example.com")

	if _, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada"); err != nil {
		t.Fatalf("mint: %v", err)
	}
	stats, err := q.ListTracerLinkStats(context.Background(), ListTracerLinkStatsParams{CvID: cv, UserID: stranger})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("a stranger read %d of someone else's traced links", len(stats))
	}
}

// A recruiter opening a CV is not a reply. If the click fed the last-activity derivation, the
// silence badge would clear at the moment it matters most — they read it and still said nothing.
// This is the same rule that keeps user_jobs.followed_up_at out of that derivation.
func TestOpeningACVLeavesTheApplicationsSilenceUntouched(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	user, cv := seedTracerCV(t, pool, "silence@example.com")
	job := insertJob(t, pool, "silence-job-1")
	if _, err := pool.Exec(ctx, `UPDATE cvs SET job_id = $1 WHERE id = $2`, job, cv); err != nil {
		t.Fatalf("bind cv to job: %v", err)
	}
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	before, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: user, Filter: "applied", Limit: 10})
	if err != nil || len(before) != 1 {
		t.Fatalf("list before: %v (%d rows)", err, len(before))
	}

	token, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	link, err := q.TracerLinkByToken(ctx, token)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := q.RecordTracerClick(ctx, RecordTracerClickParams{TracerLinkID: link.ID, VisitorHash: "reader"}); err != nil {
		t.Fatalf("record click: %v", err)
	}
	if err := q.TouchCVLastClick(ctx, link.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}

	after, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: user, Filter: "applied", Limit: 10})
	if err != nil || len(after) != 1 {
		t.Fatalf("list after: %v (%d rows)", err, len(after))
	}
	if !after[0].CvOpenedAt.Valid {
		t.Error("cv_opened_at is null after a countable click")
	}
	if after[0].LastActivityAt != before[0].LastActivityAt {
		t.Errorf("last_activity_at moved from %v to %v — opening a CV is not a reply",
			before[0].LastActivityAt, after[0].LastActivityAt)
	}
}

// The consent toggle is owner-scoped like every other write to a CV.
func TestSettingTheTracerToggleIsOwnerScoped(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()
	user, cv := seedTracerCV(t, pool, "toggle@example.com")
	stranger := seedCVUser(t, pool, "not-mine@example.com")

	n, err := q.SetCVTracerLinks(ctx, SetCVTracerLinksParams{ID: cv, UserID: stranger, TracerLinksEnabled: true})
	if err != nil {
		t.Fatalf("stranger toggle: %v", err)
	}
	if n != 0 {
		t.Errorf("a stranger updated %d rows of someone else's CV", n)
	}

	if n, err := q.SetCVTracerLinks(ctx, SetCVTracerLinksParams{ID: cv, UserID: user, TracerLinksEnabled: true}); err != nil || n != 1 {
		t.Fatalf("owner toggle: n=%d err=%v", n, err)
	}
}

// Clicks expire; tokens do not. An old PDF must keep redirecting long after the clicks behind it
// have aged out — the recruiter holding it did nothing to deserve a dead link.
func TestExpiringClicksKeepsTheTokensThatOutlivedThem(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()
	user, cv := seedTracerCV(t, pool, "retention@example.com")

	token, err := mint(t, q, user, cv, "acme-aaaaa", "header.links[0]", "https://github.com/ada")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	link, err := q.TracerLinkByToken(ctx, token)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, age := range []string{"200 days", "179 days", "1 day"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO cv_link_clicks (tracer_link_id, clicked_at) VALUES ($1, now() - $2::interval)`,
			link.ID, age); err != nil {
			t.Fatalf("seed click aged %s: %v", age, err)
		}
	}

	deleted, err := q.DeleteExpiredTracerClicks(ctx, pgtype.Interval{Days: 180, Valid: true})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 1 {
		t.Errorf("swept %d clicks, want 1 — only the one past 180 days", deleted)
	}

	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cv_link_clicks`).Scan(&left); err != nil {
		t.Fatalf("count: %v", err)
	}
	if left != 2 {
		t.Errorf("%d clicks left, want 2", left)
	}
	if _, err := q.TracerLinkByToken(ctx, token); err != nil {
		t.Errorf("the token stopped resolving when its oldest clicks aged out: %v", err)
	}
}
