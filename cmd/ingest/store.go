package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/adzunadesc"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// dbStore adapts the generated queries + connection pool to pipeline.Store. Save runs
// the job upsert, the gated enrichment enqueue, and the gated search-index enqueue in
// one transaction, so a newly ingested job is queued for both atomically with its
// write. The search_outbox queue is drained by cmd/search-drain, which builds and
// pushes documents into the live index wave by wave — cmd/ingest itself never talks
// to the search engine.
type dbStore struct {
	pool          *pgxpool.Pool
	q             *db.Queries
	targetVersion int32
	crawled       *crawledSet
	tally         *writeTally
	// hydrationWindow is how long a stored row with no description keeps being withheld from
	// the seen-set so a hydrating adapter re-attempts its detail fetch. See ExistingExternalIDs
	// and cmd/ingest's hydrationRetryWindowFor.
	hydrationWindow time.Duration
	// refetchAll empties the seen-set for the whole run, so a hydrating adapter treats every
	// listed posting as new and the pipeline re-WRITES it instead of only refreshing its
	// liveness. See ExistingExternalIDs.
	refetchAll bool
}

// dbStore is the only non-test implementation of pipeline.Store, and it is expected to carry
// all three optional capabilities — the pipeline discovers them by type assertion and
// silently degrades on a miss, which is right for a test fake and wrong for this one. A
// dropped Touch stops refreshing a re-listed posting's last_seen_at, and the 48h unseen sweep
// then closes live jobs. These state that expectation to the compiler, the way board_health.go
// already does for the one port that was exported.
var (
	_ pipeline.Store      = (*dbStore)(nil)
	_ pipeline.FormSaver  = (*dbStore)(nil)
	_ pipeline.Closer     = (*dbStore)(nil)
	_ pipeline.Toucher    = (*dbStore)(nil)
	_ pipeline.SeenLookup = (*dbStore)(nil)
)

func newDBStore(pool *pgxpool.Pool, targetVersion int, crawled *crawledSet, tally *writeTally, hydrationWindow time.Duration, refetchAll bool) *dbStore {
	return &dbStore{
		pool:            pool,
		q:               db.New(pool),
		targetVersion:   int32(targetVersion),
		crawled:         crawled,
		tally:           tally,
		hydrationWindow: hydrationWindow,
		refetchAll:      refetchAll,
	}
}

// written is what save needs back from whichever of the two writes ran, so the rest of the
// function does not care which did.
//
// row is the persisted job, and it is present ONLY when the full upsert ran. The cheap write
// returns four narrow columns on purpose — embedding the row would detoast the description and
// ship the semantic vector back for every re-seen posting, which is read amplification on the
// path built to remove write amplification. Nothing on the cheap branch needs more: the fuller
// row is read only to BUILD a search document, and needsIndex gates that to the full branch. A
// pointer rather than a zeroed struct so a mistaken read panics instead of quietly serving "".
type written struct {
	id          int64
	source      string
	companySlug string
	duplicateOf pgtype.Int8
	inserted    bool
	changed     bool
	row         *db.Job
}

func fullWrite(r db.UpsertJobRow) written {
	return written{
		id:          r.Job.ID,
		source:      r.Job.Source,
		companySlug: r.Job.CompanySlug,
		duplicateOf: r.Job.DuplicateOf,
		inserted:    r.Inserted.Bool,
		changed:     r.Changed,
		row:         &r.Job,
	}
}

// cheapWrite leaves inserted and changed false, which is what they mean: the row was neither
// created nor edited. Every consumer downstream is already gated on those two.
func cheapWrite(r db.RefreshUnchangedJobRow) written {
	return written{id: r.ID, source: r.Source, companySlug: r.CompanySlug, duplicateOf: r.DuplicateOf}
}

