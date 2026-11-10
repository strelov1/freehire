//go:build integration

// Integration tests for the paywall letter's SQL against a real Postgres: when a hit is due,
// once per account, the eligibility re-check inside the claim, and the day it shares with the
// CV letters. Run with: go test -tags=integration ./internal/engage/cvlifecycle/
package cvlifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/maildaily"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// plainUser inserts a verified free account with no CV at all.
func plainUser(t *testing.T, pool *pgxpool.Pool, label string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`,
		fmt.Sprintf("%s@example.test", label)).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", label, err)
	}
	return id
}

// plainMailable is mailable for accounts without a CV: one outside and one inside the holdout.
func plainMailable(t *testing.T, pool *pgxpool.Pool, prefix string) (mailed, held int64) {
	t.Helper()
	for i := 0; mailed == 0 || held == 0; i++ {
		if i > 200 {
			t.Fatal("no holdout/non-holdout pair in 200 accounts")
		}
		id := plainUser(t, pool, fmt.Sprintf("%s-%d", prefix, i))
		if isHoldout(t, pool, id) {
			if held == 0 {
				held = id
			}
		} else if mailed == 0 {
			mailed = id
		}
	}
	return mailed, held
}

func seedJob(t *testing.T, pool *pgxpool.Pool, slug, title, company string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('greenhouse', $1, 'https://example.test/j/'||$1, $2, $3, $1) RETURNING id`,
		slug, title, company).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

// hit records a paywall hit made `ago` ago, on the posting when jobID is not zero.
func hit(t *testing.T, pool *pgxpool.Pool, userID int64, feature string, jobID int64, ago time.Duration) {
	t.Helper()
	var job any
	if jobID != 0 {
		job = jobID
	}
	at := time.Now().Add(-ago)
	exec(t, pool, `INSERT INTO paywall_hits (user_id, feature, job_id, day, created_at) VALUES ($1, $2, $3, $4::timestamptz::date, $4)`,
		userID, feature, job, at)
}

func dueIDs(t *testing.T, q *db.Queries) map[int64]db.ListPaywallLetterDueRow {
	t.Helper()
	cfg := DefaultConfig()
	rows, err := q.ListPaywallLetterDue(context.Background(), db.ListPaywallLetterDueParams{
		HoldMinutes: cfg.PaywallAfterMinutes, MaxAgeHours: cfg.PaywallWithinHours, MaxRows: 1000,
	})
	if err != nil {
		t.Fatalf("ListPaywallLetterDue: %v", err)
	}
	due := map[int64]db.ListPaywallLetterDueRow{}
	for _, row := range rows {
		due[row.UserID] = row
	}
	return due
}

func claimPaywall(q *db.Queries, id int64) error {
	cfg := DefaultConfig()
	_, err := q.ClaimPaywallLetterSend(context.Background(), db.ClaimPaywallLetterSendParams{
		UserID: id, HoldMinutes: cfg.PaywallAfterMinutes, MaxAgeHours: cfg.PaywallWithinHours,
	})
	return err
}

func TestPaywall_DueAnHourAfterTheHitAndWhileItIsFresh(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	tooSoon, _ := plainMailable(t, pool, "soon")
	due, _ := plainMailable(t, pool, "due")
	stale, _ := plainMailable(t, pool, "stale")
	hit(t, pool, tooSoon, "tailor", 0, 20*time.Minute)
	hit(t, pool, due, "tailor", 0, 90*time.Minute)
	hit(t, pool, stale, "tailor", 0, 30*time.Hour)

	got := dueIDs(t, q)
	if _, ok := got[due]; !ok {
		t.Error("a hit an hour and a half old is not due")
	}
	if _, ok := got[tooSoon]; ok {
		t.Error("a hit twenty minutes old is already due")
	}
	if _, ok := got[stale]; ok {
		t.Error("a hit older than the window is still due")
	}
	if err := claimPaywall(q, tooSoon); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("claim for a hit that is too fresh = %v, want ErrNoRows", err)
	}
	if err := claimPaywall(q, stale); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("claim for a hit that is stale = %v, want ErrNoRows", err)
	}
}

