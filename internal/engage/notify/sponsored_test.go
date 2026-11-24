package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type serving map[int64]int64

func (s serving) ListServingSponsoredCampaigns(_ context.Context, a db.ListServingSponsoredCampaignsParams) ([]db.ListServingSponsoredCampaignsRow, error) {
	var rows []db.ListServingSponsoredCampaignsRow
	for _, id := range a.JobIds {
		if c, ok := s[id]; ok {
			rows = append(rows, db.ListServingSponsoredCampaignsRow{JobID: id, CampaignID: c, CpcCents: 10})
		}
	}
	return rows, nil
}

type sink struct {
	rows []db.UpsertSponsoredDailyParams
}

func (s *sink) UpsertSponsoredDaily(_ context.Context, a db.UpsertSponsoredDailyParams) error {
	s.rows = append(s.rows, a)
	return nil
}

var adKey = []byte("k")

func emailStore(query string) *fakeStore {
	return &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, UserID: 42, Channel: ChannelEmail, SavedSearchName: "Go", SavedSearchQuery: query,
				AccountEmail: "a@example.test"},
		},
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 7,
	}
}

func adHit(id int64) search.JobDocument {
	return search.JobDocument{ID: id, Job: jobview.Job{Title: "Sponsored role", Company: "Acme", PublicSlug: fmt.Sprintf("s-%d", id)}}
}

func runWithAds(t *testing.T, store *fakeStore, s *fakeSearcher, key []byte, imp *sponsored.Impressions) *fakeNotifier {
	t.Helper()
	n := &fakeNotifier{}
	r := New(store, s, n, DefaultConfig()).WithSponsored(serving{50: 500}, key, imp)
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEmailDigestCarriesOneSponsoredJob(t *testing.T) {
	sk := &sink{}
	imp := sponsored.NewImpressions(sk)
	s := &fakeSearcher{results: []search.SearchResult{{Hits: []search.JobDocument{adHit(50)}}}}
	n := runWithAds(t, emailStore("q=go&remote=remote"), s, adKey, imp)

	if len(n.sent) != 1 || n.sent[0].Sponsored == nil {
		t.Fatalf("digest = %+v", n.sent)
	}
	slot := n.sent[0].Sponsored
	if slot.Job.Title != "Sponsored role" || slot.CampaignID != 500 || !strings.HasPrefix(slot.Path, sponsored.ClickPath) {
		t.Errorf("slot = %+v", slot)
	}
	c, err := sponsored.VerifyToken(adKey, strings.TrimPrefix(slot.Path, sponsored.ClickPath), time.Now())
	if err != nil || c.Placement != sponsored.PlacementEmail || c.JobID != 50 || c.CampaignID != 500 {
		t.Errorf("claims = %+v, %v", c, err)
	}
	p := s.calls[0]
	if p.Query != "go" || p.Limit != 1 || p.Sort[0] != "sponsored_cpc:desc" || !strings.Contains(fmt.Sprint(p.Filter), "sponsored = true") {
		t.Errorf("params = %+v", p)
	}
	_ = imp.Flush(context.Background())
	if len(sk.rows) != 1 || sk.rows[0].CampaignID != 500 || sk.rows[0].Impressions != 1 {
		t.Errorf("impressions = %+v", sk.rows)
	}
}

func TestEmailDigestWithoutSponsored(t *testing.T) {
	hit := &fakeSearcher{results: []search.SearchResult{{Hits: []search.JobDocument{adHit(50)}}}}
	cases := []struct {
		name  string
		store *fakeStore
		s     *fakeSearcher
		key   []byte
	}{
		{"no key", emailStore("q=go"), hit, nil},
		{"search fails", emailStore("q=go"), &fakeSearcher{err: errors.New("down")}, adKey},
		{"nothing matches", emailStore("q=go"), &fakeSearcher{}, adKey},
		{"unfiltered alert", emailStore(""), hit, adKey},
		{"hit no longer serving", emailStore("q=go"), &fakeSearcher{results: []search.SearchResult{{Hits: []search.JobDocument{adHit(99)}}}}, adKey},
	}
	for _, c := range cases {
		n := runWithAds(t, c.store, c.s, c.key, nil)
		// The mail still goes out, plain.
		if len(n.sent) != 1 || n.sent[0].Sponsored != nil {
			t.Errorf("%s: sent=%+v", c.name, n.sent)
		}
	}
}

func TestOtherChannelsGetNoSponsored(t *testing.T) {
	store := emailStore("q=go")
	d := store.delivery[1]
	d.Channel, d.TelegramChatID = ChannelTelegram, pgtype.Int8{Int64: 5, Valid: true}
	store.delivery[1] = d
	s := &fakeSearcher{results: []search.SearchResult{{Hits: []search.JobDocument{adHit(50)}}}}
	n := runWithAds(t, store, s, adKey, nil)
	if len(n.sent) != 1 || n.sent[0].Sponsored != nil || len(s.calls) != 0 {
		t.Errorf("sent=%+v calls=%d", n.sent, len(s.calls))
	}
}
