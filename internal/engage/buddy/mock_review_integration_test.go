//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

func TestTheGuestSeesTheHostsCurrentLink(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	if _, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding); err != nil {
		t.Fatal(err)
	}
	const fixed = "https://meet.example.test/fixed-room"
	if _, err := mocks.SaveSettings(ctx, host, buddy.MockSettings{Timezone: "UTC", MeetingURL: fixed, Topics: buddy.MockTopics}); err != nil {
		t.Fatal(err)
	}

	list, err := mocks.List(ctx, guest)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Upcoming) != 1 {
		t.Fatalf("upcoming = %+v", list.Upcoming)
	}
	e := list.Upcoming[0]
	if e.MeetingURL != fixed || e.Role != buddy.RoleGuest || e.Counterpart != hostHandle || !e.CanCancel {
		t.Fatalf("entry = %+v", e)
	}
}

func TestAReviewBeforeTheEndIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-30*time.Minute)) // under way

	if err := mocks.Review(context.Background(), guest, id, 5, ""); !errors.Is(err, buddy.ErrInvalidReview) {
		t.Fatalf("err = %v, want ErrInvalidReview", err)
	}
}

func TestReviewsStayHiddenUntilBothHaveReviewed(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, guestHandle := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-2*time.Hour)) // ended an hour ago

	if err := mocks.Review(ctx, guest, id, 4, "Clear on trade-offs"); err != nil {
		t.Fatal(err)
	}
	hostList, err := mocks.List(ctx, host)
	if err != nil {
		t.Fatal(err)
	}
	if e := hostList.Past[0]; e.TheirReview != nil || !e.CanReview {
		t.Fatalf("host before reviewing: %+v", e)
	}
	if sum, err := mocks.MemberSummary(ctx, guest, hostHandle); err != nil || sum.RatingCount != 0 || sum.Completed != 1 {
		t.Fatalf("host page before both reviewed = %+v, %v", sum, err)
	}

	if err := mocks.Review(ctx, host, id, 5, "Great questions"); err != nil {
		t.Fatal(err)
	}
	if hostList, err = mocks.List(ctx, host); err != nil {
		t.Fatal(err)
	}
	if r := hostList.Past[0].TheirReview; r == nil || r.Rating != 4 || r.Comment != "Clear on trade-offs" {
		t.Fatalf("host sees %+v", r)
	}
	guestList, err := mocks.List(ctx, guest)
	if err != nil {
		t.Fatal(err)
	}
	if r := guestList.Past[0].TheirReview; r == nil || r.Rating != 5 {
		t.Fatalf("guest sees %+v", r)
	}
	hostPage, err := mocks.MemberSummary(ctx, guest, hostHandle)
	if err != nil || hostPage.RatingCount != 1 || hostPage.RatingAvg != 4 || len(hostPage.Recent) != 1 ||
		hostPage.Recent[0].Comment != "Clear on trade-offs" || hostPage.Recent[0].Topic != buddy.TopicCoding {
		t.Fatalf("host page = %+v, %v", hostPage, err)
	}
	if guestPage, err := mocks.MemberSummary(ctx, host, guestHandle); err != nil || guestPage.RatingCount != 1 || guestPage.RatingAvg != 5 {
		t.Fatalf("guest page = %+v, %v", guestPage, err)
	}
	if err := mocks.Review(ctx, host, id, 3, ""); !errors.Is(err, buddy.ErrAlreadyReviewed) {
		t.Fatalf("second review: err = %v, want ErrAlreadyReviewed", err)
	}
}

func TestAOneSidedReviewIsRevealedWhenTheWindowCloses(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-8*24*time.Hour))
	// Written directly: the guest reviewed inside the window, which has closed for anyone
	// writing now.
	if _, err := pool.Exec(ctx, `
		INSERT INTO buddy_mock_reviews (mock_id, author_user_id, subject_user_id, rating, comment)
		VALUES ($1::uuid, $2, $3, 3, 'Ran out of time')`, id.String(), guest, host); err != nil {
		t.Fatal(err)
	}

	hostList, err := mocks.List(ctx, host)
	if err != nil {
		t.Fatal(err)
	}
	if e := hostList.Past[0]; e.TheirReview == nil || e.TheirReview.Rating != 3 || e.CanReview {
		t.Fatalf("host after the window: %+v", e)
	}
	if sum, err := mocks.MemberSummary(ctx, guest, hostHandle); err != nil || sum.RatingCount != 1 {
		t.Fatalf("host page = %+v, %v", sum, err)
	}
	if err := mocks.Review(ctx, host, id, 5, ""); !errors.Is(err, buddy.ErrInvalidReview) {
		t.Fatalf("review after the window: err = %v, want ErrInvalidReview", err)
	}
}