func TestPaywall_NamesTheEarliestHitsFeatureAndPosting(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	id, _ := plainMailable(t, pool, "names")
	first := seedJob(t, pool, "go-dev-acme", "Go Developer", "Acme")
	second := seedJob(t, pool, "rust-dev-bolt", "Rust Developer", "Bolt")
	hit(t, pool, id, "tailor", first, 3*time.Hour)
	hit(t, pool, id, "match", second, 2*time.Hour)
	hit(t, pool, id, "assistant", 0, 90*time.Minute)

	row, ok := dueIDs(t, q)[id]
	if !ok {
		t.Fatal("the account is not listed")
	}
	if row.Feature != "tailor" || row.JobTitle != "Go Developer" || row.JobCompany != "Acme" || row.JobSlug != "go-dev-acme" {
		t.Errorf("row = %+v, want the earliest hit's feature and posting, one row for the account", row)
	}
}

func TestPaywall_AHitWithoutAPostingHasEmptyJobFields(t *testing.T) {
	pool := testdb.Pool(t)
	id, _ := plainMailable(t, pool, "nojob")
	hit(t, pool, id, "ats_review", 0, 2*time.Hour)

	row := dueIDs(t, db.New(pool))[id]
	if row.Feature != "ats_review" || row.JobTitle != "" || row.JobCompany != "" || row.JobSlug != "" {
		t.Errorf("row = %+v, want the feature alone", row)
	}
}

func TestPaywall_APrivatePostingIsNamedOnlyToItsCreator(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	creator, _ := plainMailable(t, pool, "privowner")
	stranger, _ := plainMailable(t, pool, "privstranger")
	job := seedJob(t, pool, "private-go-dev", "Go Developer", "Secret Co")
	exec(t, pool, `UPDATE jobs SET is_private = true, created_by = $1 WHERE id = $2`, creator, job)
	hit(t, pool, creator, "tailor", job, 2*time.Hour)
	hit(t, pool, stranger, "tailor", job, 2*time.Hour)

	due := dueIDs(t, q)
	if row := due[creator]; row.JobTitle != "Go Developer" || row.JobCompany != "Secret Co" || row.JobSlug != "private-go-dev" {
		t.Errorf("creator row = %+v, want their own private posting named", row)
	}
	if row, ok := due[stranger]; !ok || row.JobTitle != "" || row.JobCompany != "" || row.JobSlug != "" {
		t.Errorf("stranger row = %+v (listed=%v), want the feature alone", row, ok)
	}
}

func TestPaywall_OncePerAccountNotOncePerHitOrFeature(t *testing.T) {
	pool := testdb.Pool(t)
	r, sender := newRunner(t, db.New(pool))
	id, _ := plainMailable(t, pool, "once")
	job := seedJob(t, pool, "once-job", "Go Developer", "Acme")
	hit(t, pool, id, "tailor", job, 3*time.Hour)
	hit(t, pool, id, "match", job, 2*time.Hour)
	hit(t, pool, id, "cover-letter", 0, 2*time.Hour)

	for i := 0; i < 3; i++ {
		if _, err := r.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		// A new UTC day each time, so only the ledger can hold the letter back.
		exec(t, pool, `DELETE FROM mail_day_claims WHERE user_id = $1`, id)
	}
	if got := sentSteps(t, pool, id); fmt.Sprint(got) != "[4]" {
		t.Fatalf("steps = %v, want the paywall letter once", got)
	}
	if len(sender.letters) != 1 || sender.letters[0].Paywall.Feature != "tailor" {
		t.Fatalf("letters = %+v, want one, about the earliest hit", sender.letters)
	}
}

