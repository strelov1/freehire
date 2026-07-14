// Package pipeline is the ingest write path: it dispatches each configured board to
// its source adapter, normalizes the postings, and persists them. It is independent
// of the DB layer (via Store) and of any specific platform (via the source registry).
package pipeline

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
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

// closer is the optional Store capability a self-closing streaming source needs: closing a
// posting by its (source, external_id) identity when the feed reports it removed. Only the
// ingest dbStore implements it; ingestStream type-asserts for it and skips removals when a
// Store lacks it, so other Store implementations (and test fakes) are unaffected.
type closer interface {
	Close(ctx context.Context, source, externalID string) error
}

// toucher is the optional Store capability a HydratingSource needs: refresh a posting's liveness
// (last_seen_at, reopen if closed) by its (source, external_id) identity, WITHOUT rewriting its
// content. The pipeline uses it for a posting the adapter re-listed but did not re-fetch (its
// stored content is already current); a full upsert of the content-less listing would wipe the
// hydrated description/facets. Only the ingest dbStore implements it; the pipeline type-asserts
// for it and drops the refresh when a Store lacks it (test fakes), like closer.
type toucher interface {
	Touch(ctx context.Context, source, externalID string) error
}

// seenLookup is the optional Store capability a HydratingSource needs: the set of external_ids
// already stored for a provider, so the adapter fetches expensive per-posting detail only for
// postings the catalogue lacks. Only the ingest dbStore implements it; the runner type-asserts
// for it and falls back to the list-only Fetch when a Store lacks it (test fakes, non-DB
// callers), so other Store implementations are unaffected.
type seenLookup interface {
	ExistingExternalIDs(ctx context.Context, source string) (map[string]struct{}, error)
}

// BoardHealth is the optional per-board health port: it tells the Runner whether a
// board is currently cooled down (skip it) and records each crawl's outcome so a
// repeatedly-failing board backs off. A nil BoardHealth disables the feature entirely
// (the Runner behaves exactly as before), so unit tests and non-DB callers are
// unaffected — the same shape as the optional closer.
type BoardHealth interface {
	// Cooldown reports the board's cooldown_until and whether it is set. The Runner
	// skips the board when it is set and in the future.
	Cooldown(ctx context.Context, provider, board string) (time.Time, bool, error)
	// RecordSuccess clears the board's failure state and stamps freshness.
	RecordSuccess(ctx context.Context, provider, board string, ingested int) error
	// RecordFailure counts a failed crawl and cools the board down per the backoff policy.
	RecordFailure(ctx context.Context, provider, board, errMsg string) error
}

// Stats reports what a run did: Ingested counts saved jobs, Failed counts boards that
// errored (unknown provider or a fetch failure), Skipped counts jobs that fetched fine
// but failed to persist, and Cooled counts boards skipped because they are in cooldown
// (a deliberate back-off, distinct from a failure). Skipped is surfaced so a run whose
// every save fails (e.g. a DB schema drift) is not mistaken for a clean success.
type Stats struct {
	Ingested int
	Failed   int
	Skipped  int
	Cooled   int
}

// RunStats is a run's outcome broken down by provider. A run may cover several providers
// (a mixed board file), and the post-run unseen-job sweep is per provider, so the breakdown
// is kept rather than a single aggregate; Total folds it back when only the sum is needed.
type RunStats map[string]Stats

// Total sums the per-provider stats into one aggregate (for the run's done-log line).
func (rs RunStats) Total() Stats {
	var t Stats
	for _, s := range rs {
		t.Ingested += s.Ingested
		t.Failed += s.Failed
		t.Skipped += s.Skipped
		t.Cooled += s.Cooled
	}
	return t
}

// Runner drives ingest: for each configured board it looks up the adapter, fetches,
// normalizes, and saves. Boards run concurrently up to defaultConcurrency; a board
// failure is isolated and never aborts the run.
type Runner struct {
	Registry map[string]sources.Source
	Store    Store
	// BoardHealth is optional (nil disables per-board cooldown/health recording).
	BoardHealth BoardHealth
}

