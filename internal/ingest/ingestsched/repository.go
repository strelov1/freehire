package ingestsched

import (
	"context"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Run is one claimed slice of work: which provider, which shard of how many, and the
// budget its transient unit gets. The timeout travels WITH the claim rather than being
// looked up again at launch, so a cadence edit landing between the two cannot hand a run a
// budget the scheduler never decided on.
type Run struct {
	Provider   string
	Shard      int
	RunTimeout time.Duration
}

// Repository is the scheduling persistence contract, in package domain types.
type Repository interface {
	// Eligible lists every provider with a live board, each resolved against its
	// override row if it has one. A provider with no row comes back on defaults.
	Eligible(ctx context.Context) ([]Settings, error)
	// Reconcile makes run state match the given settings: one row per shard, surplus
	// shards dropped, and providers absent from the list forgotten entirely.
	Reconcile(ctx context.Context, settings []Settings) error
	// Claim takes up to limit due runs and marks them started. A claim older than its
	// provider's timeout plus grace is treated as dead and may be taken again.
	Claim(ctx context.Context, limit int, grace time.Duration) ([]Run, error)
	// RecordFinish stores a run's outcome and releases its claim.
	RecordFinish(ctx context.Context, provider string, shard, exitCode int, runErr string) error
}

// QueriesRepository is the sqlc-backed Repository.
type QueriesRepository struct {
	q *db.Queries
}

func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func (r *QueriesRepository) Eligible(ctx context.Context) ([]Settings, error) {
	rows, err := r.q.ListSchedulableProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schedulable providers: %w", err)
	}

	out := make([]Settings, 0, len(rows))
	for _, row := range rows {
		out = append(out, Effective(row.Provider, overrideFrom(row)))
	}
	return out, nil
}

// overrideFrom reads the LEFT JOIN's nullable half. Shards decides: it is NOT NULL in
// ingest_schedule, so a NULL here means the join found no row at all, which is exactly
// "this provider has no override" and not "this provider has an override with no shards".
func overrideFrom(row db.ListSchedulableProvidersRow) *Override {
	if !row.Shards.Valid {
		return nil
	}
	return &Override{
		Provider:       row.Provider,
		Shards:         int(row.Shards.Int32),
		Cadence:        time.Duration(row.CadenceSec.Int32) * time.Second,
		RunTimeout:     time.Duration(row.TimeoutSec.Int32) * time.Second,
		Enabled:        row.Enabled.Bool,
		DisabledReason: row.DisabledReason.String,
		Notes:          row.Notes.String,
		Managed:        row.Managed.Bool,
	}
}

func (r *QueriesRepository) Reconcile(ctx context.Context, settings []Settings) error {
	providers := make([]string, 0, len(settings))
	for _, s := range settings {
		providers = append(providers, s.Provider)

		if err := r.q.EnsureRunStateShards(ctx, db.EnsureRunStateShardsParams{
			Provider: s.Provider,
			Shards:   int32(s.Shards),
		}); err != nil {
			return fmt.Errorf("ensure shards for %s: %w", s.Provider, err)
		}
		if err := r.q.DeleteSurplusRunStateShards(ctx, db.DeleteSurplusRunStateShardsParams{
			Provider: s.Provider,
			Shards:   int32(s.Shards),
		}); err != nil {
			return fmt.Errorf("drop surplus shards for %s: %w", s.Provider, err)
		}
	}

	if err := r.q.DeleteRunStateForUnlistedProviders(ctx, providers); err != nil {
		return fmt.Errorf("forget departed providers: %w", err)
	}
	return nil
}

func (r *QueriesRepository) Claim(ctx context.Context, limit int, grace time.Duration) ([]Run, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := r.q.ClaimDueRuns(ctx, db.ClaimDueRunsParams{
		DefaultCadenceSec: int32(DefaultCadence.Seconds()),
		DefaultTimeoutSec: int32(DefaultRunTimeout.Seconds()),
		GraceSec:          int32(grace.Seconds()),
		MaxRuns:           int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("claim due runs: %w", err)
	}

	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, Run{
			Provider:   row.Provider,
			Shard:      int(row.Shard),
			RunTimeout: time.Duration(row.TimeoutSec) * time.Second,
		})
	}
	return out, nil
}

func (r *QueriesRepository) RecordFinish(ctx context.Context, provider string, shard, exitCode int, runErr string) error {
	err := r.q.RecordRunFinish(ctx, db.RecordRunFinishParams{
		Provider:  provider,
		Shard:     int32(shard),
		ExitCode:  int32(exitCode),
		LastError: runErr,
	})
	if err != nil {
		return fmt.Errorf("record finish for %s/%d: %w", provider, shard, err)
	}
	return nil
}
