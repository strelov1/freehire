package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/viewlog"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

type viewTotalsFake struct {
	row db.GetPageViewTotalsRow
	err error
}

func (f viewTotalsFake) GetPageViewTotals(context.Context) (db.GetPageViewTotalsRow, error) {
	return f.row, f.err
}

func TestPublishViewTotalsStoresTheMeasurement(t *testing.T) {
	ctx := context.Background()
	mem := cache.NewMemory()
	since := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC)
	fake := viewTotalsFake{row: db.GetPageViewTotalsRow{Viewed: 5401347, ViewedSince: pgtype.Date{Time: since, Valid: true}}}

	if err := publishViewTotals(ctx, fake, mem, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	got, found, err := viewlog.LoadTotals(ctx, mem)
	if err != nil || !found {
		t.Fatalf("found=%t err=%v, want the published totals", found, err)
	}
	if got.Viewed != 5401347 || got.ViewedSince == nil || !got.ViewedSince.Equal(since) || !got.ComputedAt.Equal(now) {
		t.Fatalf("got %+v", got)
	}
}

// Nothing rolled up yet is a null window, never a zero date a page would print.
func TestPublishViewTotalsKeepsANullWindowNull(t *testing.T) {
	ctx := context.Background()
	mem := cache.NewMemory()
	if err := publishViewTotals(ctx, viewTotalsFake{}, mem, time.Now); err != nil {
		t.Fatal(err)
	}
	got, _, _ := viewlog.LoadTotals(ctx, mem)
	if got.ViewedSince != nil {
		t.Fatalf("viewed_since = %v, want nil", got.ViewedSince)
	}
}

// A failed measurement publishes nothing, so readers keep the previous snapshot.
func TestPublishViewTotalsStoresNothingOnFailure(t *testing.T) {
	ctx := context.Background()
	mem := cache.NewMemory()
	if err := publishViewTotals(ctx, viewTotalsFake{err: errors.New("statement timeout")}, mem, time.Now); err == nil {
		t.Fatal("want the measurement's error")
	}
	if _, found, _ := viewlog.LoadTotals(ctx, mem); found {
		t.Fatal("a failed measurement must not publish")
	}
}
