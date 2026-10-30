//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/platform/db"
)

func book(mocks *buddy.MockService, guest int64, hostHandle string, at time.Time, topic string) (buddy.Mock, error) {
	return mocks.Book(context.Background(), buddy.BookInput{
		HostHandle: hostHandle, GuestUserID: guest, StartsAt: at, Topic: topic, GuestZone: "Asia/Tokyo",
	})
}

func TestBookingConfirmsTheMockAndNotifies(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, guestHandle := joined(t, pool, svc, "guest@example.test")
	other, _ := joined(t, pool, svc, "other@example.test")
	openAllWeek(t, mocks, host, "UTC")

	at := firstSlot(t, mocks, guest, hostHandle)
	m, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != buddy.MockConfirmed || !m.StartsAt.Equal(at) || m.EndsAt.Sub(m.StartsAt) != buddy.MockDuration ||
		m.HostHandle != hostHandle || m.GuestHandle != guestHandle || m.MeetingURL != hostLink(host) ||
		m.GuestTimezone != "Asia/Tokyo" || m.HostTimezone != "UTC" {
		t.Fatalf("booked = %+v", m)
	}
	if len(n.booked) != 1 || n.booked[0].ID != m.ID {
		t.Fatalf("booked notices = %v", n.booked)
	}
	now := time.Now()
	res, err := mocks.OpenSlots(ctx, other, hostHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Slots {
		if s.Start.Equal(at) {
			t.Fatalf("the booked %v slot is still offered", at)
		}
	}
}

// insertBarrier holds every CreateMock until `parties` callers have reached it, so they
// all write together: each has already passed the service's own re-derivation of the slot,
// and only the database can refuse the losers.
type insertBarrier struct {
	*buddy.QueriesRepository
	arrived sync.WaitGroup
}

func (b *insertBarrier) CreateMock(ctx context.Context, in buddy.NewMock) (buddy.Mock, error) {
	b.arrived.Done()
	b.arrived.Wait()
	return b.QueriesRepository.CreateMock(ctx, in)
}

func TestConcurrentBookingsOfOneSlotConfirmExactlyOne(t *testing.T) {
	pool, svc, _, _ := mockSetup(t)
	repo := &insertBarrier{QueriesRepository: buddy.NewQueriesRepository(db.New(pool))}
	repo.arrived.Add(2)
	mocks := buddy.NewMockService(svc, repo, &recordingNotifier{})
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	g1, _ := joined(t, pool, svc, "g1@example.test")
	g2, _ := joined(t, pool, svc, "g2@example.test")
	openAllWeek(t, mocks, host, "UTC")
	at := firstSlot(t, mocks, g1, hostHandle)

	// Both goroutines park on start and are released together; the barrier then makes the
	// two inserts run at the same moment, so the EXCLUDE, not a read, decides the winner.
	errs := make([]error, 2)
	start := make(chan struct{})
	var done sync.WaitGroup
	for i, g := range []int64{g1, g2} {
		done.Add(1)
		go func() {
			defer done.Done()
			<-start
			_, errs[i] = book(mocks, g, hostHandle, at, buddy.TopicCoding)
		}()
	}
	close(start)
	done.Wait()

	won, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, buddy.ErrSlotUnavailable):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 || refused != 1 {
		t.Fatalf("won %d, refused %d; want one of each", won, refused)
	}
	var confirmed int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM buddy_mocks WHERE status = 'confirmed'`).Scan(&confirmed); err != nil || confirmed != 1 {
		t.Fatalf("confirmed mocks = %d (err %v), want 1", confirmed, err)
	}
}

func TestAGuestAlreadyHostingIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	third, _ := joined(t, pool, svc, "third@example.test")
	openAllWeek(t, mocks, host, "UTC")

	at := firstSlot(t, mocks, guest, hostHandle)
	seedMock(t, pool, guest, third, at) // the guest hosts their own mock at that hour

	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding); !errors.Is(err, buddy.ErrSlotUnavailable) {
		t.Fatalf("err = %v, want ErrSlotUnavailable", err)
	}
}

func TestATopicTheHostDoesNotOfferIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC", buddy.TopicCoding)

	at := firstSlot(t, mocks, guest, hostHandle)
	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicBehavioral); !errors.Is(err, buddy.ErrInvalidTopic) {
		t.Fatalf("err = %v, want ErrInvalidTopic", err)
	}
}

func TestAnInstantThatIsNotASlotIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")

	// Half past a slot's start is inside the host's hours but is not a slot they offer.
	at := firstSlot(t, mocks, guest, hostHandle).Add(30 * time.Minute)
	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding); !errors.Is(err, buddy.ErrSlotUnavailable) {
		t.Fatalf("err = %v, want ErrSlotUnavailable", err)
	}
}

func TestAFourthFutureMockAsGuestIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	other, _ := joined(t, pool, svc, "other@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")

	base := time.Now().UTC().Truncate(time.Hour).Add(5 * 24 * time.Hour)
	for i := range buddy.MaxGuestMocks {
		seedMock(t, pool, other, guest, base.Add(time.Duration(i)*2*time.Hour))
	}
	at := firstSlot(t, mocks, guest, hostHandle)
	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding); !errors.Is(err, buddy.ErrGuestMockLimit) {
		t.Fatalf("err = %v, want ErrGuestMockLimit", err)
	}
}

func TestPausingKeepsBookedMocksConfirmed(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	m, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mocks.SaveSettings(ctx, host, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: hostLink(host), Topics: buddy.MockTopics, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if res, err := mocks.OpenSlots(ctx, guest, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); err != nil || len(res.Slots) != 0 {
		t.Fatalf("paused host offers %d slots (err %v)", len(res.Slots), err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM buddy_mocks WHERE id = $1::uuid`, m.ID.String()).Scan(&status); err != nil || status != buddy.MockConfirmed {
		t.Fatalf("booked mock after pause = %q (err %v), want confirmed", status, err)
	}
}
