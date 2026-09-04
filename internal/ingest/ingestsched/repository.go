package ingestsched

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Run is one claimed slice of work: which provider, which shard of how many, and the
// budget its transient unit gets. The timeout travels WITH the claim rather than being
// looked up again at launch, so a cadence edit landing between the two cannot hand a run a
// budget the scheduler never decided on.
type Run struct {
	Provider string
	// Shard and Shards together are the `--shard=i/n` selector. Shards == 1 means the
	// provider is crawled whole and no selector is passed at all.
	Shard      int
	Shards     int
	RunTimeout time.Duration
}

// Repository is what the SCHEDULER needs from the store, and nothing more. The reporting
// and editing reads behind cmd/schedule-board are deliberately absent: a tick has no
// business being able to rewrite a curator's cadence, and a fake standing in for this
// interface should not have to implement two methods the code under test never calls.
type Repository interface {
	// Eligible lists every provider with a live board, each resolved against its
	// override row if it has one. A provider with no row comes back on defaults.
	Eligible(ctx context.Context) ([]Settings, error)
	// Reconcile makes run state match the given settings: one row per shard, surplus
	// shards dropped, and providers absent from the list forgotten entirely. A provider
	// that could not be reconciled comes back in the returned slice rather than failing
	// the call — one bad row must not stop the fleet.
	Reconcile(ctx context.Context, settings []Settings) ([]Skipped, error)
	// InFlightRuns lists the claimed runs. The scheduler asks the service manager about
	// each before counting it, because a transient unit that finished tells nobody — a
	// plain count would include every run that ever succeeded, and the fleet would
	// saturate permanently.
	InFlightRuns(ctx context.Context) ([]Run, error)
	// Claim takes up to limit due runs and marks them started. A claim older than its
	// provider's timeout plus grace is treated as dead and may be taken again.
	Claim(ctx context.Context, limit int, grace time.Duration) ([]Run, error)
	// PreviewDue reports what Claim WOULD take, without taking it. Shadow mode's read.
	PreviewDue(ctx context.Context, limit int, grace time.Duration) ([]Run, error)
	// RecordFinish stores a run's outcome and releases its claim.
	RecordFinish(ctx context.Context, provider string, shard, exitCode int, runErr string) error
}

// The bounds every int -> int32 conversion in this package is squeezed through. Go's
// conversion WRAPS on overflow rather than failing, so an unbounded one stores a value the
// caller never meant and nothing anywhere reports a problem — CodeQL flags the shape, and
// it is right to. Each bound is chosen to be far past any real value and far short of
// int32.
const (
	// maxShardOrdinal mirrors the schema's upper bound on ingest_schedule.shards.
	maxShardOrdinal = 64
	// maxSeconds is a year. A cadence or a timeout past that is not a schedule.
	maxSeconds = 366 * 24 * 3600
	// maxRuns bounds one tick's claim. The fleet's real cap is 10.
	maxRuns = 1000
)

// seconds converts a duration to the bounded int32 the schema stores.
func seconds(d time.Duration) int32 {
	return toInt32(int(d/time.Second), 0, maxSeconds)
}

// toInt32 bounds v into [lo, hi] and then converts.
//
// The guards and the conversion sit together on purpose: a clamp that returned an int left
// the analyser reporting an unbounded conversion at each call site, correctly, since it
// could not follow the bound across the return. The final pair of guards is against
// CONSTANTS rather than the lo/hi parameters, for the same reason one step further in — a
// bound that arrives as an argument proves nothing about this line on its own. Every real
// hi here is far below MaxInt32, so those two guards never fire; they are what makes the
// conversion provably safe rather than safe-by-inspection.
func toInt32(v, lo, hi int) int32 {
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	if v > math.MaxInt32 {
		v = math.MaxInt32
	}
	if v < math.MinInt32 {
		v = math.MinInt32
	}
	return int32(v)
}

// clamp bounds v into [lo, hi], for callers that stay in int.
func clamp(v, lo, hi int) int {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	default:
		return v
	}
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