func TestPaywall_NeverToThePaidTheHoldoutTheUnsubscribedOrAnUnverifiedAddress(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	_, held := plainMailable(t, pool, "held")

	setups := map[string]func(id int64){
		"holdout": nil,
		"paid": func(id int64) {
			exec(t, pool, `UPDATE users SET pro_until_stripe = now() + interval '5 days' WHERE id = $1`, id)
		},
		"granted": func(id int64) {
			exec(t, pool, `UPDATE users SET ultra_until_granted = now() + interval '5 days' WHERE id = $1`, id)
		},
		"unsubscribed": func(id int64) {
			exec(t, pool, `INSERT INTO notification_settings (user_id, news_email_enabled) VALUES ($1, false)`, id)
		},
		"unverified": func(id int64) { exec(t, pool, `UPDATE users SET email_verified = false WHERE id = $1`, id) },
	}
	ids := map[string]int64{"holdout": held}
	for name, setup := range setups {
		id := held
		if setup != nil {
			id, _ = plainMailable(t, pool, "no-"+name)
		}
		ids[name] = id
		hit(t, pool, id, "tailor", 0, 2*time.Hour)
	}

	// Everything that can end the letter does so at the claim, which is what gates the send.
	for name, setup := range setups {
		if setup != nil {
			if err := claimPaywall(q, ids[name]); err != nil {
				t.Fatalf("%s: the account was eligible before the change: %v", name, err)
			}
			exec(t, pool, `DELETE FROM cv_lifecycle_sends WHERE user_id = $1`, ids[name])
			setup(ids[name])
		}
		if _, listed := dueIDs(t, q)[ids[name]]; listed {
			t.Errorf("%s: listed for the letter", name)
		}
		if err := claimPaywall(q, ids[name]); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("%s: claim = %v, want ErrNoRows", name, err)
		}
	}
}

func TestPaywall_ClaimIsOncePerAccountEver(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	id, _ := plainMailable(t, pool, "claim-once")
	hit(t, pool, id, "tailor", 0, 2*time.Hour)

	if err := claimPaywall(q, id); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := claimPaywall(q, id); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second claim = %v, want ErrNoRows", err)
	}
	hit(t, pool, id, "match", 0, 100*time.Minute)
	if _, listed := dueIDs(t, q)[id]; listed {
		t.Error("a later hit made the account due again")
	}
}

func TestPaywall_WinsTheDayOverACVLetterThenTheCVLetterFollows(t *testing.T) {
	pool := testdb.Pool(t)
	r, _ := newRunner(t, db.New(pool))
	id, _ := mailable(t, pool, "both", 2*day) // CV letter 1 is due
	hit(t, pool, id, "tailor", 0, 2*time.Hour)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sentSteps(t, pool, id); fmt.Sprint(got) != "[4]" {
		t.Fatalf("steps = %v, want only the paywall letter on the shared day", got)
	}

	exec(t, pool, `UPDATE mail_day_claims SET day = mail_today() - 1 WHERE user_id = $1`, id)
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sentSteps(t, pool, id); fmt.Sprint(got) != "[1 4]" {
		t.Fatalf("steps = %v, want the CV letter the next day", got)
	}
}

func TestPaywall_AnotherSendersDayKeepsItPendingThenItGoesOut(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	r, _ := newRunner(t, q)
	id, _ := plainMailable(t, pool, "busy-paywall")
	hit(t, pool, id, "tailor", 0, 2*time.Hour)
	if _, err := q.ClaimMailDay(context.Background(), db.ClaimMailDayParams{UserID: id, Source: maildaily.SourceBroadcast}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sentSteps(t, pool, id)) != 0 {
		t.Fatal("the paywall letter went out on another sender's day")
	}

	exec(t, pool, `UPDATE mail_day_claims SET day = mail_today() - 1 WHERE user_id = $1`, id)
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sentSteps(t, pool, id); fmt.Sprint(got) != "[4]" {
		t.Fatalf("steps after the day passed = %v, want [4]", got)
	}
}
