//go:build integration

package bookinglink_test

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestBookingsQueueAnInvitationOnlyAfterCalendarOptIn(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	var userID, appID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('candidate@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO applications (user_id, company_slug, role_title, applied_at)
		VALUES ($1, 'acme', 'Engineer', now()) RETURNING id`, userID).Scan(&appID); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertCalendarGrant(ctx, db.UpsertCalendarGrantParams{
		UserID: userID, RefreshTokenEnc: "test-encrypted",
		Scopes: []string{gmailsync.CalendarScope, gmailsync.CalendarEventsScope},
	}); err != nil {
		t.Fatal(err)
	}
	svc := bookinglink.New(bookinglink.NewQueriesRepository(q, pool), paid{}, []byte("key"), nil)
	token, err := svc.Enable(ctx, userID, appID, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	store := bookinglink.NewInvitationQueries(q, 0, "")
	in := bookinglink.BookInput{
		Token: token, StartsAt: nextWeekday10(t), Name: "Jane", Email: "jane@example.test",
	}
	if _, err := svc.Book(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Claim(ctx); err != nil || found {
		t.Fatalf("claim a booking before opt-in: found = %v, err = %v", found, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET interview_calendar_invites_enabled = true WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	in.StartsAt = in.StartsAt.Add(30 * time.Minute)
	if _, err := svc.Book(ctx, in); err != nil {
		t.Fatal(err)
	}

	outsidePilot := bookinglink.NewInvitationQueries(q, userID, "other@example.test")
	if _, found, err := outsidePilot.Claim(ctx); err != nil || found {
		t.Fatalf("pilot claimed a booking for another recruiter: found = %v, err = %v", found, err)
	}
	store = bookinglink.NewInvitationQueries(q, userID, "JANE@example.test")
	invite, found, err := store.Claim(ctx)
	if err != nil || !found || invite.Event.Attendee != in.Email || !invite.Event.StartsAt.Equal(in.StartsAt) {
		t.Fatalf("invitation = %+v, found = %v, err = %v", invite.Event, found, err)
	}
	if _, found, err := store.Claim(ctx); err != nil || found {
		t.Fatalf("second claimant took leased booking: found = %v, err = %v", found, err)
	}
	const meet = "https://meet.google.com/abc-defg-hij"
	if err := store.Record(ctx, invite, meet); err != nil {
		t.Fatal(err)
	}
	var gotID, gotMeet string
	if err := pool.QueryRow(ctx, `SELECT provider_event_id, join_url FROM application_interviews
		WHERE application_id = $1 AND starts_at = $2`, appID, in.StartsAt).Scan(&gotID, &gotMeet); err != nil {
		t.Fatal(err)
	}
	if gotID != invite.Event.ID || gotMeet != meet {
		t.Fatalf("interview provider id = %q, Meet URL = %q", gotID, gotMeet)
	}
	if _, found, err := store.Claim(ctx); err != nil || found {
		t.Fatalf("claim after confirmed delivery: found = %v, err = %v", found, err)
	}
}
