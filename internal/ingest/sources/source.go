// Package sources holds the modular job-source adapters and the registry that maps
// a platform key to its adapter. Each adapter implements one ATS platform; adding a
// platform is a new file plus one line in All.
package sources

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// DefaultSweepGrace is the grace window a provider is swept on when it declares no
// sweepGrace of its own: many crawl cycles at the hourly per-provider cadence, so a board
// failing several runs in a row keeps its jobs open. cmd/ingest's own unseen sweep and
// cmd/liveness's probeDespiteRegistered backstop (which only picks up what that sweep has
// already had a chance to close) both anchor on this single symbol so the two windows
// cannot drift apart silently.
const DefaultSweepGrace = 48 * time.Hour

// CompanyEntry is one configured board from a board file (sources/<provider>.yml): the
// company whose jobs we crawl, the platform it uses (Provider), and the platform-specific
// board id. Region is an optional per-entry hint for ATS platforms that host tenants on
// regional API domains (e.g. Lever's EU data-residency host); empty means the default host.
// Hub is an optional per-entry flag marking a board as a community/agency hub whose vacancies
// belong to many partner companies; an adapter that honours it resolves each job's employer from
// the posting and uses Company only as the hub name and per-vacancy fallback (e.g. huntflow's
// AlumniHub). It is ignored by adapters that do not implement hub resolution.
// Tenants is the companion map for a hub whose postings identify their tenant only by an opaque
// key — a URL path segment or an id — that is not itself a usable company name; it maps that key
// to the employer's display name (e.g. successfactors' "Arvato_Systems" → "Arvato Systems").
// A key absent from the map falls back to Company rather than being turned into a name by
// transforming the key, because a plausible-but-wrong employer reads worse in the catalogue than
// the parent brand. Optional and adapter-specific, exactly as Hub and Region are.
type CompanyEntry struct {
	Company  string            `yaml:"company"`
	Provider string            `yaml:"provider"`
	Board    string            `yaml:"board"`
	Region   string            `yaml:"region"`
	Hub      bool              `yaml:"hub"`
	Tenants  map[string]string `yaml:"tenants"`
}

