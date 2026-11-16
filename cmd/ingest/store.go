package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/adzunadesc"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/pgerr"
	"github.com/strelov1/freehire/internal/search/search"
)

// dbStore adapts the generated queries and pool to pipeline.Store. Save writes the job and its gated enrichment and search-index enqueues in one transaction. cmd/search-drain drains search_outbox; cmd/ingest never talks to the search engine.
type dbStore struct {
	pool          *pgxpool.Pool
	q             *db.Queries
	targetVersion int32
	crawled       *crawledSet
	tally         *writeTally
	// seen decides what the seen-set withholds, and so what a hydrating adapter re-fetches.
	seen seenPolicy
	// websites holds the company slugs whose website this run has already offered, so one
	// employer's many postings cost one statement rather than one each.
	websites sync.Map
}

// seenPolicy gathers the three independent reasons a stored posting is offered to a hydrating adapter for another detail fetch; they are one decision, made in ExistingExternalIDs.
type seenPolicy struct {
	// hydrationWindow is how long a row with NO description stays withheld, so a failed first detail fetch is retried. See hydrationRetryWindowFor.
	hydrationWindow time.Duration
	// refetchAll withholds EVERY row, so a hydrating adapter treats every listed posting as new and the pipeline re-writes it. The repair path for an adapter fix that changes what the listing yields.
	refetchAll bool
	// bodies withholds a row whose stored BODY has gone stale, so an employer's later edit is
	// re-read. Disabled unless BODY_REFRESH_DAYS is set — see sweeppolicy.BodyRefreshFor.
	bodies sweeppolicy.BodyRefresh
}

// dbStore is the only non-test implementation of pipeline.Store and must carry every optional capability below: the pipeline finds them by type assertion and silently degrades on a miss. A dropped Touch stops refreshing last_seen_at, and the 48h unseen sweep then closes live jobs.
var (
	_ pipeline.Store                    = (*dbStore)(nil)
	_ pipeline.FormSaver                = (*dbStore)(nil)
	_ pipeline.Closer                   = (*dbStore)(nil)
	_ pipeline.Toucher                  = (*dbStore)(nil)
	_ pipeline.SeenLookup               = (*dbStore)(nil)
	_ pipeline.CompanyDescriptionFiller = (*dbStore)(nil)
	_ pipeline.CompanyWebsiteRecorder   = (*dbStore)(nil)
)

func newDBStore(pool *pgxpool.Pool, targetVersion int, crawled *crawledSet, tally *writeTally, seen seenPolicy) *dbStore {
	return &dbStore{
		pool:          pool,
		q:             db.New(pool),
		targetVersion: int32(targetVersion),
		crawled:       crawled,
		tally:         tally,
		seen:          seen,
	}
}

// written is what save needs back from either write. Both return narrow columns on purpose: an embedded row would detoast the description and ship the semantic vector for every re-seen posting, and the complete job is the jobs_read view, which sqlc.embed(jobs) would bypass.
//
// full is which write ran; leftClosed is the full write's report that it left the row closed (the cheap write never changes closed_at).
type written struct {
	id          int64
	source      string
	companySlug string
	duplicateOf pgtype.Int8
	inserted    bool
	changed     bool
	full        bool
	leftClosed  bool
}

func fullWrite(r db.UpsertJobRow) written {
	return written{
		id:          r.ID,
		source:      r.Source,
		companySlug: r.CompanySlug,
		duplicateOf: r.DuplicateOf,
		inserted:    r.Inserted.Bool,
		changed:     r.Changed,
		full:        true,
		leftClosed:  r.ClosedAt.Valid,
	}
}

// cheapWrite leaves inserted and changed false: the row was neither created nor edited.
func cheapWrite(r db.RefreshUnchangedJobRow) written {
	return written{id: r.ID, source: r.Source, companySlug: r.CompanySlug, duplicateOf: r.DuplicateOf}
}

// withheldIfSeenBefore is the last_seen_at before which search had withheld a posting as
// unseen (search.UnseenWindow); a liveness write that moves it past this requeues the posting.
func withheldIfSeenBefore() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(-search.UnseenWindow), Valid: true}
}

