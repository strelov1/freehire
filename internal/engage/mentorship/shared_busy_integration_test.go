//go:build integration

package mentorship

import (
	"context"
	"slices"
	"testing"
	"time"

	aiplan "github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type paidAll struct{}

func (paidAll) RequirePaid(context.Context, int64) (aiplan.Tier, error) { return "", nil }

// weekday10UTC is 10:00 UTC on the nth weekday at least two days out.
func weekday10UTC(n int) time.Time {
	d := time.Now().UTC().AddDate(0, 0, 2)
	for ; n > 0; n-- {
		d = d.AddDate(0, 0, 1)
	}
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 10, 0, 0, 0, time.UTC)
}

func offers(res slots.SlotResult, at time.Time) bool {
	return slices.ContainsFunc(res.Slots, func(iv slots.Interval) bool { return iv.Start.Equal(at) })
}

// A confirmed mentor session removes its time from the same user's booking link, and a
// confirmed booking-link meeting removes its time from their mentor slots.
func TestMentorSessionsAndBookingLinkMeetingsBlockEachOther(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool), pool)
	svc := New(repo, Config{})
	seedBookableMentor(t, pool, svc, repo, "shared-mentor")
	mentor, ok, err := repo.PublishedProfileBySlug(ctx, "shared-mentor")
	if err != nil || !ok {
		t.Fatalf("mentor: %v %v", ok, err)
	}
	var app int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO applications (user_id, company_slug, role_title, applied_at) VALUES ($1, 'acme', 'Engineer', now()) RETURNING id`,
		mentor.UserID).Scan(&app); err != nil {
		t.Fatal(err)
	}
	links := bookinglink.New(bookinglink.NewQueriesRepository(db.New(pool), pool), paidAll{}, []byte("k"), nil)
	tok, err := links.Enable(ctx, mentor.UserID, app, "UTC")
	if err != nil {
		t.Fatal(err)
	}

	mentorAt, linkAt := weekday10UTC(0), weekday10UTC(1)
	if !linkAt.After(mentorAt) {
		linkAt = weekday10UTC(3)
	}
	before, err := links.Slots(ctx, tok, "UTC")
	if err != nil || !offers(before, mentorAt) || !offers(before, linkAt) {
		t.Fatalf("link must offer both times before any meeting: %v", err)
	}

	seeker := seedUser(t, pool, "shared-seeker@example.test")
	if _, err := svc.Book(ctx, BookingInput{MentorSlug: "shared-mentor", SeekerUserID: seeker, StartsAt: mentorAt, SeekerTimezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	after, err := links.Slots(ctx, tok, "UTC")
	if err != nil || offers(after, mentorAt) {
		t.Fatalf("a mentor session must remove its time from the booking link (err %v)", err)
	}

	if _, err := links.Book(ctx, bookinglink.BookInput{Token: tok, StartsAt: linkAt, Name: "R", Email: "r@acme.com"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.slotsFor(ctx, mentor, time.Now(), time.Now().AddDate(0, 0, 14), "UTC")
	if err != nil || offers(res, linkAt) {
		t.Fatalf("a booking-link meeting must remove its time from mentor slots (err %v)", err)
	}
	if len(res.Slots) == 0 {
		t.Fatal("the mentor must keep their other slots")
	}
}