// Job is a raw posting as an adapter yields it, before the pipeline normalizes it
// into the catalogue. ExternalID carries the platform's native posting id; the
// pipeline namespaces it by board before persisting.
type Job struct {
	ExternalID  string
	URL         string
	Title       string
	Company     string
	Location    string
	Description string
	Remote      bool
	PostedAt    *time.Time
	// WorkMode is the work arrangement when the platform states it in a STRUCTURED
	// field (a workplace-type enum or an explicit remote flag) — "remote",
	// "hybrid", or "onsite", else "". It is left empty for adapters that only
	// expose free-text location; the pipeline then falls back to parsing the
	// location string. Provenance stays clean: this carries structured signal only,
	// never the location heuristic.
	WorkMode string
	// Seniority, Category, EmploymentType, Skills, and ExperienceYearsMin are the
	// platform's STRUCTURED facet signals, already mapped into freehire's controlled
	// vocabularies (vocab.SeniorityValues / vocab.CategoryValues /
	// vocab.EmploymentTypeValues / canonical skill names). They mirror WorkMode: an
	// adapter sets them only when the platform states the value in a structured field
	// (e.g. an ATS timeType / typeOfEmployment enum), never a heuristic inferred from
	// free text, and leaves them empty/nil otherwise so the pipeline's dictionaries
	// decide. The pipeline gives a set value precedence over the dictionary (Skills are unioned).
	Seniority          string
	Category           string
	EmploymentType     string
	Skills             []string
	ExperienceYearsMin *int
	// Countries mirrors the same contract for geography: an adapter sets it only when
	// the platform states the country in a STRUCTURED field (not the free-text location
	// the description/location string carries), normalized through
	// internal/dict/location.NormalizeCountry into freehire's canonical lowercase alpha-2
	// codes. Left nil for adapters that expose only free-text location, so the
	// pipeline's location dictionary derives it instead — the same fallback WorkMode
	// and the other structured facets already follow.
	Countries []string
	// SalaryMin/SalaryMax/SalaryCurrency/SalaryPeriod mirror the same structured-only
	// contract for compensation: an adapter sets them only when the platform states a
	// salary in its own STRUCTURED field (e.g. Lever's salaryRange, Ashby's
	// compensationTiers, Recruitee's salary), currency as ISO 4217 and period as one of
	// vocab.SalaryPeriodValues ("year"/"month"/"day"/"hour"), never inferred from the
	// description. Left nil/empty otherwise, so the enrichment pass's own LLM-derived
	// guess decides — the pipeline gives a set value precedence over that guess.
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
	// Removed marks a posting the source reports as taken down (e.g. an item flagged
	// removed in JobStream's incremental feed). A streaming, self-closing source emits
	// these so the pipeline closes the job by identity instead of upserting it; all other
	// adapters leave it false and only ever emit live postings.
	Removed bool
	// SeenRefresh marks a posting a HydratingSource re-listed but did NOT fetch fresh
	// content for (it was already ingested, so detail is skipped). The pipeline refreshes
	// the row's liveness (last_seen_at, reopen) by identity WITHOUT rewriting its content,
	// so the description and facets hydrated when it was new are preserved — a content-less
	// re-upsert would re-derive the facets from an empty description and wipe them. Only a
	// HydratingSource sets it (carrying just Title/Company/URL/ExternalID for the identity);
	// all other adapters leave it false. Mutually exclusive with Removed.
	SeenRefresh bool
	// ApplyForm is the application form the platform published for this posting, set ONLY
	// by an adapter whose list endpoint already carries one. It is nil for every other
	// adapter, and nil is not a failure — most platforms do not describe their form in a
	// listing, and the two that describe it per posting are captured after ingest by
	// cmd/capture-apply-form rather than here.
	//
	// An adapter must not issue an extra request to fill this. That is the whole point of
	// the split: a form that costs a request would make a crawl's duration a function of
	// board size, and the adapter cannot tell which postings are new anyway — that answer
	// only exists after the upsert's ON CONFLICT resolves.
	ApplyForm *applyform.Form
}

// Source adapts one job-source platform. Provider is the platform key that selects
// the adapter (it matches CompanyEntry.Provider and the stored jobs.source); Fetch
// returns all current postings for one configured board.
type Source interface {
	Provider() string
	Fetch(ctx context.Context, e CompanyEntry) ([]Job, error)
}

// StreamingSource is a Source that can also stream its postings to a sink as it crawls, so the
// pipeline persists them incrementally rather than buffering the whole board until Fetch
// returns. An adapter with an expensive per-posting detail fan-out (eightfold, whose large
// catalogues take many minutes under the source's rate limit) implements it so a long crawl's
// progress is saved as it goes — partial work survives an interrupted or rate-limited run, and
// the catalogue converges across runs. emit is called once per ready posting and may be called
// concurrently. FetchStream returns an error only for a board-level failure (e.g. the listing
// failed); a single dropped posting is simply not emitted.
type StreamingSource interface {
	Source
	FetchStream(ctx context.Context, e CompanyEntry, emit func(Job)) error
}

// HydratingSource is a Source that fetches expensive per-posting detail (e.g. the description
// the list omits) only for postings the catalogue does not already have. The pipeline supplies a
// seen predicate — seen(externalID) reports whether that posting is already ingested for the
// provider — so a large aggregator (justjoin, ~20k live offers) issues detail requests only for
// new postings instead of on every crawl. An adapter opts in by implementing this in addition to
// Fetch (the list-only fallback used when the pipeline cannot supply a seen set); every other
// adapter is unaffected. The pipeline prefers FetchNew when the adapter implements it.
type HydratingSource interface {
	Source
	FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error)
}