// persist writes the posting the cheapest way that is correct. A re-crawl that matches the
// stored row on RefreshUnchangedJob's key refreshes only last_seen_at, touching no index and
// leaving updated_at alone; everything else — edited content, a closed row reappearing, a
// posting the catalogue does not hold yet — takes the full upsert. Matching no row cannot
// distinguish "changed" from "absent", and does not need to: both want the same statement.
func persist(ctx context.Context, qtx *db.Queries, p db.UpsertJobParams) (written, error) {
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
	})
	if err == nil {
		return cheapWrite(cheap), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return written{}, fmt.Errorf("refresh unchanged job: %w", err)
	}

	full, err := qtx.UpsertJob(ctx, p)
	if err != nil {
		return written{}, fmt.Errorf("upsert job: %w", err)
	}
	return fullWrite(full), nil
}

// needsIndex reports whether a persisted write changed what search would show: a
// new row, or one whose indexed content (content_hash) changed. A re-ingest that
// only refreshed bookkeeping (last_seen_at) reports neither and is skipped.
// Changed is already true on insert (a NULL prior hash is DISTINCT FROM any value);
// Inserted is OR-ed in to keep the "new or changed" intent explicit.
func needsIndex(w written) bool {
	return w.changed || w.inserted
}

// tookCheapWrite reports which of the two writes produced this. The narrow write is the only
// one that carries no row, so this is exact rather than a heuristic.
func (w written) tookCheapWrite() bool { return w.row == nil }

// clustersByRole reports whether a persisted write is worth asking the role-cluster
// question about — the gate in front of jobdedup.CanonicalForRole.
//
// It is a cost gate, not a correctness one: the lookup is a range scan on
// jobs_open_role_cluster_idx, but a full crawl replays millions of rows per pass against
// a database that is already the fleet's I/O bottleneck, so which writes pay for it is a
// real choice. The batch recompute remains the reconciler for whatever this skips.
//
// Only a genuinely new posting is asked about. A per-city fan-out arrives as inserts, so
// this catches it at the moment it happens, and a full pass then pays one lookup per new
// vacancy — thousands — instead of one per row re-seen, which is nearly the whole
// catalogue. What it deliberately leaves to the batch recompute is the posting that turns
// into a duplicate later, when an edit makes its title and description match a sibling's:
// rarer than the fan-out, and no worse off than before this existed.
//
// A row that already carries a marker knows its own answer and is not re-asked.
func clustersByRole(w written) bool {
	return w.inserted && !w.duplicateOf.Valid
}

// Save persists a posting whose adapter yielded no application form — every provider but
// Recruitee.
func (s *dbStore) Save(ctx context.Context, j job.Job) error {
	return s.save(ctx, j, nil)
}

// SaveWithApplyForm persists a posting together with the application form its adapter
// yielded, in ONE transaction. Separated from Save only at the seam: a form written after
// a committed job could be lost to a crash, and nothing would ever queue it — the capture
// queue is for the providers whose form costs a request, and Recruitee is not one of them,
// so this write is the only chance its form gets.
func (s *dbStore) SaveWithApplyForm(ctx context.Context, j job.Job, form applyform.Form) error {
	return s.save(ctx, j, &form)
}