// errCheapFenced reports that the cheap write matched the stored row but was refused because the row's state changed while it waited (a close, a content or metadata edit). Taking the jobs row now would reverse the lock order (jobs, then activity) and deadlock against a closer, so persist returns, saveOnce rolls back, and save retries in a fresh transaction.
var errCheapFenced = errors.New("cheap write fenced by a concurrent state change")

// maxFencedAttempts bounds how often one posting's cheap write may lose to a concurrent change before save reports an error; the next crawl retries it.
const maxFencedAttempts = 3

// persist refreshes activity when the stored posting matches, leaving the jobs tuple alone.
// Edited, closed and absent postings take the full write. A fenced match must be retried in a
// fresh transaction before either path can run again.
func persist(ctx context.Context, tx pgx.Tx, qtx *db.Queries, p db.UpsertJobParams) (written, error) {
	cheap, err := qtx.RefreshUnchangedJob(ctx, db.RefreshUnchangedJobParams{
		Source:               p.Source,
		ExternalID:           p.ExternalID,
		ContentHash:          p.ContentHash,
		Cities:               p.Cities,
		SalaryMinSource:      p.SalaryMinSource,
		SalaryMaxSource:      p.SalaryMaxSource,
		SalaryCurrencySource: p.SalaryCurrencySource,
		SalaryPeriodSource:   p.SalaryPeriodSource,
		EnglishLevel:         p.EnglishLevel,
		IsTech:               p.IsTech,
		Engagement:           p.Engagement,
		ApplicationDeadline:  p.ApplicationDeadline,
		Team:                 p.Team,
		BodyFromCatalogue:    p.BodyFromCatalogue,
		WithheldIfSeenBefore: withheldIfSeenBefore(),
	})
	if err == nil {
		if cheap.Fenced {
			return written{}, errCheapFenced
		}
		return cheapWrite(cheap), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return written{}, fmt.Errorf("refresh unchanged job: %w", err)
	}

	// No row matched, so no activity lock is held and the full write takes the jobs row first.
	full, err := job.Upsert(ctx, tx, p)
	if err != nil {
		return written{}, fmt.Errorf("upsert job: %w", err)
	}
	return fullWrite(full), nil
}

// needsIndex reports whether a write changed what search would show: a new row or a changed content_hash. A bookkeeping-only re-ingest (last_seen_at) is skipped.
func needsIndex(w written) bool {
	if w.closed() {
		return false
	}
	return w.changed || w.inserted
}

// closed reports whether the write left the row closed.
func (w written) closed() bool { return w.leftClosed }

// tookCheapWrite reports which of the two writes produced this, exactly rather than by
// inference from a missing field.
func (w written) tookCheapWrite() bool { return !w.full }

// clustersByRole reports whether a write is worth the role-cluster lookup (jobdedup.CanonicalForRole). A cost gate, not correctness: a full crawl replays millions of rows against an I/O-bound database.
//
// Only a new posting is asked, since a per-city fan-out arrives as inserts. A posting that becomes a duplicate later through an edit is left to the batch recompute. A row already carrying a marker is not re-asked.
func clustersByRole(w written) bool {
	return w.inserted && !w.duplicateOf.Valid
}

// needsRecentFeed reports whether a write belongs on the homepage "recently added" feed: the needsIndex eligibility (new or changed, not a role repost, not a duplicate) plus tech evidence, since the feed is IT-only. isTech.Valid is required: an unresolved (NULL) verdict is not eligible yet.
func needsRecentFeed(w written, deduped bool, isTech pgtype.Bool) bool {
	return needsIndex(w) && !deduped && !w.duplicateOf.Valid && isTech.Valid && isTech.Bool
}

// Save persists a posting whose adapter yielded no application form — every provider but
// Recruitee.
func (s *dbStore) Save(ctx context.Context, j job.Job) error {
	return s.save(ctx, j, nil)
}

// SaveWithApplyForm persists a posting with its application form in ONE transaction: a form written after a committed job could be lost to a crash, and nothing would queue it, since the capture queue covers only providers whose form costs a request.
func (s *dbStore) SaveWithApplyForm(ctx context.Context, j job.Job, form applyform.Form) error {
	return s.save(ctx, j, &form)
}

