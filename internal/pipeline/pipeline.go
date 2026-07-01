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

	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// defaultConcurrency bounds how many boards are fetched at once.
const defaultConcurrency = 8

// progressInterval is how often the run emits a heartbeat with the count of boards
// crawled so far, so a stalled board (one whose fetch hangs) is visible — the count
// stops advancing — instead of the run going silent until it finishes.
const progressInterval = 60 * time.Second

// Job is a normalized posting ready to persist: the pipeline has set the platform as
// source, namespaced the external id by board, derived the company slug, and minted
// the public slug.
type Job struct {
	Source      string
	ExternalID  string
	URL         string
	Title       string
	Company     string
	CompanySlug string
	PublicSlug  string
	Location    string
	Remote      bool
	Description string
	PostedAt    *time.Time
	// Countries/Regions/WorkMode are parsed from Location: ISO alpha-2 codes,
	// region codes, and a work-mode hint. Each is empty when the location states
	// nothing the parser can resolve.
	Countries []string
	Regions   []string
	Cities    []string
	WorkMode  string
	// Skills are the deterministic technology tags parsed from Description by
	// internal/skilltag. Empty when the description resolves no known skill.
	Skills []string
	// Seniority and Category are deterministic classification parsed from Title by
	// internal/classify. Each is "" when the title resolves nothing.
	Seniority string
	Category  string
	// Synthetic enrichment facets derived from Title/Description by jobderive
	// (internal/lang + internal/jobfacts): the posting language, employment type,
	// education level, English level, and minimum required experience. Each is
	// empty/nil when nothing is resolved.
	PostingLanguage    string
	EmploymentType     string
	EducationLevel     string
	EnglishLevel       string
	ExperienceYearsMin *int
}

// Store persists one normalized job and enqueues it for enrichment when needed,
// atomically. The pipeline is unaware of the schema version or the outbox — that is
// the Store implementation's concern.
type Store interface {
	Save(ctx context.Context, job Job) error
}

// closer is the optional Store capability a self-closing streaming source needs: closing a
// posting by its (source, external_id) identity when the feed reports it removed. Only the
// ingest dbStore implements it; ingestStream type-asserts for it and skips removals when a
// Store lacks it, so other Store implementations (and test fakes) are unaffected.
type closer interface {
	Close(ctx context.Context, source, externalID string) error
}

// Stats reports what a run did: Ingested counts saved jobs, Failed counts boards that
// errored (unknown provider or a fetch failure), and Skipped counts jobs that fetched
// fine but failed to persist. Skipped is surfaced so a run whose every save fails (e.g.
// a DB schema drift) is not mistaken for a clean ingested=0/failed=0 success.
type Stats struct {
	Ingested int
	Failed   int
	Skipped  int
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
	}
	return t
}

// Runner drives ingest: for each configured board it looks up the adapter, fetches,
// normalizes, and saves. Boards run concurrently up to defaultConcurrency; a board
// failure is isolated and never aborts the run.
type Runner struct {
	Registry map[string]sources.Source
	Store    Store
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

			ingested, failed, skipped := r.ingestBoard(ctx, e)
			crawled.Add(1)

			mu.Lock()
			s := byProv[e.Provider]
			s.Ingested += ingested
			s.Failed += failed
			s.Skipped += skipped
			byProv[e.Provider] = s
			mu.Unlock()
		}(e)
	}
	wg.Wait()

	return byProv, ctx.Err()
}

// ingestBoard fetches and saves one board, returning how many jobs it ingested, whether
// the board itself failed (1) or not (0), and how many jobs were skipped on a save error.
// A missing adapter or a fetch error fails the board; a per-job save error skips that job
// without failing the board, but is counted and logged so it is never silently swallowed.
func (r Runner) ingestBoard(ctx context.Context, e sources.CompanyEntry) (ingested, failed, skipped int) {
	src, ok := r.Registry[e.Provider]
	if !ok {
		log.Printf("ingest: %s/%s: unknown provider %q", e.Company, e.Board, e.Provider)
		return 0, 1, 0
	}

	// A streaming adapter persists postings as it crawls, so a long rate-limited board's
	// progress is saved incrementally (and survives an interrupted run) rather than buffered
	// until the whole board finishes.
	if ss, streaming := src.(sources.StreamingSource); streaming {
		return r.ingestStream(ctx, e, ss)
	}

	raw, err := src.Fetch(ctx, e)
	if err != nil {
		// Log the cause so a failed board is diagnosable (the source error carries
		// the HTTP status / timeout); the run still isolates and continues.
		log.Printf("ingest: %s board %q (%s) failed: %v", e.Provider, e.Board, e.Company, err)
		return 0, 1, 0
	}

	var firstErr error
	for _, j := range raw {
		if err := r.Store.Save(ctx, normalizeJob(e, j)); err != nil {
			skipped++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ingested++
	}
	// One line per board with skips (not one per job), so a systemic save failure —
	// e.g. the DB behind a migration — is visible without flooding the log.
	if skipped > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d jobs on save error (e.g. %v)",
			e.Provider, e.Board, e.Company, skipped, len(raw), firstErr)
	}
	return ingested, 0, skipped
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
		if err := r.Store.Save(ctx, normalizeJob(e, j)); err != nil {
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

// normalizeJob turns a raw posting into a persistable Job: the platform becomes the
// source, the external id is namespaced by board so two companies on one platform
// cannot collide, the company slug is derived with the shared normalizer, and the
// public slug is minted from the same (source, external_id) identity so it is stable
// across re-ingests and deterministic with the dedup key.
func normalizeJob(e sources.CompanyEntry, j sources.Job) Job {
	source, externalID := jobIdentity(e, j)
	// The slugs and dictionary facets (geography/work-mode/skills/classification) are
	// derived by the shared jobderive helper, so ingest and the moderator write path
	// produce identical facets. The adapter's structured facet signals (work-mode,
	// seniority, category, skills, experience) take precedence over the dictionary
	// there; an unset signal lets the dictionary decide.
	d := jobderive.Derive(jobderive.Input{
		Title:              j.Title,
		Company:            j.Company,
		Source:             source,
		ExternalID:         externalID,
		Location:           j.Location,
		Description:        j.Description,
		WorkMode:           j.WorkMode,
		Seniority:          j.Seniority,
		Category:           j.Category,
		Skills:             j.Skills,
		ExperienceYearsMin: j.ExperienceYearsMin,
	})
	return Job{
		Source:      source,
		ExternalID:  externalID,
		URL:         j.URL,
		Title:       j.Title,
		Company:     j.Company,
		CompanySlug: d.CompanySlug,
		PublicSlug:  d.PublicSlug,
		Location:    j.Location,
		Remote:      j.Remote,
		Description: j.Description,
		PostedAt:    j.PostedAt,
		Countries:   d.Countries,
		Regions:     d.Regions,
		Cities:      d.Cities,
		WorkMode:    d.WorkMode,
		Skills:      d.Skills,
		Seniority:   d.Seniority,
		Category:    d.Category,

		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		EnglishLevel:       d.EnglishLevel,
		ExperienceYearsMin: d.ExperienceYearsMin,
	}
}
