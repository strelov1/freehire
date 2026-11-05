package notify

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

func TestSearchDue(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) pgtype.Timestamptz { return ts(now.Add(-d)) }
	at := func(h int) pgtype.Time {
		return pgtype.Time{Microseconds: int64(h) * int64(time.Hour/time.Microsecond), Valid: true}
	}
	interval := func(sent, searched pgtype.Timestamptz) db.ListActiveSubscriptionsRow {
		return db.ListActiveSubscriptionsRow{DigestFrequency: FrequencyInterval, DigestIntervalHours: 1, LastDigestSentAt: sent, LastSearchedAt: searched}
	}
	daily := func(digestHour int, sent, searched pgtype.Timestamptz) db.ListActiveSubscriptionsRow {
		return db.ListActiveSubscriptionsRow{DigestFrequency: FrequencyDaily, DigestTime: at(digestHour), LastDigestSentAt: sent, LastSearchedAt: searched}
	}
	quiet := interval(ago(2*time.Hour), ago(2*time.Hour))
	quiet.QuietHoursStart, quiet.QuietHoursEnd = at(11), at(13)

	cases := []struct {
		name string
		subs []db.ListActiveSubscriptionsRow
		want bool
	}{
		{"never searched", []db.ListActiveSubscriptionsRow{interval(ago(time.Minute), pgtype.Timestamptz{})}, true},
		{"one of the person's subscriptions never searched",
			[]db.ListActiveSubscriptionsRow{interval(ago(time.Minute), ago(time.Minute)), interval(pgtype.Timestamptz{}, pgtype.Timestamptz{})}, true},
		{"instant searches every pass",
			[]db.ListActiveSubscriptionsRow{{DigestFrequency: FrequencyInstant, LastSearchedAt: ago(time.Minute)}}, true},
		{"interval not elapsed: the message could not go out", []db.ListActiveSubscriptionsRow{interval(ago(30*time.Minute), ago(30*time.Minute))}, false},
		{"interval elapsed, not searched since", []db.ListActiveSubscriptionsRow{interval(ago(65*time.Minute), ago(65*time.Minute))}, true},
		{"interval elapsed, searched since and found nothing", []db.ListActiveSubscriptionsRow{interval(ago(3*time.Hour), ago(10*time.Minute))}, false},
		{"interval elapsed, an interval since the empty search", []db.ListActiveSubscriptionsRow{interval(ago(3*time.Hour), ago(61*time.Minute))}, true},
		{"never sent, searched recently", []db.ListActiveSubscriptionsRow{interval(pgtype.Timestamptz{}, ago(10*time.Minute))}, false},
		{"the newest send across the person decides",
			[]db.ListActiveSubscriptionsRow{interval(ago(5*time.Hour), ago(2*time.Hour)), interval(ago(20*time.Minute), ago(2*time.Hour))}, false},
		{"quiet hours defer the message, so the search", []db.ListActiveSubscriptionsRow{quiet}, false},
		{"daily due", []db.ListActiveSubscriptionsRow{daily(9, ago(27*time.Hour), ago(27*time.Hour))}, true},
		{"daily already sent today", []db.ListActiveSubscriptionsRow{daily(9, ago(2*time.Hour), ago(2*time.Hour))}, false},
		{"daily not yet its hour", []db.ListActiveSubscriptionsRow{daily(15, ago(21*time.Hour), ago(21*time.Hour))}, false},
		{"daily due, searched within the hour", []db.ListActiveSubscriptionsRow{daily(9, ago(27*time.Hour), ago(20*time.Minute))}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := searchDue(now, c.subs); got != c.want {
				t.Errorf("searchDue = %v, want %v", got, c.want)
			}
		})
	}
}

// A pass stamps what it searched, so the next pass five minutes later — before the
// person's interval has elapsed — does not search the same query again.
func TestMatch_SkipsAQuerySearchedThisInterval(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{active: []db.ListActiveSubscriptionsRow{
		{ID: 1, UserID: 1, Channel: ChannelEmail, Query: "skills=go", StartAt: ts(now.Add(-48 * time.Hour)),
			DigestFrequency: FrequencyInterval, DigestIntervalHours: 1, LastDigestSentAt: ts(now.Add(-time.Minute))},
	}}
	searcher := &fakeSearcher{}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())
	r.now = func() time.Time { return now }

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Queries != 1 || len(store.searched) != 1 || !store.active[0].LastSearchedAt.Time.Equal(now) {
		t.Fatalf("first pass: queries=%d stamps=%v", stats.Queries, store.searched)
	}

	now = now.Add(5 * time.Minute)
	stats, err = r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(searcher.calls) != 1 || stats.SkippedQueries != 1 {
		t.Errorf("second pass: search calls=%d skipped=%d, want 1 and 1", len(searcher.calls), stats.SkippedQueries)
	}
}

