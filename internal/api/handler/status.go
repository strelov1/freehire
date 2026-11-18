package handler

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
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

// Status-derivation thresholds. The healthy fraction keeps a handful of failing boards from
// degrading a big provider; success freshness catches a stalled crawl whose boards all read healthy.
const (
	// healthyOperationalFrac: at or above this fraction healthy (and fresh) is green.
	healthyOperationalFrac = 0.9
	// healthyDownFrac: at or below this fraction healthy is red regardless of freshness.
	healthyDownFrac = 0.1
	// successFreshness: a provider with no success within this window reads down.
	successFreshness = 48 * time.Hour
)

// Site-status thresholds over the in-process rolling window (observability.ErrorRate), with a
// minimum-traffic guard so a few unlucky requests after a deploy can't read as an outage.
const (
	// siteErrorWindow is the trailing duration ErrorRate is computed over.
	siteErrorWindow = 10 * time.Minute
	// minSiteRequestsForSignal: below this many requests the error fraction is not trusted.
	minSiteRequestsForSignal = 20
	// siteDegradedErrorRate: above this fraction (and below siteDownErrorRate)
	// reads degraded.
	siteDegradedErrorRate = 0.02
	// siteDownErrorRate: at or above this fraction reads down even though the
	// database itself answers.
	siteDownErrorRate = 0.5
)

// poolPressure is the fraction of the connection pool held at once. A pool with no capacity
// (unconfigured, or closed) yields 0 rather than dividing by zero.
//
// Reported, never judged: a single instant cannot carry a verdict, since a healthy bursty
// site touches the ceiling routinely. The sampler keeps each day's WORST reading, so feeding
// this into deriveSiteStatus would paint whole days degraded. The Grafana rule that averages
// it over five minutes makes the verdict.
func poolPressure(acquired, maxConns int32) float64 {
	if maxConns <= 0 {
		return 0
	}
	return float64(acquired) / float64(maxConns)
}

// deriveSiteStatus maps the site's own live signals to its status:
//   - down    when the database is unreachable, regardless of every other signal;
//   - down    when the database is up but the error fraction is at or above
//     siteDownErrorRate;
//   - degraded when the error fraction exceeds siteDegradedErrorRate;
//   - operational otherwise.
//
// Pool occupancy is deliberately NOT an input; see poolPressure.
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

// severityOrder is the source of truth for the severity stored in site_status_daily: the
// slice index IS the severity, in the frontend's SEVERITY order (operational < degraded < down).
var severityOrder = []providerStatus{statusOperational, statusDegraded, statusDown}

// severityFromStatus maps a providerStatus to its stored severity.
func severityFromStatus(s providerStatus) int16 {
	for i, v := range severityOrder {
		if v == s {
			return int16(i)
		}
	}
	return 0
}

// severityToStatus is the inverse of severityFromStatus. An out-of-range value reads as
// statusDown, not statusOperational: unrecognized should read alarming.
func severityToStatus(sev int16) providerStatus {
	if sev < 0 || int(sev) >= len(severityOrder) {
		return statusDown
	}
	return severityOrder[sev]
}

// providerRollup keeps status policy independent of DB rows. Healthy boards are outside
// active cooldown; freshnessAt includes legitimate quiet pauses, while lastSuccess is observed.
type providerRollup struct {
	total       int64
	healthy     int64
	freshnessAt time.Time
}

