package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/sources"
)

// providerStatus is the public health verdict for a provider (and the fleet):
// operational, degraded, or down. String values are the wire encoding.
type providerStatus string

const (
	statusOperational providerStatus = "operational"
	statusDegraded    providerStatus = "degraded"
	statusDown        providerStatus = "down"
)

// Status-derivation thresholds. The healthy fraction (healthy/total) classifies a
// provider so a handful of failing boards can't drag a thousand-board provider
// into "degraded". Success freshness guards against a provider whose boards all
// read healthy but haven't actually succeeded in a long time (a stalled crawl).
// These are the single knobs to tune the policy.
const (
	// healthyOperationalFrac: at or above this fraction healthy (and fresh) is green.
	healthyOperationalFrac = 0.9
	// healthyDownFrac: at or below this fraction healthy is red regardless of freshness.
	healthyDownFrac = 0.1
	// successFreshness: a provider with no success within this window reads down.
	successFreshness = 48 * time.Hour
)

// providerRollup is the derivation input for one provider: only the facts the
// status policy needs (board totals and last-success instant), decoupled from the
// db row so deriveStatus is a pure, unit-testable function. healthy counts boards
// being served (not in an active cooldown), so a board that merely erred once but is
// still crawled every cycle counts as healthy — only a board the backoff actually
// sidelined is unhealthy. A zero lastSuccess means "never succeeded".
type providerRollup struct {
	total       int64
	healthy     int64
	lastSuccess time.Time
}

// deriveStatus maps a provider's rollup to its status at instant now:
//   - down    when it has no boards, no fresh success, or ≤10% healthy;
//   - operational when ≥90% healthy and fresh;
//   - degraded  otherwise.
func deriveStatus(r providerRollup, now time.Time) providerStatus {
	if r.total <= 0 {
		return statusDown
	}
	fresh := !r.lastSuccess.IsZero() && now.Sub(r.lastSuccess) <= successFreshness
	if !fresh {
		return statusDown
	}
	frac := float64(r.healthy) / float64(r.total)
	switch {
	case frac <= healthyDownFrac:
		return statusDown
	case frac >= healthyOperationalFrac:
		return statusOperational
	default:
		return statusDegraded
	}
}

// fleetStatus is the overall verdict, derived by folding every provider into one
// fleet-wide rollup — the served fraction across ALL boards and the freshest success —
// rather than taking the worst single provider. A handful of small blocked providers
// can't red a fleet that is broadly healthy, while a broad outage (most boards cooled)
// or a fleet-wide stall (no provider fresh) still surfaces. An empty fleet is
// operational (nothing is broken).
func fleetStatus(rolls []providerRollup, now time.Time) providerStatus {
	if len(rolls) == 0 {
		return statusOperational
	}
	var fleet providerRollup
	for _, r := range rolls {
		fleet.total += r.total
		fleet.healthy += r.healthy
		if r.lastSuccess.After(fleet.lastSuccess) {
			fleet.lastSuccess = r.lastSuccess
		}
	}
	return deriveStatus(fleet, now)
}

// statusProvider is the public, sanitized per-provider entry: board counts,
// freshness, and the derived status. It deliberately has no field for last_error
// or board identifiers — sanitization by construction, so an internal detail
// cannot leak by omission.
type statusProvider struct {
	Provider      string         `json:"provider"`
	Kind          string         `json:"kind"`
	Status        providerStatus `json:"status"`
	TotalBoards   int64          `json:"total_boards"`
	HealthyBoards int64          `json:"healthy_boards"`
	CooledBoards  int64          `json:"cooled_boards"`
	LastRun       *string        `json:"last_run"`
	LastSuccess   *string        `json:"last_success"`
	IngestedTotal int64          `json:"ingested_total"`
}

// IngestStatus serves the public, unauthenticated ingest-fleet status: a
// per-provider health rollup over board_health with a derived operational/
// degraded/down status per provider and an overall fleet status, plus
// last_job_added_at — the created_at of the most recently added open, public
// job, a live signal that the pipeline is actually writing rows (independent
// of per-provider crawl health). Sanitized by construction — the DTO carries
// no error text or board identifier — so no internal detail can leak. An
// empty fleet yields overall "operational" with no providers and a null
// last_job_added_at.
func (h *statsHandlers) IngestStatus(c *fiber.Ctx) error {
	rows, err := h.queries.ProviderHealthRollup(c.Context())
	if err != nil {
		return err
	}
	lastJobAddedAt, err := h.queries.LatestOpenJobAddedAt(c.Context())
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	// One registry per request to classify each provider by adapter kind (ATS /
	// aggregator / company page). Taxonomy, not a crawl registry — this host holds
	// no ingest credentials.
	reg := sources.Taxonomy()
	providers := make([]statusProvider, len(rows))
	rolls := make([]providerRollup, len(rows))
	for i, r := range rows {
		rolls[i] = providerRollup{
			total:       r.TotalBoards,
			healthy:     r.HealthyBoards,
			lastSuccess: tsTime(r.LastSuccessAt),
		}
		providers[i] = statusProvider{
			Provider:      r.Provider,
			Kind:          sources.ProviderKind(reg, r.Provider),
			Status:        deriveStatus(rolls[i], now),
			TotalBoards:   r.TotalBoards,
			HealthyBoards: r.HealthyBoards,
			CooledBoards:  r.CooledBoards,
			LastRun:       isoOrNil(r.LastRunAt),
			LastSuccess:   isoOrNil(r.LastSuccessAt),
			IngestedTotal: r.IngestedTotal,
		}
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"overall":           fleetStatus(rolls, now),
			"generated_at":      now.Format(time.RFC3339),
			"last_job_added_at": isoOrNil(lastJobAddedAt),
			"providers":         providers,
		},
	})
}

// tsTime unwraps a nullable timestamp to a time.Time, using the zero value for
// NULL so deriveStatus reads it as "never".
func tsTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

// isoOrNil renders a nullable timestamp as an RFC 3339 UTC string, or nil for
// NULL so the wire field is `null` rather than a zero date.
func isoOrNil(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}