// CoverageGated is a HydratingSource that can also skip a posting's detail when the pipeline's
// aggregator-coverage gate is going to discard that posting anyway — the employer is already
// covered by a non-aggregator source.
//
// It exists because `seen` alone cannot bound the work on such a board. A discarded posting is
// never stored, so it is never seen, so the ordinary hydrating crawl pays for its body on
// EVERY run and throws the body away every time. That is a rounding error on most aggregators
// (measured on prod across the twelve hydrating ones: 0-3% for all but three) and it is the
// whole board on one: remote.com lists ~5.7k postings of which ~4.1k are already covered, so
// 71% of its hydration budget bought nothing, hourly, forever.
//
// covered is called ONCE with every company the crawl listed and answers which of them are
// already covered, keyed by the SAME strings passed in — the adapter states company names and
// never has to know how freehire slugs them. The pipeline passes a resolver only when the gate
// actually applies to this board, so an adapter that gets here can trust the answer.
//
// The contract is about COST, not about the result: a covered posting is still yielded, just
// without a body. Dropping it instead would be the adapter quietly making the gate's decision,
// which is the pipeline's to make and to count (Stats.ATSCovered).
type CoverageGated interface {
	HydratingSource
	FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool,
		covered func(companies []string) map[string]bool) ([]Job, error)
}

// boardless marks an adapter whose API has no per-tenant board id, so config
// validation lets its entries omit board. A boardless adapter may serve one company
// (greenhouse/lever and the other multi-tenant ATS adapters are NOT boardless and
// still require a board) or aggregate many (see aggregator).
type boardless interface{ boardless() }

// aggregator marks a boardless adapter that aggregates postings from many companies
// (e.g. jobstash) rather than serving a single company. It keeps such an adapter in
// the source facet: a single-company boardless platform is redundant with the company
// filter and excluded, but filtering by an aggregator is not.
type aggregator interface{ aggregator() }

// selfClosing marks an adapter that closes its own removed postings (via a Job with
// Removed set, emitted from its stream) and therefore must be excluded from the post-run
// unseen sweep. Such a source re-reports only changed postings each run, so the sweep's
// last_seen_at cutoff would wrongly close every still-open posting it did not touch; the
// stream's removal events are the authoritative close signal instead. See SelfClosingProviders.
type selfClosing interface{ selfClosing() }

// fullCatalog marks an aggregator whose every crawl lists the WHOLE catalogue in one run —
// not a per-company board and not a subset. For such a source an open job the run did not
// see is genuinely gone, so the post-run sweep may close it by source alone, dropping the
// crawled-company scope that would otherwise leak the postings of a company that vanished
// from the feed entirely (see cmd/ingest's sweep). The marker is only sound when the adapter
// FAILS a truncated crawl (returns an error, not a partial success): a silently-truncated
// run looks like a shrunken catalogue and a source-scoped close would retire everything it
// never reached. cmd/ingest gates the source-scoped close on a zero-Failed run for exactly
// this reason. See FullCatalogProviders.
type fullCatalog interface{ fullCatalog() }

// sweepGrace marks an adapter that needs the post-run unseen sweep to wait longer than the
// default before closing its jobs, and reports how long. It is the opposite case to fullCatalog:
// the crawl deliberately reaches only a SLICE of the source's catalogue (a keyword's first N
// pages of a feed far deeper than any crawl can walk), so a posting that merely drifted past
// that depth reads as unseen. On the default window it would be closed and then reopened when it
// drifts back — churn that also lands in job_daily_stats as a phantom removal. A window wider
// than the drift absorbs it, at the cost of a genuinely withdrawn posting lingering that long.
// The marker is only sound for a source whose postings CANNOT be probed for liveness; anything
// verifiable should be closed on evidence instead. See SweepGraceWindows.
type sweepGrace interface{ sweepGrace() time.Duration }
