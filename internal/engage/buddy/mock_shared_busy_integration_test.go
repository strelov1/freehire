//go:build integration

package buddy_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A mock booked in buddy blocks the same time for a booking link, for host and guest alike,
// and the host's hours (written through buddy) are what the booking link reads.
func TestAMockBlocksTheTimeInTheBookingLinkToo(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "shared-host@example.test")
	guest, _ := joined(t, pool, svc, "shared-guest@example.test")
	openAllWeek(t, mocks, host, "UTC")

	at := firstSlot(t, mocks, guest, hostHandle)
	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding); err != nil {
		t.Fatal(err)
	}

	links := bookinglink.NewQueriesRepository(db.New(pool), pool)
	taken := slots.Interval{Start: at, End: at.Add(buddy.MockDuration)}
	for name, user := range map[string]int64{"host": host, "guest": guest} {
		busy, err := links.Busy(context.Background(), user, at.Add(-time.Hour), at.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(busy, func(iv slots.Interval) bool { return iv.Start.Equal(taken.Start) && iv.End.Equal(taken.End) }) {
			t.Errorf("%s: booking-link busy %v lacks the mock %v", name, busy, taken)
		}
	}
	sc, err := links.Schedule(context.Background(), host)
	if err != nil || len(sc.Rules) != 7 || sc.Zone != "UTC" {
		t.Fatalf("host schedule = %+v, %v; want 7 rules in UTC", sc, err)
	}
}
