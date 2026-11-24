package sponsored

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeUpserter struct {
	fail bool
	got  []db.UpsertSponsoredDailyParams
}

func (f *fakeUpserter) UpsertSponsoredDaily(_ context.Context, a db.UpsertSponsoredDailyParams) error {
	if f.fail {
		return errors.New("down")
	}
	f.got = append(f.got, a)
	return nil
}

func TestImpressionsFlushBatchesPerCampaignDay(t *testing.T) {
	b := NewImpressions(nil)
	d1 := time.Date(2026, 11, 24, 23, 0, 0, 0, time.UTC)
	d2 := d1.Add(2 * time.Hour) // next UTC day
	b.Add(7, d1)
	b.Add(7, d1)
	b.Add(8, d1)
	b.Add(7, d2)
	up := &fakeUpserter{}
	b.store = up
	if err := b.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	total := map[int64]int64{}
	for _, g := range up.got {
		total[g.CampaignID] += g.Impressions
		if g.ValidClicks != 0 || g.InvalidClicks != 0 || g.SpentCents != 0 {
			t.Errorf("flush touched click counters: %+v", g)
		}
	}
	if len(up.got) != 3 || total[7] != 3 || total[8] != 1 {
		t.Errorf("rows=%d totals=%v, want 3 rows, 7:3 8:1", len(up.got), total)
	}
	if err := b.Flush(context.Background()); err != nil || len(up.got) != 3 {
		t.Errorf("second flush wrote again: %v", err)
	}
}

func TestImpressionsFailedFlushKeepsCounts(t *testing.T) {
	up := &fakeUpserter{fail: true}
	b := NewImpressions(up)
	now := time.Now()
	b.Add(7, now)
	if err := b.Flush(context.Background()); err == nil {
		t.Fatal("want the store error")
	}
	b.Add(7, now)
	up.fail = false
	if err := b.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(up.got) != 1 || up.got[0].Impressions != 2 {
		t.Errorf("got %+v, want one row of 2", up.got)
	}
}

func TestImpressionsNilIsNoop(t *testing.T) {
	var b *Impressions
	b.Add(1, time.Now()) // an unconfigured placement must not need a nil check
}