// Run ingests every configured board and returns the stats per provider. It returns an
// error only for a context cancellation, never for a single board's failure. All boards
// run in one bounded concurrent pool regardless of provider, so a slow self-pacing
// provider occupies one slot without blocking the others.
func (r Runner) Run(ctx context.Context, entries []sources.CompanyEntry) (RunStats, error) {
	var (
		mu     sync.Mutex
		byProv = RunStats{}
		wg     sync.WaitGroup
	)

	// Heartbeat the crawl progress: boards run concurrently and a successful board
	// logs nothing, so without this a run that stalls on one hung board is silent.
	var crawled atomic.Int64
	total := len(entries)
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		log.Printf("ingest: progress %d/%d boards crawled", crawled.Load(), total)
	})
	defer stopHeartbeat()
	sem := make(chan struct{}, defaultConcurrency)

	for _, e := range entries {
		wg.Add(1)
		go func(e sources.CompanyEntry) {
			defer wg.Done()

			// Acquire a slot, but abandon the board if the run is cancelled — both
			// before starting and while parked waiting for a slot.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}

			ingested, failed, skipped, cooled := r.ingestBoard(ctx, e)
			crawled.Add(1)

			mu.Lock()
			s := byProv[e.Provider]
			s.Ingested += ingested
			s.Failed += failed
			s.Skipped += skipped
			s.Cooled += cooled
			byProv[e.Provider] = s
			mu.Unlock()
		}(e)
	}
	wg.Wait()

	return byProv, ctx.Err()
}

