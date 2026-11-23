package bookinglink

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	aiplan "github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/slots"
)

type fakeRepo struct {
	link    *Link
	owner   int64
	writes  int
	busy    []slots.Interval
	booked  []NewBooking
	bookErr error
}

func (f *fakeRepo) Upsert(_ context.Context, userID, appID int64, seed, hash []byte, zone string) (Link, bool, error) {
	f.writes++
	if userID != f.owner {
		return Link{}, false, nil
	}
	if f.link == nil {
		f.link = &Link{ID: 1, UserID: userID, ApplicationID: appID, TokenSeed: seed, TokenHash: hash}
	}
	f.link.Zone = zone
	return *f.link, true, nil
}

func (f *fakeRepo) ForApplication(context.Context, int64, int64) (Link, bool, error) {
	if f.link == nil {
		return Link{}, false, nil
	}
	return *f.link, true, nil
}

func (f *fakeRepo) ByHash(_ context.Context, h []byte) (Link, bool, error) {
	if f.link == nil || !bytes.Equal(f.link.TokenHash, h) {
		return Link{}, false, nil
	}
	return *f.link, true, nil
}

func (f *fakeRepo) SetHash(_ context.Context, _ int64, h []byte) error {
	f.link.TokenHash = h
	return nil
}

func (f *fakeRepo) Busy(context.Context, int64, time.Time, time.Time) ([]slots.Interval, error) {
	return f.busy, nil
}

func (f *fakeRepo) Book(_ context.Context, b NewBooking) error {
	if f.bookErr != nil {
		return f.bookErr
	}
	f.booked = append(f.booked, b)
	return nil
}

type fakePlans map[int64]bool

func (p fakePlans) RequirePaid(_ context.Context, userID int64) (aiplan.Tier, error) {
	if !p[userID] {
		return "", aiplan.ErrPaymentRequired
	}
	return "", nil
}

const owner = 7

// monday is 2026-11-16, a Monday; now sits early on it so Wednesday is well past the notice.
var now = time.Date(2026, 11, 16, 6, 0, 0, 0, time.UTC)

func newTestService(repo *fakeRepo, paid bool) *Service {
	s := New(repo, fakePlans{owner: paid}, []byte("test-key"), nil)
	s.now = func() time.Time { return now }
	return s
}