// save persists the posting, retrying a fenced cheap write in a fresh transaction. The retry is a fresh CHEAP attempt, never a forced full write, which would rewrite from parameters that predate the refusing change; a key that no longer matches falls to the full write by itself.
func (s *dbStore) save(ctx context.Context, j job.Job, form *applyform.Form) error {
	f := j.Fields()
	err := retryFenced(
		func() error { return s.saveOrRetrySlug(ctx, j, form) },
		func() { s.tally.recordFenced(f.Source) },
	)
	if errors.Is(err, errCheapFenced) {
		return fmt.Errorf("save %s/%s: %w", f.Source, f.ExternalID, err)
	}
	return err
}

// retryFenced runs attempt until it stops reporting errCheapFenced, at most maxFencedAttempts
// times, calling onFenced after each refused attempt. A budget spent is an error, not a skip.
func retryFenced(attempt func() error, onFenced func()) error {
	for n := 1; ; n++ {
		err := attempt()
		if !errors.Is(err, errCheapFenced) {
			return err
		}
		onFenced()
		if n == maxFencedAttempts {
			return fmt.Errorf("cheap write fenced %d times: %w", n, err)
		}
	}
}

func (s *dbStore) saveOrRetrySlug(ctx context.Context, j job.Job, form *applyform.Form) error {
	err := s.saveOnce(ctx, j, form, false)
	if constraint, ok := pgerr.UniqueViolationConstraint(err); !ok || constraint != "jobs_public_slug_key" {
		return err
	}
	// A failed statement aborts the transaction. saveOnce has rolled it back before
	// this retry, so the posting and all of its outboxes still commit together.
	log.Print("ingest: public slug collision; retrying with the full identity hash")
	return s.saveOnce(ctx, j, form, true)
}

// fuzzyCanonicalFn is a seam so a test can force a real SQL failure inside the lookup.
var fuzzyCanonicalFn = func(ctx context.Context, q *db.Queries, p db.UpsertJobParams, id int64) (int64, bool, error) {
	return jobdedup.CanonicalForFuzzy(ctx, q, p, id)
}

// fuzzyCanonical runs the fuzzy lookup inside a savepoint. A failed SELECT (a statement timeout,
// say) aborts the transaction; rolling back to the savepoint revives it, so the row is still
// written, unmarked, and the batch pass decides. Only a savepoint failure is returned.
func (s *dbStore) fuzzyCanonical(ctx context.Context, tx pgx.Tx, params db.UpsertJobParams, id int64) (int64, bool, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	canon, ok, lerr := fuzzyCanonicalFn(ctx, s.q.WithTx(sp), params, id)
	if lerr != nil {
		log.Printf("ingest: fuzzy dedup lookup failed, writing unmarked: %v", lerr)
		return 0, false, sp.Rollback(ctx)
	}
	return canon, ok, sp.Commit(ctx)
}