// A query searched after a gap pages back to its previous search, so an hour that
// matched more than one page loses nothing — and stops there.
func TestMatch_PagesBackToThePreviousSearch(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	lastSearched := now.Add(-time.Hour)
	store := &fakeStore{active: []db.ListActiveSubscriptionsRow{
		{ID: 1, UserID: 1, Channel: ChannelEmail, Query: "skills=go", StartAt: ts(now.Add(-48 * time.Hour)),
			DigestFrequency: FrequencyInterval, DigestIntervalHours: 1,
			LastDigestSentAt: ts(lastSearched), LastSearchedAt: ts(lastSearched)},
	}}
	page := func(first int64, oldest time.Time) search.SearchResult {
		hits := []search.JobDocument{hit(first, now), hit(first+1, oldest)}
		return search.SearchResult{Hits: hits}
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		page(1, now.Add(-30*time.Minute)),          // newer than the last search
		page(3, lastSearched.Add(-30*time.Minute)), // still within matchLag of it
		page(5, lastSearched.Add(-2*time.Hour)),    // past it: the last page read
		page(7, lastSearched.Add(-3*time.Hour)),    // never read
	}}
	cfg := DefaultConfig()
	cfg.MatchLimit = 2
	r := New(store, searcher, &fakeNotifier{}, cfg)
	r.now = func() time.Time { return now }

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(searcher.calls) != 3 || stats.Searches != 3 {
		t.Fatalf("search calls=%d searches=%d, want 3", len(searcher.calls), stats.Searches)
	}
	for i, c := range searcher.calls {
		if c.Offset != i*2 {
			t.Errorf("call %d offset=%d, want %d", i, c.Offset, i*2)
		}
	}
	if len(store.recorded) != 6 {
		t.Errorf("recorded=%d, want 6 (every hit on the three pages read)", len(store.recorded))
	}
}

// A never-searched group reads one page, as every search did before paging.
func TestMatch_FirstSearchReadsOnePage(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{active: []db.ListActiveSubscriptionsRow{
		{ID: 1, UserID: 1, Channel: ChannelEmail, Query: "skills=go", StartAt: ts(now.Add(-48 * time.Hour)), DigestFrequency: FrequencyInterval, DigestIntervalHours: 1},
	}}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hit(1, now), hit(2, now)}},
		{Hits: []search.JobDocument{hit(3, now), hit(4, now)}},
	}}
	cfg := DefaultConfig()
	cfg.MatchLimit = 2
	r := New(store, searcher, &fakeNotifier{}, cfg)
	r.now = func() time.Time { return now }

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(searcher.calls) != 1 {
		t.Errorf("search calls=%d, want 1", len(searcher.calls))
	}
}

// A saved query the search cannot parse fails every pass and never delivers, so
// the pass pauses its subscriptions and does not search it; the rest run as usual.
func TestMatch_PausesASubscriptionWhoseQueryCannotRun(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	broken := "q=go+OR+rust+OR+java+OR+python+OR+ruby+OR+php"
	store := &fakeStore{active: []db.ListActiveSubscriptionsRow{
		{ID: 1, UserID: 1, Channel: ChannelEmail, Query: broken, StartAt: ts(now)},
		{ID: 2, UserID: 2, Channel: ChannelTelegram, Query: broken, StartAt: ts(now)},
		{ID: 3, UserID: 3, Channel: ChannelEmail, Query: "skills=go", StartAt: ts(now)},
	}}
	searcher := &fakeSearcher{}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())
	r.now = func() time.Time { return now }

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.paused) != 2 || stats.Paused != 2 {
		t.Errorf("paused=%v stats.Paused=%d, want subscriptions 1 and 2", store.paused, stats.Paused)
	}
	if len(searcher.calls) != 1 || stats.FailedQueries != 0 {
		t.Errorf("search calls=%d failed=%d, want only the runnable query searched", len(searcher.calls), stats.FailedQueries)
	}
}
