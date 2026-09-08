package talentnetwork

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// memberRow builds one stored row. The CV is written as JSON on purpose: the snapshot's
// job is to turn stored bytes into cards, and a test that handed it a Structured would
// skip the half most likely to break.
func memberRow(handle, timezone, city, cv string, fresh time.Time) db.ListTalentNetworkMembersRow {
	return db.ListTalentNetworkMembersRow{
		TalentHandle:               pgtype.Text{String: handle, Valid: true},
		Timezone:                   pgtype.Text{String: timezone, Valid: timezone != ""},
		Cities:                     []string{city},
		ResumeStructured:           []byte(cv),
		ResumeStructuredUploadedAt: pgtype.Timestamptz{Time: fresh, Valid: true},
		Specializations:            []string{},
	}
}

const (
	backendCV = `{"total_years":8,"skills":["Go","PostgreSQL"],
	  "experience":[{"title":"Senior Backend Engineer","company":"Acme","current":true,"stack":["Go"]}]}`
	frontendCV = `{"total_years":3,"skills":["React","TypeScript"],
	  "experience":[{"title":"Frontend Developer","company":"Acme","current":true,"stack":["React"]}]}`
	dataCV = `{"total_years":12,"skills":["Python","Airflow"],
	  "experience":[{"title":"Lead Data Engineer","company":"Acme","current":true,"stack":["Python"]}]}`
)

type fakeCatalogueStore struct {
	mu       sync.Mutex
	rows     []db.ListTalentNetworkMembersRow
	one      db.GetTalentNetworkMemberByHandleRow
	oneErr   error
	listErr  error
	listCall int
	oneCall  int
}

func (f *fakeCatalogueStore) ListTalentNetworkMembers(context.Context) ([]db.ListTalentNetworkMembersRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCall++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.rows, nil
}

func (f *fakeCatalogueStore) GetTalentNetworkMemberByHandle(_ context.Context, handle string) (db.GetTalentNetworkMemberByHandleRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.oneCall++
	if f.oneErr != nil {
		return db.GetTalentNetworkMemberByHandleRow{}, f.oneErr
	}
	if f.one.TalentHandle.String != handle {
		return db.GetTalentNetworkMemberByHandleRow{}, pgx.ErrNoRows
	}
	return f.one, nil
}

func (f *fakeCatalogueStore) calls() (list, one int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCall, f.oneCall
}

// threeMembers is one of each discipline, deliberately at DIFFERENT freshness so the
// order is unambiguous, and one of them without a timezone.
func threeMembers(base time.Time) []db.ListTalentNetworkMembersRow {
	return []db.ListTalentNetworkMembersRow{
		memberRow("backend-aaaa", "Europe/Berlin", "berlin", backendCV, base),
		memberRow("frontend-bbbb", "America/New_York", "new-york", frontendCV, base.Add(-time.Hour)),
		memberRow("data-engineering-cccc", "", "lisbon", dataCV, base.Add(-2*time.Hour)),
	}
}

func newTestCatalogue(t *testing.T, store Store, now func() time.Time) *Catalogue {
	t.Helper()
	return NewCatalogue(store, time.Minute, now)
}

func handles(page Page) []string {
	out := make([]string, 0, len(page.Members))
	for _, m := range page.Members {
		out = append(out, m.Handle)
	}
	return out
}

