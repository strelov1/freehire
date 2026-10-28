//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// greetedDaysAgo makes an account look as if it signed up and was welcomed the given
// number of days ago, which is the clock every later onboarding step is paced from.
func greetedDaysAgo(t *testing.T, pool *pgxpool.Pool, userID int64, days int) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`UPDATE users SET created_at = now() - make_interval(days => $2) WHERE id = $1`,
		userID, days); err != nil {
		t.Fatalf("backdate signup: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO onboarding_emails (user_id, step, sent_at)
		 VALUES ($1, 'welcome', now() - make_interval(days => $2))`,
		userID, days); err != nil {
		t.Fatalf("record welcome: %v", err)
	}
}

// The buddies letter skips anyone who already knows the feature: a member, a member
// who left, and anyone the one-off 'interview-buddies' broadcast already reached. It
// also reaches past the shared 14-day window — the letter is due on day 14 — while
// still stopping at its own.
func TestListBuddiesCandidates_SkipsMembersAndBroadcastRecipients(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)

	eligible := insertUser(t, pool, "buddies-eligible@example.test")
	member := insertUser(t, pool, "buddies-member@example.test")
	left := insertUser(t, pool, "buddies-left@example.test")
	broadcast := insertUser(t, pool, "buddies-broadcast@example.test")
	otherCampaign := insertUser(t, pool, "buddies-other-campaign@example.test")
	tooOld := insertUser(t, pool, "buddies-too-old@example.test")
	tooSoon := insertUser(t, pool, "buddies-too-soon@example.test")
	for _, id := range []int64{eligible, member, left, broadcast, otherCampaign, tooOld, tooSoon} {
		verifyEmail(t, pool, id)
	}
	for _, id := range []int64{eligible, member, left, broadcast, otherCampaign} {
		greetedDaysAgo(t, pool, id, 18)
	}
	greetedDaysAgo(t, pool, tooOld, 25)
	greetedDaysAgo(t, pool, tooSoon, 10)

	if _, err := q.JoinBuddyMember(ctx, JoinBuddyMemberParams{UserID: member, Handle: "calm-otter-11"}); err != nil {
		t.Fatalf("join member: %v", err)
	}
	if _, err := q.JoinBuddyMember(ctx, JoinBuddyMemberParams{UserID: left, Handle: "quiet-heron-22"}); err != nil {
		t.Fatalf("join leaver: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE buddy_members SET left_at = now() WHERE user_id = $1`, left); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO broadcast_emails (user_id, campaign) VALUES ($1, 'interview-buddies'), ($2, 'discord')`,
		broadcast, otherCampaign); err != nil {
		t.Fatalf("record broadcasts: %v", err)
	}

	rows, err := q.ListBuddiesCandidates(ctx, ListBuddiesCandidatesParams{WindowDays: 21, AfterDays: 14, MaxRows: 100})
	if err != nil {
		t.Fatalf("ListBuddiesCandidates: %v", err)
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}

	if !got[eligible] {
		t.Error("an 18-day-old account greeted 18 days ago did not get the buddies letter")
	}
	if !got[otherCampaign] {
		t.Error("a different broadcast campaign suppressed the buddies letter")
	}
	if got[member] {
		t.Error("an active interview-buddies member got the letter introducing the feature")
	}
	if got[left] {
		t.Error("an account that joined and left interview buddies got the letter introducing it")
	}
	if got[broadcast] {
		t.Error("an account the interview-buddies broadcast reached got the same pitch again")
	}
	if got[tooOld] {
		t.Error("an account outside the buddies window got the letter")
	}
	if got[tooSoon] {
		t.Error("an account greeted 10 days ago got the day-14 letter early")
	}

	// The ledger closes the step, and the step's vocabulary must admit it (0210).
	if err := q.RecordOnboardingEmail(ctx, RecordOnboardingEmailParams{UserID: eligible, Step: "buddies"}); err != nil {
		t.Fatalf("record buddies: %v", err)
	}
	again, err := q.ListBuddiesCandidates(ctx, ListBuddiesCandidatesParams{WindowDays: 21, AfterDays: 14, MaxRows: 100})
	if err != nil {
		t.Fatalf("ListBuddiesCandidates again: %v", err)
	}
	for _, r := range again {
		if r.ID == eligible {
			t.Error("an account already sent the buddies letter was offered it again")
		}
	}
}