func (s *dbStore) save(ctx context.Context, j job.Job, form *applyform.Form) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after Commit is a no-op; suppress its error (matches the other stores).
	defer func() { _ = tx.Rollback(ctx) }()

	// The aggregate's read projection carries every persistable field; the write path
	// never touches enrichment (SetJobEnrichment owns those columns), so it is not mapped.
	// The mapping also stamps content_hash — which the upsert reports back as
	// inserted/changed, driving the incremental index push below — and role_fingerprint.
	params := j.Fields().UpsertParams()

	qtx := s.q.WithTx(tx)
	saved, err := persist(ctx, qtx, params)
	if err != nil {
		return err
	}

	// A company that posts one role once per city sends it as many board identities, all
	// sharing a role_fingerprint. Ask — after the upsert, because the answer depends on
	// the written row's id — whether an older canonical copy already exists, and point
	// this row at it. Without this the copies stay separate searchable jobs until the
	// next batch recompute hours later, and every one of them reaches search, the
	// subscription digests, and the company's job count as a vacancy of its own.
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

	// The form belongs to the posting whether or not the posting is a searchable canon: a
	// repost is still a job somebody can apply to, and its form is what applying costs.
	// This is the opposite call from the enrichment enqueue below, and deliberately so —
	// that one spends an LLM budget on an invisible row, this one writes bytes we already
	// hold.
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

	// A duplicate never reaches search, so enriching it pays an LLM for an invisible row.
	if !deduped {
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion: s.targetVersion,
			JobID:         saved.id,
		}); err != nil {
			return fmt.Errorf("enqueue enrichment: %w", err)
		}
	}

	// Queue this write for the live search index, atomically with the row it indexes,
	// only when the write inserted or changed indexed content and the row is a
	// searchable canon (mirrors the enrichment gate above). cmd/search-drain builds
	// and pushes the document later, in a batch with whatever else queued around the
	// same time — cmd/ingest never talks to the search engine directly. Unconditional
	// on whether search is configured anywhere: the row is cheap, and an unconfigured
	// deployment simply never drains it.
	if needsIndex(saved) && !deduped && !saved.duplicateOf.Valid {
		if err := qtx.EnqueueSearchOutbox(ctx, saved.id); err != nil {
			return fmt.Errorf("enqueue search outbox: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Queue a form capture for a provider whose form costs its own request — AFTER the
	// commit, and never fatal.
	//
	// After, because a failed statement aborts the surrounding transaction: inside it, a
	// hiccup on this queue would discard the posting itself, and failing a crawl over a
	// field nothing yet reads is the wrong trade. The gap that opens — a crash between the
	// commit and this line — closes by itself, because the enqueue is gated on the job
	// having no stored form rather than on it being new: the next crawl of the board
	// queues it again.
	if applyform.NeedsRequestCapture(saved.source) {
		if _, err := s.q.EnqueueApplyFormCapture(ctx, saved.id); err != nil {
			log.Printf("ingest: queue apply-form capture for job %d: %v", saved.id, err)
		}
	}

	// Queue a full-description fetch for an Adzuna posting whose stored URL is Adzuna's
	// own hosted job page — the API gives only a ~500-character snippet. Same after-
	// commit, never-fatal, gated-on-not-done-yet shape as the apply-form capture above;
	// see adzunadesc.Eligible for why only a subset of Adzuna's own URLs qualify.
	if adzunadesc.Eligible(saved.source, params.URL) {
		if _, err := s.q.EnqueueAdzunaDescriptionCapture(ctx, saved.id); err != nil {
			log.Printf("ingest: queue adzuna description capture for job %d: %v", saved.id, err)
		}
	}

	// Record this (provider, company) as crawled so the post-run stale sweep only
	// closes jobs of companies this run actually wrote — never a provider's whole
	// catalogue when a run crawled only some of its boards. Uses the persisted row so
	// aggregator sources (one board, per-job companies) scope by their real companies.
	if s.crawled != nil {
		s.crawled.record(saved.source, saved.companySlug)
	}

	// Which write this posting took, counted after the commit so a rolled-back attempt is not
	// reported as a saving. The run-end share is the only thing that would show a provider whose
	// rows never match the cheap key — see writeTally.
	s.tally.record(saved.source, saved.tookCheapWrite())

	return nil
}

// Close soft-closes a posting by its (source, external_id) identity — the stream-driven
// close path a self-closing source (jobtech) uses for ads its incremental feed reports
// removed. Idempotent (the query no-ops on an already-closed or absent row), so a re-sent
// removal in the trailing window costs nothing.
func (s *dbStore) Close(ctx context.Context, source, externalID string) error {
	if _, err := s.q.CloseJobBySourceExternalID(ctx, db.CloseJobBySourceExternalIDParams{
		Source:     source,
		ExternalID: externalID,
	}); err != nil {
		return fmt.Errorf("close job %s/%s: %w", source, externalID, err)
	}
	return nil
}

// ExistingExternalIDs returns the set of external_ids already stored for the board about to be
// crawled — the pipeline's seen-set for a hydrating source, so per-posting detail is fetched only
// for postings the catalogue lacks. Implements pipeline.seenLookup.
//
// The lookup runs once per board, so a board-bearing provider reads only its own namespace: on
// workday the provider-wide query returns 1.27M ids in ~168s against ~1.8s for a board's 25k. A
// boardless adapter has no namespace to scope by and reads the provider's whole set, which is what
// its single "board" means anyway.
//
// Each id maps to its row's tech evidence (is_tech IS TRUE), which the refresh path judges the
// catalogue filter on — a re-listed posting carries no description to derive it from.
//
// A row still without a description is withheld from the set until it is pipeline.
// HydrationRetryWindow old, so the crawl re-attempts the detail fetch its first pass lost rather
// than treating the body-less row as finished.
//
// refetchAll withholds EVERY row, which is the repair path for an adapter fix that changes what
// the LISTING yields (a mis-read remote flag, a location built from the wrong place). Such a fix
// reaches new postings on the next crawl and reaches stored ones never: a re-listed posting takes
// the refresh path, which by design rewrites no content. Emptying the set puts the provider's
// whole catalogue back through the ordinary write, so nothing about the repair is a second code
// path that could disagree with the crawl.
func (s *dbStore) ExistingExternalIDs(ctx context.Context, source, board string) (map[string]bool, error) {
	set := map[string]bool{}
	if s.refetchAll {
		return set, nil
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-s.hydrationWindow), Valid: true}
	if board == "" {
		rows, err := s.q.ExistingExternalIDs(ctx, db.ExistingExternalIDsParams{
			Source:          source,
			HydrationCutoff: cutoff,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
		}
		return set, nil
	}
	// The pattern is escaped by externalid.BoardPattern: a board name may contain LIKE syntax,
	// and a third of the workday board names carry an underscore.
	rows, err := s.q.ExistingExternalIDsByBoard(ctx, db.ExistingExternalIDsByBoardParams{
		Source:          source,
		Pattern:         externalid.BoardPattern(board),
		HydrationCutoff: cutoff,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
	}
	return set, nil
}

// Touch refreshes an already-ingested posting's liveness (last_seen_at, reopen if closed) by
// its (source, external_id) identity, without rewriting content — the path a HydratingSource
// (justjoin) uses for an offer it re-listed but did not re-fetch, so its hydrated description
// and facets are preserved. Implements pipeline.toucher.
func (s *dbStore) Touch(ctx context.Context, source, externalID string) error {
	companySlug, err := s.q.TouchJob(ctx, db.TouchJobParams{
		Source:     source,
		ExternalID: externalID,
	})
	if err != nil {
		return fmt.Errorf("touch job %s/%s: %w", source, externalID, err)
	}
	// Record the company as crawled, exactly as Save does, so the post-run stale sweep keeps
	// this company in scope. Otherwise a company whose offers were all touched (none newly
	// saved) would fall out of the crawled-set and its removed offers would never close.
	if s.crawled != nil {
		s.crawled.record(source, companySlug)
	}
	// A touch IS the cheap write for a hydrating source: TouchJob refreshes liveness and writes
	// no content, the same bargain RefreshUnchangedJob makes on the board path. Counting it keeps
	// the per-provider share meaning one thing for both source shapes. Leaving it out would be
	// worse than a gap — a hydrating provider re-lists through here and only NEW offers reach
	// save(), so the run would report 0% and read as the very churn the line exists to expose.
	s.tally.record(source, true)
	return nil
}

// CanonicalCompanySlugs implements pipeline.AliasLookup: for a batch of folded company slugs,
// the canonical slug a merge elected for each employer that has one.
//
// Read rather than written here, like ExistingExternalIDs — the store already owns the pool,
// and the pipeline asks for this once per board run, not once per posting.
func (s *dbStore) CanonicalCompanySlugs(ctx context.Context, foldedKeys []string) (map[string]string, error) {
	rows, err := s.q.ResolveCompanySlugAliases(ctx, foldedKeys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		// One canonical slug per folded key is the writer's invariant, not the schema's (see
		// the query). Rows arrive ordered, so the first wins deterministically; a second,
		// different one means two waves elected different winners for one employer, which
		// would otherwise show up only as a company that keeps changing url.
		if prev, ok := out[r.FoldedKey]; ok {
			if prev != r.CanonicalSlug {
				log.Printf("ingest: company alias conflict: folded key %q maps to both %q and %q; using %q",
					r.FoldedKey, prev, r.CanonicalSlug, prev)
			}
			continue
		}
		out[r.FoldedKey] = r.CanonicalSlug
	}
	return out, nil
}
