// Package ingestsched decides when each provider's crawl is due, claims a due run exactly
// once, and hands it to a launcher. It replaces deploy/bin/gen-ingest-timers.sh, which
// materialised one static systemd timer per provider from a script nothing on the host
// invoked — so between its manual runs the schedule was a photograph of a catalog that had
// since moved, and every divergence was silent.
//
// The rule the package is built around: the ROSTER is the boards table. This package's own
// ingest_schedule table holds OVERRIDES. A provider with no override row is scheduled on
// the defaults below; not crawling a provider requires a row that says so and says why.
package ingestsched

import "time"

// What a provider with no override row is scheduled on. These are the values ~226 of the
// 238 live providers ran on under the script this replaces, so they are the fleet's actual
// shape rather than a conservative guess.
const (
	// DefaultShards crawls a provider whole. Only seven providers are large enough to
	// need partitioning.
	DefaultShards = 1

	// DefaultCadence matches the hourly per-provider timers the script generated.
	DefaultCadence = time.Hour

	// DefaultRunTimeout is 2400s of crawl budget plus the 600s worst-case wait the
	// retired ingest-slot.sh semaphore could impose, kept whole so the number stays
	// comparable to the units it replaces.
	DefaultRunTimeout = 3000 * time.Second
)

// Override is a curator's ingest_schedule row: the settings for one provider, and the
// measurement that justifies them.
//
// Every field is populated — the columns are NOT NULL with defaults — so an override
// speaks for the whole provider rather than for individual fields. That is deliberate: a
// row explicitly setting the hourly cadence is still an override, and inferring
// per-field provenance by comparing against the defaults would call it one only by
// accident.
type Override struct {
	Provider       string
	Shards         int
	Cadence        time.Duration
	RunTimeout     time.Duration
	Enabled        bool
	DisabledReason string
	Notes          string
	Managed        bool
}

// Settings is what a provider is actually scheduled on, after an override (if any) is
// applied to the defaults.
type Settings struct {
	Provider       string
	Shards         int
	Cadence        time.Duration
	RunTimeout     time.Duration
	Enabled        bool
	DisabledReason string
	Notes          string

	// Managed gates the rollout: while the static timers still run, only a managed
	// provider is launched, so the two cannot both drive one provider. Removed with the
	// column once every provider is cut over.
	Managed bool

	// Overridden is false when the provider has no ingest_schedule row at all, which is
	// what the report shows as "running on defaults".
	Overridden bool
}

// ShardSelector is one `--shard=Index/Of` run. An unsharded provider is 1/1, so callers
// have one shape to handle rather than two.
type ShardSelector struct {
	Index int
	Of    int
}

// Effective resolves what a provider is scheduled on. A nil override is the ordinary case
// and the point of the design: absence means scheduled on defaults, never unscheduled.
func Effective(provider string, o *Override) Settings {
	if o == nil {
		return Settings{
			Provider:   provider,
			Shards:     DefaultShards,
			Cadence:    DefaultCadence,
			RunTimeout: DefaultRunTimeout,
			Enabled:    true,
		}
	}
	return Settings{
		Provider:       provider,
		Shards:         o.Shards,
		Cadence:        o.Cadence,
		RunTimeout:     o.RunTimeout,
		Enabled:        o.Enabled,
		DisabledReason: o.DisabledReason,
		Notes:          o.Notes,
		Managed:        o.Managed,
		Overridden:     true,
	}
}

// Schedulable reports whether the scheduler should keep run state for this provider and
// launch it. A disabled provider's run state is DELETED rather than kept idle, so
// re-enabling starts from a fresh due time instead of a months-old one that would fire
// immediately — and so the claim query needs no `enabled` predicate of its own.
//
// The Managed conjunct is rollout-only and goes away with the column in task 8.5 of
// openspec/changes/ingest-scheduler-in-db; until then it is what stops the scheduler and
// the static timers from both driving one provider.
func (s Settings) Schedulable() bool { return s.Enabled && s.Managed }

// ShardSelectors lists the runs that together cover this provider once.
func (s Settings) ShardSelectors() []ShardSelector {
	out := make([]ShardSelector, 0, s.Shards)
	for i := 1; i <= s.Shards; i++ {
		out = append(out, ShardSelector{Index: i, Of: s.Shards})
	}
	return out
}