// Reconcile makes run state match settings, one provider at a time.
//
// A provider whose reconcile fails is REPORTED and stepped over, not returned as the whole
// call's error. The per-provider timers this replaces had that isolation for free, and
// giving it up would mean one bad row — a lock timeout, a shard count nobody bounded —
// stops every provider on every tick until somebody finds it.
//
// A failed provider still counts as PRESENT for the departed-provider delete below. Its
// rows are not what failed, and dropping them would turn a transient error into a lost
// stagger and a lost run history.
func (r *QueriesRepository) Reconcile(ctx context.Context, settings []Settings) ([]Skipped, error) {
	providers := make([]string, 0, len(settings))
	var failed []Skipped

	for _, s := range settings {
		providers = append(providers, s.Provider)
		if err := r.reconcileOne(ctx, s); err != nil {
			failed = append(failed, Skipped{s.Provider, err.Error()})
		}
	}

	if err := r.q.DeleteRunStateForUnlistedProviders(ctx, providers); err != nil {
		return failed, fmt.Errorf("forget departed providers: %w", err)
	}
	return failed, nil
}

func (r *QueriesRepository) reconcileOne(ctx context.Context, s Settings) error {
	if err := r.q.EnsureRunStateShards(ctx, db.EnsureRunStateShardsParams{
		Provider: s.Provider,
		Shards:   toInt32(s.Shards, 1, maxShardOrdinal),
	}); err != nil {
		return fmt.Errorf("ensure shards: %w", err)
	}
	if err := r.q.DeleteSurplusRunStateShards(ctx, db.DeleteSurplusRunStateShardsParams{
		Provider: s.Provider,
		Shards:   toInt32(s.Shards, 1, maxShardOrdinal),
	}); err != nil {
		return fmt.Errorf("drop surplus shards: %w", err)
	}
	return nil
}

func (r *QueriesRepository) Claim(ctx context.Context, limit int, grace time.Duration) ([]Run, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := r.q.ClaimDueRuns(ctx, db.ClaimDueRunsParams{
		DefaultCadenceSec: seconds(DefaultCadence),
		DefaultTimeoutSec: seconds(DefaultRunTimeout),
		GraceSec:          seconds(grace),
		MaxRuns:           toInt32(limit, 0, maxRuns),
	})
	if err != nil {
		return nil, fmt.Errorf("claim due runs: %w", err)
	}

	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, Run{
			Provider:   row.Provider,
			Shard:      int(row.Shard),
			Shards:     int(row.Shards),
			RunTimeout: time.Duration(row.TimeoutSec) * time.Second,
		})
	}
	return out, nil
}

func (r *QueriesRepository) InFlightRuns(ctx context.Context) ([]Run, error) {
	rows, err := r.q.ListInFlightRuns(ctx, seconds(DefaultRunTimeout))
	if err != nil {
		return nil, fmt.Errorf("list in-flight runs: %w", err)
	}

	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, Run{
			Provider:   row.Provider,
			Shard:      int(row.Shard),
			Shards:     int(row.Shards),
			RunTimeout: time.Duration(row.TimeoutSec) * time.Second,
		})
	}
	return out, nil
}

func (r *QueriesRepository) PreviewDue(ctx context.Context, limit int, grace time.Duration) ([]Run, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := r.q.PreviewDueRuns(ctx, db.PreviewDueRunsParams{
		DefaultTimeoutSec: seconds(DefaultRunTimeout),
		GraceSec:          seconds(grace),
		MaxRuns:           toInt32(limit, 0, maxRuns),
	})
	if err != nil {
		return nil, fmt.Errorf("preview due runs: %w", err)
	}

	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, Run{
			Provider:   row.Provider,
			Shard:      int(row.Shard),
			Shards:     int(row.Shards),
			RunTimeout: time.Duration(row.TimeoutSec) * time.Second,
		})
	}
	return out, nil
}

func (r *QueriesRepository) RecordFinish(ctx context.Context, provider string, shard, exitCode int, runErr string) error {
	// Bounded again here, not only where the status was parsed. Both columns are int32,
	// and an out-of-range int WRAPS on conversion rather than failing — so a value that
	// slipped past the parser would be stored as a status the run never had. Two guards
	// because this method is the one that writes, and it takes a plain int from anywhere.
	err := r.q.RecordRunFinish(ctx, db.RecordRunFinishParams{
		Provider:  provider,
		Shard:     toInt32(shard, 0, maxShardOrdinal),
		ExitCode:  toInt32(exitCode, 0, maxExitStatus),
		LastError: runErr,
	})
	if err != nil {
		return fmt.Errorf("record finish for %s/%d: %w", provider, shard, err)
	}
	return nil
}
