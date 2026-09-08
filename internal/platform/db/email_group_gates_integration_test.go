//go:build integration

// Integration tests for the three email-group gates that migration 0152 split apart.
// They are SQL semantics — three predicates that read the same table two opposite
// ways on purpose — so nothing short of a real Postgres can check them. Run with:
// go test -tags=integration ./internal/platform/db/
//
// What is being pinned is the split itself, and the fact that a missing settings row
// means different things per group:
//
//	group     column                gate shape           no settings row means
//	--------  --------------------  -------------------  ---------------------
//	activity  enabled               JOIN ... AND enabled  do NOT send
//	news      news_email_enabled    COALESCE(..., true)   DO send
//	alerts    alerts_email_enabled  COALESCE(..., true)   DO send
//
// Before 0152 the first two shared one column, so declining campaigns also stopped
// somebody's application reminders — and an account with no row could not decline
// campaigns at all, because the only thing that creates the row is a page behind the
// login. That is what a subscriber wrote in about.
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setEmailGroups writes one account's notification rule. A nil pointer leaves that
// column at its default, which is how a test says "this account has a row but never
// touched this switch".
func setEmailGroups(t *testing.T, pool *pgxpool.Pool, userID int64, activity, alerts, news *bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO notification_settings (user_id, enabled, channels, alerts_email_enabled, news_email_enabled)
		 VALUES ($1, COALESCE($2, false), '{email}', COALESCE($3, true), COALESCE($4, true))
		 ON CONFLICT (user_id) DO UPDATE
		   SET enabled              = EXCLUDED.enabled,
		       alerts_email_enabled = EXCLUDED.alerts_email_enabled,
		       news_email_enabled   = EXCLUDED.news_email_enabled`,
		userID, activity, alerts, news)
	if err != nil {
		t.Fatalf("set notification settings: %v", err)
	}
}

func verifyEmail(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET email_verified = true WHERE id = $1`, userID); err != nil {
		t.Fatalf("verify email: %v", err)
	}
}

func ptr(b bool) *bool { return &b }

// The headline case, and the complaint that started this change: somebody who has
// never opened the settings page still gets campaigns — and must be able to stop
// them without gaining anything else or losing anything else.
func TestNewsGate_IsIndependentOfTheActivityFlag(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)

	noRow := insertUser(t, pool, "no-row@example.test")
	declinedNews := insertUser(t, pool, "declined-news@example.test")
	declinedActivity := insertUser(t, pool, "declined-activity@example.test")
	for _, id := range []int64{noRow, declinedNews, declinedActivity} {
		verifyEmail(t, pool, id)
	}
	// A row that says "no campaigns" but leaves the activity flag alone.
	setEmailGroups(t, pool, declinedNews, nil, nil, ptr(false))
	// And a row that says "no lifecycle mail" but leaves campaigns alone — the
	// case that used to be impossible to express.
	setEmailGroups(t, pool, declinedActivity, ptr(false), nil, nil)

	rows, err := q.ListBroadcastCandidates(ctx, ListBroadcastCandidatesParams{
		Campaign: "discord-invite", MaxRows: 100,
	})
	if err != nil {
		t.Fatalf("ListBroadcastCandidates: %v", err)
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}

	if !got[noRow] {
		t.Error("an account that never opened the settings page must still receive campaigns")
	}
	if got[declinedNews] {
		t.Error("an account that declined campaigns still received one")
	}
	if !got[declinedActivity] {
		t.Error("declining lifecycle mail stopped a campaign; that is the coupling 0152 removed")
	}
}

// The mirror: the nudge gate must not have moved. It is an inner join, so a missing
// row means no nudges, and the news switch must not change that either way.
func TestActivityGate_StillOptsInByRowAndIgnoresTheNewsSwitch(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	truncateSubs(t, pool)

	noRow := insertUser(t, pool, "activity-no-row@example.test")
	newsOffOnly := insertUser(t, pool, "activity-news-off@example.test")
	activityOn := insertUser(t, pool, "activity-on@example.test")
	setEmailGroups(t, pool, newsOffOnly, ptr(true), nil, ptr(false))
	setEmailGroups(t, pool, activityOn, ptr(true), nil, nil)

	// The predicate every nudge query shares, read directly: the queries themselves
	// need application rows and timers this test has no reason to build, and what is
	// under test is the join shape, not the schedule.
	rows, err := pool.Query(ctx,
		`SELECT u.id FROM users u
		 JOIN notification_settings ns ON ns.user_id = u.id AND ns.enabled
		 ORDER BY u.id`)
	if err != nil {
		t.Fatalf("query the activity gate: %v", err)
	}
	defer rows.Close()
	got := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[id] = true
	}

	if got[noRow] {
		t.Error("an account with no settings row received lifecycle mail; that gate is opt-in and must stay so")
	}
	if !got[newsOffOnly] {
		t.Error("declining campaigns silenced lifecycle mail; that is the coupling 0152 removed, in the other direction")
	}
	if !got[activityOn] {
		t.Error("an account that enabled notifications did not pass the gate")
	}
}

// The alerts switch silences the email digest and nothing else. A Telegram
// destination is something the account connected itself, and a link in an email
// must not reach past the channel it arrived on.
func TestAlertsGate_SilencesEmailDigestsOnlyAndKeepsTheOtherChannels(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)

	user := insertUser(t, pool, "alerts@example.test")
	search := insertSavedSearch(t, pool, user, "Go remote", `{"q":"go"}`)
	for _, channel := range []string{"email", "telegram"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO subscriptions (user_id, saved_search_id, channel) VALUES ($1, $2, $3)`,
			user, search, channel); err != nil {
			t.Fatalf("insert %s subscription: %v", channel, err)
		}
	}

	before, err := q.ListActiveSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSubscriptions: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("with no settings row both subscriptions must be active, got %d", len(before))
	}

	setEmailGroups(t, pool, user, nil, ptr(false), nil)

	after, err := q.ListActiveSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSubscriptions: %v", err)
	}
	if len(after) != 1 || after[0].Channel != "telegram" {
		t.Fatalf("after declining email alerts, got %d subscriptions %+v; want only the telegram one", len(after), after)
	}

	// And the per-subscription state is preserved, not cleared, so turning the
	// master switch back on restores exactly what was subscribed before.
	setEmailGroups(t, pool, user, nil, ptr(true), nil)
	restored, err := q.ListActiveSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSubscriptions: %v", err)
	}
	if len(restored) != 2 {
		t.Errorf("turning alerts back on restored %d subscriptions, want the original 2", len(restored))
	}
}

// The onboarding sequence reads the same news switch as campaigns, so declining one
// declines both — they are the same kind of mail from the same person.
func TestOnboardingSequence_ReadsTheNewsSwitch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)

	wants := insertUser(t, pool, "onboarding-yes@example.test")
	declined := insertUser(t, pool, "onboarding-no@example.test")
	verifyEmail(t, pool, wants)
	verifyEmail(t, pool, declined)
	setEmailGroups(t, pool, declined, nil, nil, ptr(false))

	rows, err := q.ListWelcomeCandidates(ctx, ListWelcomeCandidatesParams{WindowDays: 14, MaxRows: 100})
	if err != nil {
		t.Fatalf("ListWelcomeCandidates: %v", err)
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}
	if !got[wants] {
		t.Error("a fresh verified account did not get the welcome mail")
	}
	if got[declined] {
		t.Error("an account that declined news still got the onboarding sequence")
	}
}
