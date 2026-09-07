package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/observability"
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

// Site-status derivation thresholds and window. Mirrors the provider
// thresholds above: a minimum-traffic guard so a couple of unlucky requests
// right after a deploy can't read as an outage, then two error-fraction
// thresholds over the in-process rolling window (see
// internal/platform/observability.ErrorRate).
const (
	// siteErrorWindow is the trailing duration ErrorRate is computed over.
	siteErrorWindow = 10 * time.Minute
	// minSiteRequestsForSignal: below this many requests in the window, the
	// error fraction is not trusted — too little data to distinguish a real
	// problem from a couple of unlucky requests.
	minSiteRequestsForSignal = 20
	// siteDegradedErrorRate: above this fraction (and below siteDownErrorRate)
	// reads degraded.
	siteDegradedErrorRate = 0.02
	// siteDownErrorRate: at or above this fraction reads down even though the
	// database itself answers.
	siteDownErrorRate = 0.5
)

// deriveSiteStatus maps the site's own live signals to its status:
//   - down    when the database is unreachable, regardless of error rate;
//   - operational when the database is up and there isn't enough traffic in
//     the window to trust the error fraction;
//   - down    when the database is up but the error fraction is at or above
//     siteDownErrorRate;
//   - degraded when the error fraction exceeds siteDegradedErrorRate;
//   - operational otherwise.
func deriveSiteStatus(dbUp bool, errorRate float64, totalRequests int64) providerStatus {
	if !dbUp {
		return statusDown
	}
	if totalRequests < minSiteRequestsForSignal {
		return statusOperational
	}
	switch {
	case errorRate >= siteDownErrorRate:
		return statusDown
	case errorRate > siteDegradedErrorRate:
		return statusDegraded
	default:
		return statusOperational
	}
}

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

// siteHealth is the public, sanitized verdict for the site/API itself —
// independent of the ingest fleet's own status (`overall`). database is
// "up" or "down", the wire encoding health.go's Health handler already
// uses. error_rate is the fraction of 5xx responses over the trailing
// window_minutes, computed from the process's own recent traffic (see
// internal/platform/observability.ErrorRate) — never from an external
// Prometheus query.
type siteHealth struct {
	Status        providerStatus `json:"status"`
	Database      string         `json:"database"`
	ErrorRate     float64        `json:"error_rate"`
	WindowMinutes int            `json:"window_minutes"`
}

// IngestStatus serves the public, unauthenticated status read: the site/API's
// own live status (siteHealth), plus a per-provider health rollup over
// board_health with a derived operational/degraded/down status per provider
// and an overall fleet status, plus last_job_added_at — the created_at of the
// most recently added open, public job, a live signal that the pipeline is
// actually writing rows (independent of per-provider crawl health).
// Sanitized by construction — the DTO carries no error text or board
// identifier — so no internal detail can leak. An empty fleet yields overall
// "operational" with no providers and a null last_job_added_at.
//
// The database is checked FIRST and the ingest-fleet queries are skipped
// entirely when it fails: they would only fail the same way (the fleet
// rollup and the site check share the one pool), and reporting a 500 with no
// body in exactly the outage this endpoint exists to surface would defeat
// the point of adding a site-status section at all. That short-circuit
// reports overall "operational" with no providers — the same "no data"
// value an empty rollup already gets — rather than "down": a database
// outage means the fleet's health is UNKNOWN, not that it has failed, and
// only site.status should carry the database's own honest "down" verdict.
func (h *statsHandlers) IngestStatus(c *fiber.Ctx) error {
	now := time.Now().UTC()
	errorRate, totalRequests := observability.ErrorRate(siteErrorWindow)
	dbUp := h.pool.Ping(c.Context()) == nil
	site := siteHealth{
		Status:        deriveSiteStatus(dbUp, errorRate, totalRequests),
		Database:      dbStatusLabel(dbUp),
		ErrorRate:     errorRate,
		WindowMinutes: int(siteErrorWindow / time.Minute),
	}
	if !dbUp {
		// overall reads "operational" here — the same "no data" convention
		// fleetStatus already uses for a genuinely empty rollup (see its own
		// comment) — rather than "down": a database hiccup means the ingest
		// fleet's health is UNKNOWN for this request, not that it failed. site
		// carries the honest "down" verdict for the database itself; conflating
		// the two would report a fleet outage that may not exist.
		return statusResponse(c, now, statusOperational, nil, []statusProvider{}, site)
	}

	rows, err := h.queries.ProviderHealthRollup(c.Context())
	if err != nil {
		return err
	}
	lastJobAddedAt, err := h.queries.LatestOpenJobAddedAt(c.Context())
	if err != nil {
		return err
	}

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

	return statusResponse(c, now, fleetStatus(rolls, now), isoOrNil(lastJobAddedAt), providers, site)
}

// statusResponse renders the /api/v1/status envelope shared by the healthy
// path and the database-down short-circuit above, so the two agree on shape
// by construction rather than by two hand-kept copies of the same map.
func statusResponse(c *fiber.Ctx, now time.Time, overall providerStatus, lastJobAddedAt *string, providers []statusProvider, site siteHealth) error {
	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"overall":           overall,
			"generated_at":      now.Format(time.RFC3339),
			"last_job_added_at": lastJobAddedAt,
			"providers":         providers,
			"site":              site,
		},
	})
}

// dbStatusLabel renders a database-availability bool as the wire string
// health.go's Health handler already uses ("up"/"down"), so the two
// endpoints agree on vocabulary.
func dbStatusLabel(up bool) string {
	if up {
		return "up"
	}
	return "down"
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
