//go:build integration

package cvlifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type checkoutVerifierFunc func(context.Context, int64, string) (bool, error)

func (f checkoutVerifierFunc) CheckoutAbandoned(ctx context.Context, id int64, session string) (bool, error) {
	return f(ctx, id, session)
}

func TestCheckoutFeedbackUsesStripeBeforeClaiming(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	id, held := plainMailable(t, pool, "checkout")

	record := func(userID int64, session string, age time.Duration) {
		t.Helper()
		err := q.RecordCheckoutFeedbackSession(ctx, db.RecordCheckoutFeedbackSessionParams{
			UserID: userID, StripeSessionID: session,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-age), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	expired := plainUser(t, pool, "past-checkout-window")
	record(expired, "cs_past_window", 73*time.Hour)
	record(held, "cs_holdout", 25*time.Hour)
	record(id, "cs_old", 73*time.Hour)
	record(id, "cs_latest", 25*time.Hour)
	record(id, "cs_slow_response", 26*time.Hour)

	rows, err := q.ListCheckoutFeedbackDue(ctx, db.ListCheckoutFeedbackDueParams{MaxRows: 10})
	if err != nil || len(rows) != 1 || rows[0].UserID != id || rows[0].StripeSessionID != "cs_latest" {
		t.Fatalf("due checkouts = %+v, %v", rows, err)
	}

	claim := db.ClaimCheckoutFeedbackSendParams{UserID: id, StripeSessionID: "cs_latest", Email: rows[0].Email}
	for _, change := range []struct{ set, reset string }{
		{"UPDATE users SET pro_until_stripe = now() + interval '1 day' WHERE id = $1",
			"UPDATE users SET pro_until_stripe = NULL WHERE id = $1"},
		{"INSERT INTO notification_settings (user_id, news_email_enabled) VALUES ($1, false)",
			"DELETE FROM notification_settings WHERE user_id = $1"},
		{"UPDATE users SET banned_at = now(), ban_reason = 'test' WHERE id = $1",
			"UPDATE users SET banned_at = NULL, ban_reason = NULL WHERE id = $1"},
	} {
		exec(t, pool, change.set, id)
		if _, err := q.ClaimCheckoutFeedbackSend(ctx, claim); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("claim after %q: %v", change.set, err)
		}
		exec(t, pool, change.reset, id)
	}
	changedEmail := claim
	changedEmail.Email = "old@example.test"
	if _, err := q.ClaimCheckoutFeedbackSend(ctx, changedEmail); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("claim with stale email: %v", err)
	}

	record(id, "cs_next", 2*time.Hour)
	if _, err := q.ClaimCheckoutFeedbackSend(ctx, claim); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("claim after a new checkout: %v", err)
	}

	exec(t, pool, "UPDATE checkout_feedback_sessions SET stripe_session_id = $2, created_at = now() - interval '25 hours' WHERE user_id = $1", id, claim.StripeSessionID)

	converted := true
	verifier := checkoutVerifierFunc(func(_ context.Context, userID int64, session string) (bool, error) {
		if userID != id || session != "cs_latest" {
			t.Errorf("checked wrong checkout: %d %q", userID, session)
		}
		return !converted, nil
	})
	sender := &fakeSender{store: newFakeStore()}
	r, err := New(q, nil, sender, verifier, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	run := func(wantSent int) {
		t.Helper()
		stats, err := r.RunCheckoutFeedback(ctx)
		if err != nil || stats.Sent[StepCheckoutFeedback] != wantSent || stats.Errors != 0 {
			t.Fatalf("checkout run: %+v, %v", stats, err)
		}
	}
	run(0)
	var stillRecorded bool
	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM checkout_feedback_sessions WHERE user_id = $1)", expired).Scan(&stillRecorded)
	if err != nil || stillRecorded {
		t.Fatalf("checkout past the send window was not pruned: %t, %v", stillRecorded, err)
	}
	converted = false
	if stats, err := r.Run(ctx); err != nil || stats.Sent[StepCheckoutFeedback] != 0 {
		t.Fatalf("CV lifecycle pass touched checkout feedback: %+v, %v", stats, err)
	}
	dry, err := r.DryRunCheckoutFeedback(ctx)
	if err != nil || dry.Due[StepCheckoutFeedback] != 1 || len(sender.sent) != 0 {
		t.Fatalf("checkout dry run: %+v, %v", dry, err)
	}
	run(1)
	run(0)
	if len(sender.sent) != 1 || sender.sent[0] != fmt.Sprintf("%d:%d:%s", StepCheckoutFeedback, id, claim.Email) {
		t.Fatalf("sent to %v, want only %d", sender.sent, id)
	}
}
