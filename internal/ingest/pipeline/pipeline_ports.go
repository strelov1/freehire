package pipeline

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/job"
)

// defaultConcurrency bounds how many boards are fetched at once.
const defaultConcurrency = 8

// progressInterval is how often the run emits a heartbeat with the count of boards
// crawled so far, so a stalled board (one whose fetch hangs) is visible — the count
// stops advancing — instead of the run going silent until it finishes.
const progressInterval = 60 * time.Second

// Store persists one Job aggregate and enqueues it for enrichment when needed,
// atomically. It accepts only a job.Job — a value obtainable exclusively through
// the aggregate factory (job.New) — so the write path cannot persist a posting that
// bypassed the deterministic derivation. The pipeline is unaware of the schema
// version or the outbox — that is the Store implementation's concern.
type Store interface {
	Save(ctx context.Context, j job.Job) error
}

// FormSaver is the optional Store capability for a posting whose adapter also yielded the
// platform's application form: write the job and the form together, so a captured form
// cannot be lost to a failure between the two.
//
// It exists as a capability rather than a wider Save signature because exactly one
// provider's listing carries a form, and every other Store — including every test fake —
// would otherwise take a parameter it has nothing to do with. Same discovery rule as
// Closer and Toucher: the pipeline type-asserts, and a Store that lacks it still ingests
// the posting through the plain Save. The form is then simply not captured, which is the
// right degradation — a crawl must not fail over a field nothing yet reads.
type FormSaver interface {
	SaveWithApplyForm(ctx context.Context, j job.Job, form applyform.Form) error
}

// Closer is the optional Store capability a self-closing streaming source needs: closing a
// posting by its (source, external_id) identity when the feed reports it removed. Only the
// ingest dbStore implements it; ingestStream type-asserts for it and skips removals when a
// Store lacks it, so other Store implementations (and test fakes) are unaffected.
type Closer interface {
	Close(ctx context.Context, source, externalID string) error
}

// Toucher is the optional Store capability a HydratingSource needs: refresh a posting's liveness
// (last_seen_at, reopen if closed) by its (source, external_id) identity, WITHOUT rewriting its
// content. The pipeline uses it for a posting the adapter re-listed but did not re-fetch (its
// stored content is already current); a full upsert of the content-less listing would wipe the
// hydrated description/facets. Only the ingest dbStore implements it; the pipeline type-asserts
// for it and drops the refresh when a Store lacks it (test fakes), like Closer.
type Toucher interface {
	// Touch reports the refreshed row's stored company slug ("" when there was no row).
	Touch(ctx context.Context, source, externalID string) (companySlug string, err error)
}

// CoveredRefresher refreshes the liveness of OPEN stored rows a board listed but did
// not write, either because of the aggregator-coverage gate or an unreadable detail.
// Missing and closed rows are no-ops: only successfully hydrated content may reopen
// an unreadable posting. It reports refreshed rows without counting another posting;
// failures withhold the board's close scopes through Stats.RefreshFailed. Return one
// stored company slug per refreshed row so an unproven board can withhold the actual
// employers too, even when an unreadable marker names the configured board owner.
type CoveredRefresher interface {
	RefreshCovered(ctx context.Context, source string, externalIDs []string) ([]string, error)
}

// CoveredCloser closes the OPEN stored rows an aggregator board listed for an employer the
// coverage gate found already crawled first-hand, so the aggregator copy stops showing beside
// the employer's own postings. A lapsed coverage lets the next crawl write the posting again,
// which reopens it. A Store lacking it refreshes those rows instead (CoveredRefresher). Return
// one stored company slug per closed row.
type CoveredCloser interface {
	CloseCovered(ctx context.Context, source string, externalIDs []string) ([]string, error)
}

// SeenLookup is the optional Store capability a HydratingSource needs: the external_ids already
// stored for the board about to be crawled, so the adapter fetches expensive per-posting detail
// only for postings the catalogue lacks. Only the ingest dbStore implements it; the runner
// type-asserts for it and falls back to the list-only Fetch when a Store lacks it (test fakes,
// non-DB callers), so other Store implementations are unaffected.
//
// The lookup is board-scoped because it runs once per crawled board: reading the provider's whole
// catalogue for each of its boards is affordable only for a boardless adapter, which passes an
// empty board and gets the provider-wide set.
//
// Each id maps to whether its stored row carries tech evidence (is_tech). Membership drives the
// hydration decision; the flag lets a liveness refresh face the catalogue filter on the same
// evidence a write would, which a content-less listing cannot supply.
//
// A stored row whose detail fetch failed — no description — is withheld from the set for
// HydrationRetryWindow, so the crawl retries the fetch instead of treating the half-ingested
// posting as done.
type SeenLookup interface {
	ExistingExternalIDs(ctx context.Context, source, board string) (map[string]bool, error)
}

