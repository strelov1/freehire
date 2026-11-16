//go:build integration

package ingestsched

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestDiscoveredProviderIsScheduledWithoutChangingOperatorOverrides(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "hrsmart", "gistworld.mua.hrdepartment.com")
	if err := repo.AdoptDiscoveredProvider(ctx, "hrsmart"); err != nil {
		t.Fatal(err)
	}
	r := reportFor(t, repo, "hrsmart")
	if !r.Schedulable() || r.Cadence != DefaultCadence || r.RunTimeout != DefaultRunTimeout {
		t.Fatalf("new native provider is not scheduled on existing defaults: %+v", r)
	}
	if err := repo.SaveOverride(ctx, OverrideInput{Provider: "hrsmart", Enabled: ptrBool(false), Managed: ptrBool(false), DisabledReason: ptrStr("Operator paused this provider")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AdoptDiscoveredProvider(ctx, "hrsmart"); err != nil {
		t.Fatal(err)
	}
	r = reportFor(t, repo, "hrsmart")
	if r.Enabled || r.Managed || r.DisabledReason != "Operator paused this provider" {
		t.Fatalf("discovery overwrote an operator decision: %+v", r)
	}
	addBoard(t, pool, "lever", "existing-board")
	addBoard(t, pool, "lever", "newly-discovered-board")
	if err := repo.AdoptDiscoveredProvider(ctx, "lever"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(pool).GetIngestSchedule(ctx, "lever"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("discovery took over an existing static provider: %v", err)
	}
}

func TestDiscoveryAdoptsAFirstLiveBoardDespiteRetiredOrRejectedHistory(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "hrsmart", "rejected.example.hrdepartment.com")
	addBoard(t, pool, "hrsmart", "retired.example.hrdepartment.com")
	if _, err := pool.Exec(ctx, "UPDATE boards SET status=CASE WHEN board LIKE 'rejected.%' THEN 'rejected' ELSE 'retired' END WHERE provider='hrsmart'"); err != nil {
		t.Fatal(err)
	}
	addBoard(t, pool, "hrsmart", "gistworld.mua.hrdepartment.com")
	if err := repo.AdoptDiscoveredProvider(ctx, "hrsmart"); err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, repo, "hrsmart"); !r.Schedulable() {
		t.Fatalf("inactive history blocked the first live ATS: %+v", r)
	}
}
