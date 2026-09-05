package ingestsched

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// ProviderReport is one line of the schedule as an operator reads it: what the provider is
// scheduled on, and what its runs have actually been doing.
type ProviderReport struct {
	Settings

	// ShardsInState counts run-state rows, NOT Settings.Shards. The two differ exactly
	// when a shard-count change has not been reconciled yet, and a report that showed the
	// intended number would hide that.
	ShardsInState int
	InFlight      int

	NextDueAt      *time.Time
	LastFinishedAt *time.Time
}

// OverrideInput is a partial edit. A nil field means "leave this alone" on an existing row
// and "use the documented default" on a new one, so changing only the shard count cannot
// silently reset a cadence someone measured.
type OverrideInput struct {
	Provider       string
	Shards         *int
	Cadence        *time.Duration
	RunTimeout     *time.Duration
	Enabled        *bool
	DisabledReason *string
	Notes          *string
	Managed        *bool
}

// Report lists every eligible provider with its effective settings and run status.
func (r *QueriesRepository) Report(ctx context.Context) ([]ProviderReport, error) {
	rows, err := r.q.ReportIngestSchedule(ctx)
	if err != nil {
		return nil, fmt.Errorf("report ingest schedule: %w", err)
	}

	out := make([]ProviderReport, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProviderReport{
			Settings: Effective(row.Provider, overrideFrom(db.ListSchedulableProvidersRow{
				Provider:       row.Provider,
				Shards:         row.Shards,
				CadenceSec:     row.CadenceSec,
				TimeoutSec:     row.TimeoutSec,
				Enabled:        row.Enabled,
				DisabledReason: row.DisabledReason,
				Notes:          row.Notes,
				Managed:        row.Managed,
			})),
			ShardsInState:  int(row.ShardsInState),
			InFlight:       int(row.InFlight),
			NextDueAt:      timePtr(row.NextDueAt),
			LastFinishedAt: timePtr(row.LastFinishedAt),
		})
	}
	return out, nil
}

// SaveOverride writes one provider's override. It does not validate the combination —
// the table's CHECK does, so a disable with no reason is refused here for exactly the same
// reason it is refused in psql.
func (r *QueriesRepository) SaveOverride(ctx context.Context, in OverrideInput) error {
	err := r.q.UpsertIngestSchedule(ctx, db.UpsertIngestScheduleParams{
		Provider:       in.Provider,
		Shards:         int4Ptr(in.Shards),
		CadenceSec:     int4Seconds(in.Cadence),
		TimeoutSec:     int4Seconds(in.RunTimeout),
		Enabled:        boolPtr(in.Enabled),
		DisabledReason: textPtr(in.DisabledReason),
		Notes:          textPtr(in.Notes),
		Managed:        boolPtr(in.Managed),
	})
	if err != nil {
		return fmt.Errorf("save override for %s: %w", in.Provider, err)
	}
	return nil
}

func timePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func int4Ptr(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: toInt32(*v, 0, maxShardOrdinal), Valid: true}
}

func int4Seconds(d *time.Duration) pgtype.Int4 {
	if d == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: seconds(*d), Valid: true}
}

func boolPtr(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