// StoredBodyLookup is the optional Store capability behind an empty posting body: the body the
// catalogue already holds for that identity ("" when none). A detail fetch that fails yields
// an empty description, and UpsertJob keeps the stored one — so deriving the facets, the
// content hash and the role fingerprint from the empty text would write a row whose facets
// describe no body at all (measured on paylocity 2026-11-04: 36 of 42 reopened rows had lost
// their skills, posting language or employment type that way).
type StoredBodyLookup interface {
	StoredDescription(ctx context.Context, source, externalID string) (string, error)
}

// CompanyWebsiteRecorder is the optional Store capability behind sources.Job.CompanyWebsite:
// record the employer's own website on its company row when none is known yet. It is a
// capability for the same reason CompanyDescriptionFiller is: only the ingest dbStore has a
// companies table, and a Store without it still ingests the posting. The website is then
// not recorded, which is the right degradation for a field no write depends on.
type CompanyWebsiteRecorder interface {
	RecordCompanyWebsite(ctx context.Context, companySlug, website string) error
}

// CompanyDescriptionFiller is the optional Store capability a sources.CompanyDescriber
// adapter needs: apply a company-level description to that company's tagline/
// company_info gap. It exists as a capability because only the ingest dbStore has a
// company_info table to write to — a test fake or a future non-Postgres Store simply
// skips the write, exactly like FormSaver/Closer/Toucher. slug is the board's company,
// already normalized (normalize.CompanySlug), so the Store implementation matches the
// same key every other company-info source writes under; name is the board's own
// display name, needed only when the slug has no existing row yet to insert one.
//
// CompanyDescriptionDue is asked BEFORE the adapter is: a company is due when it holds no
// description yet AND no fetch has been recorded for it since checkedSince. A company that
// already holds one is never due — the fill keeps whatever is stored (existing keys win),
// so re-asking could not change it. RecordCompanyDescription records an ANSWERED fetch,
// blank included: it always stamps the attempt and fills the gap only when description is
// non-empty, so a board whose employer never wrote an "about us" is asked once per window
// rather than once per run.
type CompanyDescriptionFiller interface {
	CompanyDescriptionDue(ctx context.Context, slug string, checkedSince time.Time) (bool, error)
	RecordCompanyDescription(ctx context.Context, slug, name, description string) error
}

// CompanyDescriptionRefreshWindow is how long an answered company-description fetch that
// found nothing stays answered. Before the window existed the description was fetched on
// every crawl of every board: measured on prod 2026-10-19 that was 3,963 workable boards ×
// 8 runs a day, half of Workable's ~63k requests a day and half of its 18,520 refusals in
// two days — for a field that, once filled, the fill can never change. A week bounds the
// cost of a blank one to one request per board per week while still noticing an employer
// who fills it in.
const CompanyDescriptionRefreshWindow = 7 * 24 * time.Hour

// HydrationRetryWindow is how long after a posting enters the catalogue its missing description
// is still worth another detail request. Being stored is what makes a posting "seen", so without
// a window a failed first hydration is permanent: the row keeps its empty body for life (that is
// how ~3.3k live rows across the hydrating sources lost theirs, freehire#1866). The window is
// bounded rather than open-ended because the other reason a body is missing is that the source
// published none, and retrying those forever would spend a detail request per crawl on every one
// of them. Two weeks covers a transient outage or a rate-limit stretch without funding a
// permanent retry loop.
const HydrationRetryWindow = 14 * 24 * time.Hour

// DeadlineRecheckWindow is how long after a stored application deadline passes the posting's
// detail is still worth re-reading. The deadline lives only on the detail, so a re-listed
// posting would otherwise close on the date stored when it was first seen, even after the
// employer extended it. Two weeks covers a late extension without paying a detail request on
// every crawl for each expired posting a source keeps listing.
const DeadlineRecheckWindow = 14 * 24 * time.Hour