func enabled(t *testing.T) (*Service, *fakeRepo, string) {
	t.Helper()
	repo := &fakeRepo{owner: owner}
	s := newTestService(repo, true)
	tok, err := s.Enable(context.Background(), owner, 42, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return s, repo, tok
}

func TestEnableReturnsTheSameTokenAgainAndStoresOnlyItsHash(t *testing.T) {
	s, repo, tok := enabled(t)
	if len(tok) < 22 { // 128 bits in unpadded base64url
		t.Fatalf("token %q is too short", tok)
	}
	again, err := s.Enable(context.Background(), owner, 42, "Europe/Berlin")
	if err != nil || again != tok {
		t.Fatalf("second Enable = %q, %v; want %q", again, err, tok)
	}
	if read, ok, err := s.TokenFor(context.Background(), owner, 42); err != nil || !ok || read != tok {
		t.Fatalf("TokenFor = %q, %v, %v; want %q", read, ok, err, tok)
	}
	if bytes.Contains(repo.link.TokenSeed, []byte(tok)) || !bytes.Equal(repo.link.TokenHash, hashOf(tok)) {
		t.Fatal("the stored row must hold the token's hash, not the token")
	}
}

func TestEnableRefusesAFreeAccountWithoutWriting(t *testing.T) {
	repo := &fakeRepo{owner: owner}
	_, err := newTestService(repo, false).Enable(context.Background(), owner, 42, "Europe/Berlin")
	if !errors.Is(err, aiplan.ErrPaymentRequired) || repo.writes != 0 {
		t.Fatalf("err = %v, writes = %d; want ErrPaymentRequired and no write", err, repo.writes)
	}
}

func TestEnableOnSomeoneElsesApplicationIsNotFound(t *testing.T) {
	repo := &fakeRepo{owner: 99}
	_, err := newTestService(repo, true).Enable(context.Background(), owner, 42, "Europe/Berlin")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestEnableRefusesANonPlaceZone(t *testing.T) {
	for _, z := range []string{"", "Local", "EST", "Mars/Olympus"} {
		_, err := newTestService(&fakeRepo{owner: owner}, true).Enable(context.Background(), owner, 42, z)
		if !errors.Is(err, ErrInvalidZone) {
			t.Errorf("zone %q: err = %v, want ErrInvalidZone", z, err)
		}
	}
}

func TestSlotsAreWeekdayWorkingHoursInTheOwnersZone(t *testing.T) {
	s, _, tok := enabled(t)
	res, err := s.Slots(context.Background(), tok, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if res.Zone != "Europe/Berlin" || len(res.Slots) == 0 {
		t.Fatalf("zone %q, %d slots", res.Zone, len(res.Slots))
	}
	perDay := map[string]int{}
	for _, iv := range res.Slots {
		if iv.End.Sub(iv.Start) != SlotDuration {
			t.Fatalf("slot %v is not 30 minutes", iv)
		}
		if wd := iv.Start.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("weekend slot %v", iv.Start)
		}
		if h := iv.Start.Hour(); h < 10 || iv.End.Hour()*60+iv.End.Minute() > 18*60 {
			t.Fatalf("slot %v outside 10:00-18:00 Berlin", iv)
		}
		perDay[iv.Start.Format(time.DateOnly)]++
	}
	if perDay["2026-11-18"] != 16 {
		t.Fatalf("Wednesday has %d slots, want 16", perDay["2026-11-18"])
	}
}

func TestAnUnknownViewerZoneFallsBackToUTC(t *testing.T) {
	s, _, tok := enabled(t)
	res, err := s.Slots(context.Background(), tok, "Nowhere/Land")
	if err != nil || res.Zone != "UTC" {
		t.Fatalf("zone %q, err %v; want UTC", res.Zone, err)
	}
}

func TestABadTokenAndALapsedOwnerAnswerAlike(t *testing.T) {
	s, repo, tok := enabled(t)
	if _, err := s.Slots(context.Background(), "nope", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad token: %v", err)
	}
	lapsed := newTestService(repo, false)
	if _, err := lapsed.Slots(context.Background(), tok, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lapsed owner slots: %v", err)
	}
	wed10 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC) // 10:00 Berlin
	if _, err := lapsed.Book(context.Background(), BookInput{Token: tok, StartsAt: wed10, Name: "R", Email: "r@acme.com"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lapsed owner book: %v", err)
	}
}

func TestBookTakesAnOfferedSlotAndRefusesOffGridOrBusy(t *testing.T) {
	s, repo, tok := enabled(t)
	wed10 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC) // 10:00 Berlin
	b, err := s.Book(context.Background(), BookInput{Token: tok, StartsAt: wed10, Name: " Jane ", Email: "jane@acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	if !b.EndsAt.Equal(wed10.Add(SlotDuration)) || b.RecruiterName != "Jane" || len(repo.booked) != 1 {
		t.Fatalf("booking %+v", b)
	}

	offGrid := wed10.Add(7 * time.Minute)
	if _, err := s.Book(context.Background(), BookInput{Token: tok, StartsAt: offGrid, Name: "J", Email: "j@acme.com"}); !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("10:07: %v", err)
	}
	repo.busy = []slots.Interval{{Start: wed10, End: wed10.Add(SlotDuration)}}
	if _, err := s.Book(context.Background(), BookInput{Token: tok, StartsAt: wed10, Name: "J", Email: "j@acme.com"}); !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("busy slot: %v", err)
	}
}

func TestBookRefusesBadInputBeforeReadingTheLink(t *testing.T) {
	s, _, tok := enabled(t)
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	for _, in := range []BookInput{
		{Token: tok, StartsAt: at, Name: "  ", Email: "j@acme.com"},
		{Token: tok, StartsAt: at, Name: "J", Email: "not-an-email"},
		{Token: tok, StartsAt: at, Name: "J", Email: "Jane <j@acme.com>"},
		{Token: tok, StartsAt: at, Name: "J\r\nBcc: x", Email: "j@acme.com"},
		{Token: tok, StartsAt: at, Name: "J\x00", Email: "j@acme.com"},
		{Token: tok, StartsAt: at, Name: "J", Email: "j\x00@acme.com"},
	} {
		if _, err := s.Book(context.Background(), in); !errors.Is(err, ErrInvalidBooking) {
			t.Errorf("%+v: err = %v, want ErrInvalidBooking", in, err)
		}
	}
}

func TestTokenForHealsAHashAfterTheKeyChanged(t *testing.T) {
	_, repo, old := enabled(t)
	s := New(repo, fakePlans{owner: true}, []byte("rotated-key"), nil)
	tok, ok, err := s.TokenFor(context.Background(), owner, 42)
	if err != nil || !ok || tok == old {
		t.Fatalf("TokenFor = %q, %v, %v", tok, ok, err)
	}
	if _, _, err := s.live(context.Background(), tok); err != nil {
		t.Fatalf("the healed token must resolve: %v", err)
	}
}