// ingestBoard fetches and saves one board, returning how many jobs it ingested, whether
// the board itself failed (1) or not (0), how many jobs were skipped on a save error, and
// whether the board was skipped for cooldown (1). A missing adapter or a fetch error fails
// the board; a per-job save error skips that job without failing the board, but is counted
// and logged so it is never silently swallowed. A board in cooldown is skipped before its
// adapter is touched, and each crawl's board-level outcome is recorded to BoardHealth.
func (r Runner) ingestBoard(ctx context.Context, e sources.CompanyEntry) (ingested, failed, skipped, cooled int) {
	// Cooldown gate — before the adapter lookup, so a backed-off board costs nothing.
	if r.cooledDown(ctx, e) {
		log.Printf("ingest: %s board %q (%s) in cooldown — skipping", e.Provider, e.Board, e.Company)
		return 0, 0, 0, 1
	}

	src, ok := r.Registry[e.Provider]
	if !ok {
		log.Printf("ingest: %s/%s: unknown provider %q", e.Company, e.Board, e.Provider)
		r.recordFailure(ctx, e, "unknown provider "+e.Provider)
		return 0, 1, 0, 0
	}

	// A streaming adapter persists postings as it crawls, so a long rate-limited board's
	// progress is saved incrementally (and survives an interrupted run) rather than buffered
	// until the whole board finishes.
	if ss, streaming := src.(sources.StreamingSource); streaming {
		ing, fail, skip := r.ingestStream(ctx, e, ss)
		// A streaming board that saved nothing AND failed is a true outage; partial progress
		// (some jobs saved before a mid-crawl error) is a success signal, not a board failure.
		if fail > 0 && ing == 0 {
			r.recordFailure(ctx, e, "streaming board failed with no progress")
		} else {
			r.recordSuccess(ctx, e, ing)
		}
		return ing, fail, skip, 0
	}

	raw, err := r.fetchBoard(ctx, e, src)
	if err != nil {
		// Log the cause so a failed board is diagnosable (the source error carries
		// the HTTP status / timeout); the run still isolates and continues.
		log.Printf("ingest: %s board %q (%s) failed: %v", e.Provider, e.Board, e.Company, err)
		r.recordFailure(ctx, e, err.Error())
		return 0, 1, 0, 0
	}

	var firstErr error
	for _, j := range raw {
		// A HydratingSource marks an already-ingested posting it re-listed but did not
		// re-fetch: refresh its liveness by identity instead of re-upserting content-less
		// (which would wipe the description/facets hydrated when it was new).
		if j.SeenRefresh {
			if err := r.touch(ctx, e, j); err != nil {
				skipped++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			ingested++
			continue
		}
		dj, err := normalizeJob(e, j)
		if err != nil {
			skipped++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := r.Store.Save(ctx, dj); err != nil {
			skipped++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ingested++
	}
	// One line per board with skips (not one per job), so a systemic failure — e.g.
	// the DB behind a migration, or a board whose postings won't construct — is visible
	// without flooding the log.
	if skipped > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d jobs on construct or save error (e.g. %v)",
			e.Provider, e.Board, e.Company, skipped, len(raw), firstErr)
	}
	// The board was reachable (Fetch succeeded), so it is healthy regardless of per-job
	// save skips — those are stats.Skipped, not a board outage.
	r.recordSuccess(ctx, e, ingested)
	return ingested, 0, skipped, 0
}

// fetchBoard fetches a board's postings, preferring a hydrating adapter's FetchNew — which
// fetches per-posting detail (e.g. the description the list omits) only for postings not already
// ingested — when the adapter opts in AND the Store can supply the provider's seen-set. It falls
// back to the list-only Fetch otherwise. The seen predicate namespaces the adapter's raw posting
// id the same way the write path does, so it matches the stored external_id. A seen-set lookup
// error fails OPEN (empty set → every posting treated as new), so a health hiccup never skips the
// board.
func (r Runner) fetchBoard(ctx context.Context, e sources.CompanyEntry, src sources.Source) ([]sources.Job, error) {
	hs, ok := src.(sources.HydratingSource)
	if !ok {
		return src.Fetch(ctx, e)
	}
	sl, ok := r.Store.(seenLookup)
	if !ok {
		return src.Fetch(ctx, e)
	}
	set, err := sl.ExistingExternalIDs(ctx, e.Provider)
	if err != nil {
		log.Printf("ingest: %s seen-set lookup failed, hydrating every posting as new: %v", e.Provider, err)
		set = nil
	}
	seen := func(externalID string) bool {
		_, ok := set[sources.NamespaceExternalID(e.Board, externalID)]
		return ok
	}
	return hs.FetchNew(ctx, e, seen)
}

// touch refreshes an already-ingested posting's liveness (last_seen_at, reopen) by identity,
// without rewriting its content. It routes through the Store's optional toucher capability; a
// Store without it (a test fake) drops the refresh, matching how ingestStream handles closer.
func (r Runner) touch(ctx context.Context, e sources.CompanyEntry, j sources.Job) error {
	t, ok := r.Store.(toucher)
	if !ok {
		return nil
	}
	source, externalID := jobIdentity(e, j)
	return t.Touch(ctx, source, externalID)
}

// cooledDown reports whether a board is currently backed off. It fails OPEN: a health
// lookup error is logged and the board is crawled anyway, so a health-store hiccup never
// stalls ingest. Always false when no BoardHealth port is wired.
func (r Runner) cooledDown(ctx context.Context, e sources.CompanyEntry) bool {
	if r.BoardHealth == nil {
		return false
	}
	until, set, err := r.BoardHealth.Cooldown(ctx, e.Provider, e.Board)
	if err != nil {
		log.Printf("ingest: cooldown check %s/%s: %v", e.Provider, e.Board, err)
		return false
	}
	return set && until.After(time.Now())
}

// recordSuccess / recordFailure persist a board's outcome best-effort: a health-store
// error is logged and never propagated (health must not fail ingest), and both no-op
// when no BoardHealth port is wired.
func (r Runner) recordSuccess(ctx context.Context, e sources.CompanyEntry, ingested int) {
	if r.BoardHealth == nil {
		return
	}
	if err := r.BoardHealth.RecordSuccess(ctx, e.Provider, e.Board, ingested); err != nil {
		log.Printf("ingest: record board success %s/%s: %v", e.Provider, e.Board, err)
	}
}

func (r Runner) recordFailure(ctx context.Context, e sources.CompanyEntry, msg string) {
	if r.BoardHealth == nil {
		return
	}
	if err := r.BoardHealth.RecordFailure(ctx, e.Provider, e.Board, msg); err != nil {
		log.Printf("ingest: record board failure %s/%s: %v", e.Provider, e.Board, err)
	}
}

// ingestStream drives a streaming board: it persists each posting the adapter emits as it is
// crawled, so progress is durable mid-run. The emit sink normalizes and saves under a mutex
// (the adapter may emit concurrently) and tallies the same ingested/skipped counts as the
// buffered path; a board-level FetchStream error counts the board failed but keeps whatever was
// already saved (the 48h unseen-sweep guards against a short crawl closing the un-reached tail).
func (r Runner) ingestStream(ctx context.Context, e sources.CompanyEntry, ss sources.StreamingSource) (ingested, failed, skipped int) {
	var (
		mu       sync.Mutex
		firstErr error
		total    int
	)
	emit := func(j sources.Job) {
		mu.Lock()
		defer mu.Unlock()
		total++
		// A self-closing source emits removed postings: close by identity instead of
		// upserting. The Store must implement closer (the ingest dbStore does); a Store
		// without it simply drops the removal (e.g. a test fake that never sees them).
		if j.Removed {
			c, ok := r.Store.(closer)
			if !ok {
				return
			}
			source, externalID := jobIdentity(e, j)
			if err := c.Close(ctx, source, externalID); err != nil {
				skipped++
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			ingested++
			return
		}
		dj, err := normalizeJob(e, j)
		if err != nil {
			skipped++
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		if err := r.Store.Save(ctx, dj); err != nil {
			skipped++
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		ingested++
	}

	err := ss.FetchStream(ctx, e, emit)
	if skipped > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d jobs on save error (e.g. %v)",
			e.Provider, e.Board, e.Company, skipped, total, firstErr)
	}
	if err != nil {
		log.Printf("ingest: %s board %q (%s) failed after %d saved: %v",
			e.Provider, e.Board, e.Company, ingested, err)
		return ingested, 1, skipped
	}
	return ingested, 0, skipped
}

// jobIdentity is the dedup key a posting persists under: the provider is the source, and the
// external id is namespaced by board so two companies on one platform cannot collide. Both
// the upsert (via normalizeJob) and the stream-driven close derive identity here, so a
// removal closes exactly the row a live emit would have upserted.
func jobIdentity(e sources.CompanyEntry, j sources.Job) (source, externalID string) {
	return e.Provider, sources.NamespaceExternalID(e.Board, j.ExternalID)
}

// normalizeJob turns a raw posting into the Job aggregate through the factory: the
// platform becomes the source and the external id is namespaced by board (so two
// companies on one platform cannot collide), and job.New derives the slugs and the
// dictionary facets internally — so ingest, the moderator write path, and Telegram
// extraction all produce identical facets from the one door. It returns
// job.ErrInvalidDraft for a posting with no title/identity, which the caller skips.
func normalizeJob(e sources.CompanyEntry, j sources.Job) (job.Job, error) {
	source, externalID := jobIdentity(e, j)
	return job.New(job.Draft{
		Source:             source,
		ExternalID:         externalID,
		URL:                j.URL,
		Title:              j.Title,
		Company:            j.Company,
		Location:           j.Location,
		Remote:             j.Remote,
		Description:        j.Description,
		PostedAt:           j.PostedAt,
		WorkMode:           j.WorkMode,
		Seniority:          j.Seniority,
		Category:           j.Category,
		EmploymentType:     j.EmploymentType,
		Skills:             j.Skills,
		ExperienceYearsMin: j.ExperienceYearsMin,
	})
}
