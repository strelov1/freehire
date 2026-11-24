package search

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeServing struct {
	rows []db.ListServingSponsoredCampaignsRow
	got  db.ListServingSponsoredCampaignsParams
}

func (f *fakeServing) ListServingSponsoredCampaigns(_ context.Context, a db.ListServingSponsoredCampaignsParams) ([]db.ListServingSponsoredCampaignsRow, error) {
	f.got = a
	return f.rows, nil
}

func TestServingCampaignsAndAttach(t *testing.T) {
	f := &fakeServing{rows: []db.ListServingSponsoredCampaignsRow{{JobID: 2, CampaignID: 9, CpcCents: 40}}}
	now := time.Date(2026, 11, 24, 23, 59, 0, 0, time.FixedZone("x", 3600*5))
	serving, err := ServingCampaigns(context.Background(), f, []int64{1, 2}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.got.Today.Time; got.Year() != 2026 || got.Month() != 11 || got.Day() != 24 || got.Location() != time.UTC {
		t.Errorf("today = %v, want the UTC day 2026-11-24", got)
	}
	var open, sponsored JobDocument
	open.ID, sponsored.ID = 1, 2
	open.AttachSponsored(serving)
	sponsored.AttachSponsored(serving)
	if open.Sponsored || open.SponsoredCPC != 0 {
		t.Errorf("unserved job marked: %+v", open)
	}
	if !sponsored.Sponsored || sponsored.SponsoredCPC != 40 {
		t.Errorf("served job not marked: %+v", sponsored)
	}
	// Written only when true; the served view flag must not leak out of the index fields.
	b, _ := json.Marshal(open)
	if s := string(b); strings.Contains(s, `"sponsored`) {
		t.Errorf("unsponsored document carries the keys: %s", s)
	}
}

func TestServingCampaignsNilReader(t *testing.T) {
	if m, err := ServingCampaigns(context.Background(), nil, []int64{1}, time.Now()); err != nil || m != nil {
		t.Fatalf("got %v, %v", m, err)
	}
}

// A hit decoded from the index must not read as a placed ad: the index key lands on the
// document's own field, never on the served jobview flag.
func TestIndexedSponsoredKeyDoesNotSetServedFlag(t *testing.T) {
	var d JobDocument
	if err := json.Unmarshal([]byte(`{"id":1,"sponsored":true,"sponsored_cpc":25}`), &d); err != nil {
		t.Fatal(err)
	}
	if !d.Sponsored || d.SponsoredCPC != 25 || d.Job.Sponsored {
		t.Fatalf("doc = %+v", d)
	}
}