func TestList_ProjectsEveryMember(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	page, err := c.List(context.Background(), Query{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("total = %d, want 3", page.Total)
	}
	if got := handles(page); got[0] != "backend-aaaa" {
		t.Errorf("order = %v, want freshest first", got)
	}
	if page.Members[0].Card.Category != "backend" {
		t.Errorf("card category = %q, want backend", page.Members[0].Card.Category)
	}
	if page.Members[0].TimezoneRegion != "Europe" {
		t.Errorf("timezone region = %q, want Europe", page.Members[0].TimezoneRegion)
	}
}

// A row whose stored CV cannot be parsed is still a member — they joined. It renders as
// a card with nothing on it rather than vanishing, because vanishing is indistinguishable
// from having left.
func TestList_KeepsAMemberWithAnUnreadableCV(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: []db.ListTalentNetworkMembersRow{
		memberRow("backend-aaaa", "Europe/Berlin", "berlin", `{"broken`, base),
	}}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	page, err := c.List(context.Background(), Query{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("total = %d, want the member kept", page.Total)
	}
}

func TestList_FilterValuesWithinOneFilterAreOr(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	page, err := c.List(context.Background(), Query{Categories: []string{"backend", "frontend"}, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("total = %d for two categories, want 2 — values within one filter must be OR", page.Total)
	}
}

func TestList_DifferentFiltersAreAnd(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	page, err := c.List(context.Background(), Query{
		Categories:  []string{"backend", "frontend"},
		Seniorities: []string{"senior"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := handles(page); len(got) != 1 || got[0] != "backend-aaaa" {
		t.Errorf("handles = %v, want only the senior backend one — filters must narrow together", got)
	}
}

func TestList_AnEmptyFilterEqualsAnAbsentOne(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	absent, err := c.List(context.Background(), Query{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	empty, err := c.List(context.Background(), Query{
		Categories: []string{}, Skills: []string{}, Cities: []string{}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if absent.Total != empty.Total {
		t.Errorf("empty filters returned %d, absent returned %d", empty.Total, absent.Total)
	}
}

// A member with no timezone is EXCLUDED by a timezone filter rather than passed through.
// Silently keeping them would make the filter mean something other than what it says.
func TestList_TimezoneFilterExcludesMembersWithoutOne(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	page, err := c.List(context.Background(), Query{TimezoneRegions: []string{"Europe", "America"}, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, h := range handles(page) {
		if h == "data-engineering-cccc" {
			t.Error("a member with no timezone passed a timezone filter")
		}
	}
	if page.Total != 2 {
		t.Errorf("total = %d, want 2", page.Total)
	}
}

func TestList_MinYears(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	page, err := c.List(context.Background(), Query{MinYears: 8, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("total = %d for min 8 years, want 2 (8 and 12)", page.Total)
	}
}

// Walking every page must return each member exactly once. A test that only checks the
// first page cannot see the bug this guards: an unstable order silently drops some rows
// and repeats others across LIMIT/OFFSET.
func TestList_PagingCoversEveryMemberExactlyOnce(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// All three share ONE timestamp, so only the handle tie-break makes the order total.
	rows := []db.ListTalentNetworkMembersRow{
		memberRow("backend-aaaa", "Europe/Berlin", "berlin", backendCV, base),
		memberRow("frontend-bbbb", "Europe/Berlin", "berlin", frontendCV, base),
		memberRow("data-engineering-cccc", "Europe/Berlin", "berlin", dataCV, base),
	}
	store := &fakeCatalogueStore{rows: rows}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	seen := map[string]int{}
	for offset := 0; offset < 3; offset++ {
		page, err := c.List(context.Background(), Query{Limit: 1, Offset: offset})
		if err != nil {
			t.Fatalf("List offset %d: %v", offset, err)
		}
		if len(page.Members) != 1 {
			t.Fatalf("page at offset %d has %d members, want 1", offset, len(page.Members))
		}
		seen[page.Members[0].Handle]++
	}
	if len(seen) != 3 {
		t.Errorf("walking every page saw %d distinct members, want 3: %v", len(seen), seen)
	}
	for h, n := range seen {
		if n != 1 {
			t.Errorf("member %s appeared %d times", h, n)
		}
	}
}

func TestList_OffsetPastTheEndIsEmptyNotAnError(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	page, err := c.List(context.Background(), Query{Limit: 10, Offset: 99})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Members) != 0 || page.Total != 3 {
		t.Errorf("members = %d, total = %d; want 0 and 3", len(page.Members), page.Total)
	}
}

func TestList_ServesFromTheSnapshotWithinTheTTL(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	now := base
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return now })

	for i := 0; i < 5; i++ {
		if _, err := c.List(context.Background(), Query{Limit: 10}); err != nil {
			t.Fatalf("List: %v", err)
		}
		now = now.Add(10 * time.Second)
	}
	if list, _ := store.calls(); list != 1 {
		t.Errorf("store read %d times within one TTL, want 1", list)
	}

	now = base.Add(2 * time.Minute)
	if _, err := c.List(context.Background(), Query{Limit: 10}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if list, _ := store.calls(); list != 2 {
		t.Errorf("store read %d times after the TTL expired, want 2", list)
	}
}

// Concurrent readers arriving at a cold catalogue must produce ONE read, not one each.
// The catalogue is the whole membership; a thundering herd here is a self-inflicted
// outage on the same table every authenticated request already touches.
func TestList_ConcurrentCallersRefreshOnce(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.List(context.Background(), Query{Limit: 10}); err != nil {
				t.Errorf("List: %v", err)
			}
		}()
	}
	wg.Wait()
	if list, _ := store.calls(); list != 1 {
		t.Errorf("store read %d times for 32 concurrent callers, want 1", list)
	}
}

// A failed refresh must not blank a catalogue that was already serving. A stale list is
// worth more than an empty one, and an empty one reads as "nobody is in the network".
func TestList_KeepsTheLastGoodSnapshotWhenARefreshFails(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	now := base
	store := &fakeCatalogueStore{rows: threeMembers(base)}
	c := newTestCatalogue(t, store, func() time.Time { return now })

	if _, err := c.List(context.Background(), Query{Limit: 10}); err != nil {
		t.Fatalf("first List: %v", err)
	}

	store.listErr = errors.New("database down")
	now = base.Add(2 * time.Minute)
	page, err := c.List(context.Background(), Query{Limit: 10})
	if err != nil {
		t.Fatalf("List after a failed refresh: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("total = %d after a failed refresh, want the last good snapshot's 3", page.Total)
	}
}

// A COLD catalogue that cannot read has nothing to serve, and must say so rather than
// answer "no members".
func TestList_ReportsAFailureWhenThereIsNoSnapshotYet(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{listErr: errors.New("database down")}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	if _, err := c.List(context.Background(), Query{Limit: 10}); err == nil {
		t.Error("List over a cold, unreadable catalogue returned no error")
	}
}

func TestByHandle_ReadsTheDatabaseNotTheSnapshot(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{
		rows: threeMembers(base),
		one: db.GetTalentNetworkMemberByHandleRow{
			TalentHandle:               pgtype.Text{String: "backend-aaaa", Valid: true},
			Timezone:                   pgtype.Text{String: "Europe/Berlin", Valid: true},
			Cities:                     []string{"berlin"},
			ResumeStructured:           []byte(backendCV),
			ResumeStructuredUploadedAt: pgtype.Timestamptz{Time: base, Valid: true},
			Specializations:            []string{},
		},
	}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	if _, err := c.List(context.Background(), Query{Limit: 10}); err != nil {
		t.Fatalf("List: %v", err)
	}
	m, err := c.ByHandle(context.Background(), "backend-aaaa")
	if err != nil {
		t.Fatalf("ByHandle: %v", err)
	}
	if m.Card.Category != "backend" {
		t.Errorf("card category = %q, want backend", m.Card.Category)
	}
	if _, one := store.calls(); one != 1 {
		t.Errorf("ByHandle read the database %d times, want 1 — it must not serve from the snapshot", one)
	}
}

func TestByHandle_AbsentAndMalformedAnswerTheSame(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{oneErr: pgx.ErrNoRows}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	for _, handle := range []string{"backend-zzzz", "not a handle", "", "../etc/passwd"} {
		if _, err := c.ByHandle(context.Background(), handle); !errors.Is(err, ErrNotFound) {
			t.Errorf("ByHandle(%q) error = %v, want ErrNotFound", handle, err)
		}
	}
}

// A malformed handle must be refused before it reaches the database, so a crafted path
// costs a string comparison rather than a query.
func TestByHandle_RefusesAMalformedHandleWithoutQuerying(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeCatalogueStore{}
	c := newTestCatalogue(t, store, func() time.Time { return base })

	if _, err := c.ByHandle(context.Background(), "NOT-a-Handle!"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if _, one := store.calls(); one != 0 {
		t.Errorf("the store was queried %d times for a malformed handle, want 0", one)
	}
}
