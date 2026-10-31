//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

func TestMockSlotsAreListedInTheViewersZone(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	openAllWeek(t, mocks, host, "Europe/Berlin", buddy.TopicCoding, buddy.TopicBehavioral)

	now := time.Now()
	res, err := mocks.OpenSlots(ctx, viewer, hostHandle, now, now.AddDate(0, 0, 40), "Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if res.Zone != "Asia/Tokyo" || len(res.Slots) == 0 {
		t.Fatalf("zone %q, %d slots", res.Zone, len(res.Slots))
	}
	for _, s := range res.Slots {
		if s.Start.Before(now.Add(buddy.MockMinimumNotice)) || s.Start.After(now.Add(buddy.MockHorizon+time.Minute)) {
			t.Fatalf("slot %v outside the notice period and the horizon", s.Start)
		}
		if s.Start.Location().String() != "Asia/Tokyo" {
			t.Fatalf("slot %v not expressed in the viewer's zone", s.Start)
		}
	}
	if len(res.Topics) != 2 || res.Topics[0] != buddy.TopicCoding || res.Topics[1] != buddy.TopicBehavioral {
		t.Fatalf("topics = %v", res.Topics)
	}
}

func TestPausedOrUnsetHostOffersNoSlots(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	_, unsetHandle := joined(t, pool, svc, "unset@example.test")
	paused, pausedHandle := joined(t, pool, svc, "paused@example.test")
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	openAllWeek(t, mocks, paused, "UTC")
	if _, err := mocks.SaveSettings(ctx, paused, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: hostLink(paused), Topics: buddy.MockTopics, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for _, handle := range []string{unsetHandle, pausedHandle} {
		res, err := mocks.OpenSlots(ctx, viewer, handle, now, now.AddDate(0, 0, 7), "UTC")
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Slots) != 0 || len(res.Topics) != 0 || res.Slots == nil {
			t.Fatalf("%s offers %d slots and topics %v; want an empty, non-nil list", handle, len(res.Slots), res.Topics)
		}
	}
}

func TestBeingAGuestHidesTheHostsOwnSlot(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	h, hHandle := joined(t, pool, svc, "h@example.test")
	other, _ := joined(t, pool, svc, "other@example.test")
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	openAllWeek(t, mocks, h, "UTC")

	at := firstSlot(t, mocks, viewer, hHandle)
	seedMock(t, pool, other, h, at) // h is the GUEST of someone else's mock at that hour

	now := time.Now()
	res, err := mocks.OpenSlots(ctx, viewer, hHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Slots {
		if s.Start.Equal(at) {
			t.Fatalf("h's %v slot is offered while h is a guest in another mock", at)
		}
	}
}

func TestSlotListingRefusesStrangersAndBlocks(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	blocker, _ := joined(t, pool, svc, "blocker@example.test")
	blocked, _ := joined(t, pool, svc, "blocked@example.test")
	outsider := seedUser(t, pool, "outsider@example.test")
	openAllWeek(t, mocks, host, "UTC")
	now := time.Now()

	if _, err := mocks.OpenSlots(ctx, outsider, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("non-member: err = %v, want ErrNotMember", err)
	}
	for _, stmt := range [][2]int64{{blocker, host}, {host, blocked}} {
		if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, stmt[0], stmt[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, viewer := range []int64{blocker, blocked} {
		if _, err := mocks.OpenSlots(ctx, viewer, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); !errors.Is(err, buddy.ErrNotFound) {
			t.Fatalf("viewer %d across a block: err = %v, want ErrNotFound", viewer, err)
		}
	}
	if _, err := mocks.OpenSlots(ctx, host, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("own handle: err = %v, want ErrNotFound", err)
	}
}