// deriveStatus maps a provider's rollup to its status at instant now:
//   - down    when it has no boards, no fresh success, or ≤10% healthy;
//   - operational when ≥90% healthy and fresh;
//   - degraded  otherwise.
func deriveStatus(r providerRollup, now time.Time) providerStatus {
	if r.total <= 0 {
		return statusDown
	}
	fresh := !r.freshnessAt.IsZero() && now.Sub(r.freshnessAt) <= successFreshness
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

// fleetStatus folds every provider into one rollup (served fraction across ALL boards, freshest
// success) rather than taking the worst provider, so a few small blocked providers can't red a
// healthy fleet while a broad outage or stall still surfaces. An empty fleet is operational.
func fleetStatus(rolls []providerRollup, now time.Time) providerStatus {
	if len(rolls) == 0 {
		return statusOperational
	}
	var fleet providerRollup
	for _, r := range rolls {
		fleet.total += r.total
		fleet.healthy += r.healthy
		if r.freshnessAt.After(fleet.freshnessAt) {
			fleet.freshnessAt = r.freshnessAt
		}
	}
	return deriveStatus(fleet, now)
}

// siteHistoryFromRows converts the trailing-90-days read (ascending by day) to the wire
// shape. Always non-nil so an empty history serializes as `[]`, never `null`.
func siteHistoryFromRows(rows []db.SiteStatusHistoryRow) []apigen.SiteHistoryEntry {
	entries := make([]apigen.SiteHistoryEntry, len(rows))
	for i, r := range rows {
		entries[i] = apigen.SiteHistoryEntry{
			Day:    r.Day.Time.Format(dateLayout),
			Status: apigen.HealthStatus(severityToStatus(r.WorstSeverity)),
		}
	}
	return entries
}

// currentSiteHealth computes the site's live status for both the HTTP handler and the
// daily-history sampler in cmd/server, so they agree on what "current status" means. History
// is NOT populated here: the caller decides whether that query is worth running.
//
// A plain function taking the pool, not a statsHandlers method: the sampler has no
// statsHandlers.
func currentSiteHealth(ctx context.Context, pool *pgxpool.Pool) (health apigen.SiteHealth, dbUp bool) {
	errorRate, totalRequests := observability.ErrorRate(siteErrorWindow)
	dbUp = pool.Ping(ctx) == nil

	// Stat reads in-memory pool counters: no query, no round trip.
	stat := pool.Stat()
	pressure := poolPressure(stat.AcquiredConns(), stat.MaxConns())

	return apigen.SiteHealth{
		Status:        apigen.HealthStatus(deriveSiteStatus(dbUp, errorRate, totalRequests)),
		Database:      apigen.SiteHealthDatabase(dbStatusLabel(dbUp)),
		ErrorRate:     errorRate,
		WindowMinutes: int64(siteErrorWindow / time.Minute),
		PoolPressure:  pressure,
	}, dbUp
}

// IngestStatus serves the public status read: the site's live status, a per-provider health
// rollup over board_health with an overall fleet status, and last_job_added_at (created_at of
// the newest open public job, a signal that the pipeline is writing rows). The DTO carries no
// error text or board identifier. An empty fleet yields "operational", no providers and a
// null last_job_added_at.
//
// The database is checked FIRST and the fleet queries are skipped when it fails: they share
// the pool and would fail too, and a bodyless 500 in the very outage this endpoint exists to
// show would defeat it. That path reports overall "operational" (fleet health is UNKNOWN, not
// failed); only site.status carries the database's "down".
func (h *statsHandlers) IngestStatus(ctx context.Context) (*apigen.IngestStatusOK, error) {
	now := time.Now().UTC()
	site, dbUp := currentSiteHealth(ctx, h.pool)
	if !dbUp {
		// History can't be read either; siteHistoryFromRows(nil) keeps the "[] never null" invariant.
		site.History = siteHistoryFromRows(nil)
		return statusResponse(now, statusOperational, apigen.NilString{Null: true}, []apigen.StatusProvider{}, site), nil
	}

	historyRows, err := h.queries.SiteStatusHistory(ctx)
	if err != nil {
		return nil, err
	}
	site.History = siteHistoryFromRows(historyRows)

	rows, err := h.queries.ProviderHealthRollup(ctx)
	if err != nil {
		return nil, err
	}
	rows = foldHealthFamilies(rows)
	lastJobAddedAt, err := h.queries.LatestOpenJobAddedAt(ctx)
	if err != nil {
		return nil, err
	}

	// Taxonomy classifies each provider by adapter kind; it is not a crawl registry (no ingest credentials here).
	reg := sources.Taxonomy()
	providers := make([]apigen.StatusProvider, len(rows))
	rolls := make([]providerRollup, len(rows))
	for i, r := range rows {
		rolls[i] = providerRollup{
			total:       r.TotalBoards,
			healthy:     r.HealthyBoards,
			freshnessAt: tsTime(r.FreshnessAt),
		}
		providers[i] = apigen.StatusProvider{
			Provider:      r.Provider,
			Kind:          apigen.ProviderKind(sources.ProviderKind(reg, r.Provider)),
			Status:        apigen.HealthStatus(deriveStatus(rolls[i], now)),
			TotalBoards:   r.TotalBoards,
			HealthyBoards: r.HealthyBoards,
			CooledBoards:  r.CooledBoards,
			LastRun:       isoOrNil(r.LastRunAt),
			LastSuccess:   isoOrNil(r.LastSuccessAt),
			IngestedTotal: r.IngestedTotal,
		}
	}

	return statusResponse(now, fleetStatus(rolls, now), isoOrNil(lastJobAddedAt), providers, site), nil
}

// statusResponse builds the /api/v1/status answer for both the healthy and database-down paths.
func statusResponse(now time.Time, overall providerStatus, lastJobAddedAt apigen.NilString, providers []apigen.StatusProvider, site apigen.SiteHealth) *apigen.IngestStatusOK {
	return &apigen.IngestStatusOK{Data: apigen.IngestStatus{
		Overall:        apigen.HealthStatus(overall),
		GeneratedAt:    now.Format(time.RFC3339),
		LastJobAddedAt: lastJobAddedAt,
		Providers:      providers,
		Site:           site,
	}}
}

// dbStatusLabel renders database availability as the "up"/"down" wire string health.go uses.
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

// isoOrNil renders a nullable timestamp as an RFC 3339 UTC string, or null for
// NULL so the wire field is `null` rather than a zero date.
func isoOrNil(ts pgtype.Timestamptz) apigen.NilString {
	if !ts.Valid {
		return apigen.NilString{Null: true}
	}
	return apigen.NewNilString(ts.Time.UTC().Format(time.RFC3339))
}