func (s *dbStore) saveOnce(ctx context.Context, j job.Job, form *applyform.Form, fullSlugHash bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after Commit is a no-op; suppress its error (matches the other stores).
	defer func() { _ = tx.Rollback(ctx) }()

	// The read projection carries every persistable field; enrichment is owned by SetJobEnrichment, so it is not mapped. The mapping also stamps content_hash and role_fingerprint.
	params := j.Fields().UpsertParams()
	if fullSlugHash {
		params.PublicSlug = normalize.JobSlugWithFullHash(params.Title, params.Company, params.Source, params.ExternalID)
		// Keep the incoming content hash, just as UpsertJob does when it preserves
		// an existing URL after a title edit. An unchanged recrawl must stay cheap.
	}

	qtx := s.q.WithTx(tx)
	saved, err := persist(ctx, tx, qtx, params)
	if err != nil {
		return err
	}

	// A company posting one role per city arrives as many board identities sharing a role_fingerprint. After the upsert (the answer needs the row id), point this row at an older canonical copy if one exists; otherwise the copies stay separate searchable jobs until the next batch recompute.
	deduped := false
	if clustersByRole(saved) {
		if canon, ok := jobdedup.CanonicalForRole(ctx, qtx, params, saved.id); ok {
			if _, err := qtx.MarkJobDuplicateOfRole(ctx, db.MarkJobDuplicateOfRoleParams{
				ID:              saved.id,
				DuplicateOfRole: pgtype.Int8{Int64: canon.ID, Valid: true},
			}); err != nil {
				return fmt.Errorf("mark job %d duplicate of %d: %w", saved.id, canon.ID, err)
			}
			deduped = true
		}
	}
	// The fuzzy tier, the same question for a near-identical repost the exact role cluster
	// missed (descriptions differing by a localized block). CanonicalForFuzzy runs the batch
	// pass's own rule, so the next reindex keeps this verdict. Only a row the role tier left
	// alone is asked: a marked row is already invisible.
	if !deduped && clustersByRole(saved) {
		canon, ok, err := s.fuzzyCanonical(ctx, tx, params, saved.id)
		if err != nil {
			return err
		}
		if ok {
			if _, err := qtx.MarkJobDuplicateOfFuzzy(ctx, db.MarkJobDuplicateOfFuzzyParams{
				ID:               saved.id,
				DuplicateOfFuzzy: pgtype.Int8{Int64: canon, Valid: true},
			}); err != nil {
				return fmt.Errorf("mark job %d fuzzy duplicate of %d: %w", saved.id, canon, err)
			}
			deduped = true
		}
	}

	// The form belongs to the posting even if it is not a searchable canon (a repost is still applicable). Unlike the enrichment enqueue below, this writes bytes we already hold rather than spending LLM budget.
	if form != nil {
		payload, err := json.Marshal(form)
		if err != nil {
			return fmt.Errorf("encode apply form: %w", err)
		}
		if err := qtx.UpsertApplyForm(ctx, db.UpsertApplyFormParams{
			JobID:    saved.id,
			Provider: form.Provider,
			Payload:  payload,
		}); err != nil {
			return fmt.Errorf("upsert apply form: %w", err)
		}
	}

	// A duplicate never reaches search, so enriching it pays an LLM for an invisible row; so
	// does a row the write left closed (a deadline already past).
	if !deduped && !saved.closed() {
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion: s.targetVersion,
			JobID:         saved.id,
		}); err != nil {
			return fmt.Errorf("enqueue enrichment: %w", err)
		}
	}

	// Queue the write for the live search index atomically with the row, when it inserted or changed indexed content and the row is a searchable canon (mirrors the enrichment gate). Unconditional on search being configured: an unconfigured deployment never drains it.
	if needsIndex(saved) && !deduped && !saved.duplicateOf.Valid {
		if err := qtx.EnqueueSearchOutbox(ctx, saved.id); err != nil {
			return fmt.Errorf("enqueue search outbox: %w", err)
		}
	}

	// Queue the write for the homepage "recently added" feed atomically with the row; eligibility is needsRecentFeed. cmd/tg-extract is deliberately not wired in, like search_outbox.
	if needsRecentFeed(saved, deduped, params.IsTech) {
		if err := qtx.EnqueueRecentFeedOutbox(ctx, saved.id); err != nil {
			return fmt.Errorf("enqueue recent feed outbox: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Queue a form capture for a provider whose form costs its own request, AFTER the commit and never fatal: a failed statement would abort the transaction and discard the posting. The crash gap closes itself because the enqueue is gated on no stored form, not on the row being new.
	if applyform.NeedsRequestCapture(saved.source) && !saved.closed() {
		if _, err := s.q.EnqueueApplyFormCapture(ctx, saved.id); err != nil {
			log.Printf("ingest: queue apply-form capture for job %d: %v", saved.id, err)
		}
	}

	// Queue a full-description fetch for an Adzuna posting on Adzuna's own hosted page (the API gives only a ~500-char snippet). Same after-commit, never-fatal shape as the form capture; see adzunadesc.Eligible.
	if adzunadesc.Eligible(saved.source, params.URL) {
		if _, err := s.q.EnqueueAdzunaDescriptionCapture(ctx, saved.id); err != nil {
			log.Printf("ingest: queue adzuna description capture for job %d: %v", saved.id, err)
		}
	}

	// Record this (provider, company) as crawled so the stale sweep closes only jobs of companies this run wrote, not a provider's whole catalogue. Uses the persisted row so aggregator sources scope by their real companies.
	if s.crawled != nil {
		s.crawled.record(saved.source, saved.companySlug)
	}

	// Count which write this posting took after the commit, so a rolled-back attempt is not reported. See writeTally.
	s.tally.record(saved.source, saved.tookCheapWrite())

	return nil
}

// Close soft-closes a posting by (source, external_id), the close path for a self-closing source (jobtech). Idempotent: an already-closed or absent row is a no-op.
func (s *dbStore) Close(ctx context.Context, source, externalID string) error {
	if _, err := s.q.CloseJobBySourceExternalID(ctx, db.CloseJobBySourceExternalIDParams{
		Source:     source,
		ExternalID: externalID,
	}); err != nil {
		return fmt.Errorf("close job %s/%s: %w", source, externalID, err)
	}
	return nil
}

// ExistingExternalIDs returns the external_ids already stored for the board about to be crawled: the seen-set for a hydrating source, so detail is fetched only for postings the catalogue lacks. Implements pipeline.seenLookup.
//
// A board-bearing provider reads only its own namespace; a boardless adapter reads the provider's whole set. Each id maps to its tech evidence (is_tech IS TRUE), which the refresh path needs because a re-listed posting carries no description.
//
// Rows withheld from the set, so the crawl re-reads them:
//   - no description, until pipeline.HydrationRetryWindow old (retries a lost detail fetch);
//   - stored deadline passed within pipeline.DeadlineRecheckWindow (picks up an employer's extension);
//   - stale BODY, so an employer's later edit reaches the catalogue. Disabled unless BODY_REFRESH_DAYS is set and bounded by a slice; see sweeppolicy.BodyRefreshFor. The disabled form travels through the same parameters, not a second query;
//   - every row when refetchAll is set: the repair path for an adapter fix that changes what the LISTING yields, which otherwise never reaches stored rows because a re-listed posting rewrites no content. It reuses the ordinary write.
func (s *dbStore) ExistingExternalIDs(ctx context.Context, source, board string) (map[string]bool, error) {
	set := map[string]bool{}
	if s.seen.refetchAll {
		return set, nil
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-s.seen.hydrationWindow), Valid: true}
	bodyCutoff := pgtype.Timestamptz{Time: s.seen.bodies.Cutoff, Valid: true}
	deadlineFrom := pgtype.Date{Time: time.Now().UTC().Add(-pipeline.DeadlineRecheckWindow), Valid: true}
	if board == "" {
		rows, err := s.q.ExistingExternalIDs(ctx, db.ExistingExternalIDsParams{
			Source:              source,
			HydrationCutoff:     cutoff,
			BodyRefreshCutoff:   bodyCutoff,
			BodyRefreshSlices:   s.seen.bodies.Slices,
			BodyRefreshSlot:     s.seen.bodies.Slot,
			DeadlineRecheckFrom: deadlineFrom,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
		}
		return set, nil
	}
	// The pattern is escaped by externalid.BoardPattern: board names may contain LIKE syntax (workday names carry underscores).
	rows, err := s.q.ExistingExternalIDsByBoard(ctx, db.ExistingExternalIDsByBoardParams{
		Source:              source,
		Pattern:             externalid.BoardPattern(board),
		HydrationCutoff:     cutoff,
		BodyRefreshCutoff:   bodyCutoff,
		BodyRefreshSlices:   s.seen.bodies.Slices,
		BodyRefreshSlot:     s.seen.bodies.Slot,
		DeadlineRecheckFrom: deadlineFrom,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
	}
	return set, nil
}

var _ pipeline.StoredBodyLookup = (*dbStore)(nil)

// StoredDescription returns the body stored for a posting, "" when there is no row. Implements
// pipeline.StoredBodyLookup.
func (s *dbStore) StoredDescription(ctx context.Context, source, externalID string) (string, error) {
	body, err := s.q.StoredJobDescription(ctx, db.StoredJobDescriptionParams{Source: source, ExternalID: externalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return body, err
}

// refreshCoveredChunk bounds one RefreshCoveredJobs statement. A board's covered postings are
// refreshed in one pass after its crawl — 4dayweek's ~22k at once — and a chunk keeps any one
// statement's row locks and runtime small beside the crawl's own writes.
const refreshCoveredChunk = 5000

// RefreshCovered refreshes open stored rows listed without a write (coverage-gated or
// unreadable), and records each refreshed row's company as crawled, as Touch does, so the sweep's
// company scope reaches the rows the aggregator no longer lists. Implements
// pipeline.CoveredRefresher. Not counted in the write tally: these postings bypassed
// content writes.
//
// The companies are recorded only once every chunk has succeeded: a company whose postings span
// two chunks, one refreshed and one not, must not enter the sweep's scope, or the sweep would
// close its listed-but-unrefreshed rows.
func (s *dbStore) RefreshCovered(ctx context.Context, source string, externalIDs []string) ([]string, error) {
	var refreshed []string
	for chunk := range slices.Chunk(externalIDs, refreshCoveredChunk) {
		// The bulk statement skips rows another writer holds, commits what it could write and
		// releases every lock before any retry. A row it left unwritten is retried ALONE, waiting
		// for the writer: holding no other row while it waits, it cannot be one edge of a cycle
		// with a multi-row writer that visits rows in another order.
		rows, err := s.q.RefreshCoveredJobs(ctx, db.RefreshCoveredJobsParams{
			Source:      source,
			ExternalIds: chunk,
		})
		if err != nil {
			return nil, fmt.Errorf("refresh covered jobs %s: %w", source, err)
		}
		for _, r := range rows {
			if r.Refreshed {
				refreshed = append(refreshed, r.CompanySlug)
				continue
			}
			slug, ok, err := s.refreshCoveredOne(ctx, source, r.ExternalID)
			if err != nil {
				return nil, fmt.Errorf("refresh covered job %s/%s: %w", source, r.ExternalID, err)
			}
			if ok {
				refreshed = append(refreshed, slug)
			}
		}
	}
	if s.crawled != nil {
		for _, slug := range refreshed {
			s.crawled.record(source, slug)
		}
	}
	return refreshed, nil
}

// refreshCoveredOne refreshes one listed row the bulk statement left unwritten, waiting for the
// writer that held it. ok=false is a row that is closed, private or absent now: a no-op, as the
// refresh always was. A statement the state fence refused after waiting has ended, so the retry is
// a fresh statement; running out of attempts, or the context ending, is an error — the caller
// counts it as a failed refresh and the board never proves coverage with a listed row unrefreshed.
func (s *dbStore) refreshCoveredOne(ctx context.Context, source, externalID string) (slug string, ok bool, err error) {
	for attempt := 1; attempt <= maxFencedAttempts; attempt++ {
		row, err := s.q.RefreshCoveredJob(ctx, db.RefreshCoveredJobParams{Source: source, ExternalID: externalID})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if row.Refreshed {
			return row.CompanySlug, true, nil
		}
		s.tally.recordFenced(source)
	}
	return "", false, fmt.Errorf("fenced %d times", maxFencedAttempts)
}

// CloseCovered closes the open stored rows an aggregator board listed for employers the
// coverage gate found already crawled first-hand, and records each closed row's company as
// crawled, as RefreshCovered does. Implements pipeline.CoveredCloser.
func (s *dbStore) CloseCovered(ctx context.Context, source string, externalIDs []string) ([]string, error) {
	var closed []string
	for chunk := range slices.Chunk(externalIDs, refreshCoveredChunk) {
		slugs, err := s.q.CloseCoveredJobs(ctx, db.CloseCoveredJobsParams{
			Source:      source,
			ExternalIds: chunk,
		})
		if err != nil {
			return nil, fmt.Errorf("close covered jobs %s: %w", source, err)
		}
		closed = append(closed, slugs...)
	}
	if s.crawled != nil {
		for _, slug := range closed {
			s.crawled.record(source, slug)
		}
	}
	return closed, nil
}