// BoardHealth is the optional per-board health port: it tells the Runner whether a
// board is currently cooled down (skip it) and records each crawl's outcome so a
// repeatedly-failing board backs off. A nil BoardHealth disables the feature entirely
// (the Runner behaves exactly as before), so unit tests and non-DB callers are
// unaffected — the same shape as the optional Closer.
type BoardHealth interface {
	// Pauses reports the board's two independent pauses — the failure cooldown and the quiet
	// pause (QuietFor) — in one lookup. The Runner skips the board while either is in the future.
	Pauses(ctx context.Context, provider, board, region string) (BoardPauses, error)
	// RecordSuccess clears the board's failure state and stamps freshness. reached reports
	// whether the board's LISTING named any posting at all (boardListedAnyPosting) — a
	// separate question from ingested, which counts only the ones that became rows, and the
	// one an empty feed answers differently from a live board whose every posting we already
	// hold, or cannot read, or failed to save. See board_health.last_yield_at (migration
	// 0158) for what it feeds, and boardListedAnyPosting for why it is deliberately the most
	// generous reading available. fingerprint is the board's listingFingerprint from a COMPLETE
	// crawl, or "" when this crawl cannot vouch for the whole listing, which leaves the quiet
	// state untouched (migration 0198).
	RecordSuccess(ctx context.Context, provider, board, region string, ingested int, reached bool, fingerprint string) error
	// RecordFailure counts a failed crawl and cools the board down per the backoff policy.
	RecordFailure(ctx context.Context, provider, board, region string, cause error) error
	// CooledBoards returns up to limit (board, region) pairs of the provider currently in
	// an active cooldown — the recovery probe's candidates.
	CooledBoards(ctx context.Context, provider string, limit int) ([]CooledBoard, error)
	// ClearCooldowns clears the active cooldown of every currently-cooled board of the
	// provider and returns how many were cleared. Called once a probe proves the provider
	// reachable again.
	ClearCooldowns(ctx context.Context, provider string) (int, error)
}

// CoverageLookup is the optional Store-adjacent port the ingest-time aggregator coverage gate
// needs: for a batch of company slugs, which are already covered by an OPEN posting from a
// non-aggregator source. Optional: nil disables the gate entirely (ATS board files, test
// fakes) — the same shape as BoardHealth.
//
// companySlugs are company_slug values as normalize.CompanySlug produced them AND the alias
// registry resolved them — the same value the upsert will store. The returned map's keys are
// expected back in exactly that form — a lookup may consult other SPELLINGS of
// a slug internally, but that is its own business and never leaks into the answer.
//
// COVERAGE MEANS CURRENT COVERAGE. An implementation MUST only count a posting the catalogue
// has seen recently, and owns how recent that is. The gate's yes is unrecoverable — the
// aggregator's posting is never written, so it is never in the database, in /find, or in
// search, and it leaves no trace anyone can query afterwards. A no merely costs a duplicate
// row, which aggregator-ats-dedup's periodic pass already marks on its own schedule. So a
// coverage claim has to be about the present tense ("we still crawl this employer"), not the
// past: without a freshness bound one forgotten row holds its slug forever, which is how a
// 2013 posting from a board that had left sources/ suppressed every live posting for a
// different employer of the same name (issue #2315).
//
// That bound is also why the port is Postgres-backed rather than search-backed. last_seen_at
// cannot reach the search index: the incremental drain pushes a document only when its
// content_hash moves, and the write that stamps the column on the common path
// (RefreshUnchangedJob) deliberately writes nothing else and enqueues nothing — so an indexed
// copy would be most wrong for exactly the actively-crawled rows the gate must credit.
type CoverageLookup interface {
	NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string) (map[string]bool, error)
}

// aggregatorCoverage answers whether a company (keyed by the company_slug normalize.Slug
// produced, whatever spelling the lookup matched) is already covered by a non-aggregator source, for the
// board currently being ingested. nil means the gate does not apply — saveOne treats a nil
// resolver as "never covered". Two callers build one: the buffered path (ingestFetched)
// resolves the whole board's companies in one batched CoverageLookup call up front; the
// streaming path (ingestStream) has no such batch, so it resolves — and memoizes — one
// company at a time as postings are emitted.
type aggregatorCoverage func(companySlug string) bool

// BoardPauses is a board's two pauses; a zero time means that pause is not set. Cooldown
// backs off a FAILING board (CooldownFor), Quiet a board whose listing keeps coming back
// unchanged (QuietFor). They are kept apart so the recovery probe, which clears every cooldown
// of a provider at once, never erases a quiet pause.
type BoardPauses struct {
	Cooldown time.Time
	Quiet    time.Time
}

// CooledBoard identifies one board currently in an active cooldown. Region disambiguates a
// board id that repeats across independent regional slices of one provider (e.g. Adzuna's
// "it-jobs" once per country) — without it, every region's health/cooldown state would
// collide on the same (provider, board) identity. A provider whose board is region-invariant
// (everyone but Adzuna today) reports Region == "".
type CooledBoard struct {
	Board  string
	Region string
}
