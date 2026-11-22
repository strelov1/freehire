package viewlog

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
)

func TestTotalsRoundTrip(t *testing.T) {
	ctx := context.Background()
	mem := cache.NewMemory()
	since := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	want := Totals{Viewed: 5401347, ViewedSince: &since, ComputedAt: time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC)}
	if err := StoreTotals(ctx, mem, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadTotals(ctx, mem)
	if err != nil || !found {
		t.Fatalf("found=%t err=%v, want the stored totals", found, err)
	}
	if got.Viewed != want.Viewed || got.ViewedSince == nil || !got.ViewedSince.Equal(since) || !got.ComputedAt.Equal(want.ComputedAt) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// No cache and a cold one both read as "nothing published", never as an error: the caller
// then decides what to serve.
func TestLoadTotalsWithoutAPublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]cache.Cache{"no cache": nil, "cold cache": cache.NewMemory()} {
		if _, found, err := LoadTotals(ctx, c); found || err != nil {
			t.Errorf("%s: found=%t err=%v, want not found and no error", name, found, err)
		}
	}
}
