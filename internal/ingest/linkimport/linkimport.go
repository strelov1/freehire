// Package linkimport imports one vacancy into the catalog from its page URL. Some
// postings live only as a single detail page that no board feed enumerates (a Teamtailor
// custom-domain site with an empty listing, a Breezy private-link posting), so a board
// entry cannot reach them — but a link to the page can.
//
// It is the on-demand counterpart to the board crawlers, and the shared engine behind
// both surfaces that offer it: cmd/resolve-url (an operator with a list of URLs) and the
// browser extension's "add this page" (a signed-in user standing on a vacancy we lack).
// Keeping it in one package keeps one definition of the import: the registry order, the
// canonical write path, the enrichment enqueue, and the search push.
package linkimport

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// Result identifies the posting an import wrote, under the destination's own catalog
// identity — so a later crawl of the same posting from its board dedups onto this row.
type Result struct {
	Source     string
	ExternalID string
	PublicSlug string
	// CompanySlug is the employer the posting was filed under, empty when the page named no
	// company. The intake asks the catalog about it to tell a genuinely new employer from one
	// we already carry under another ATS.
	CompanySlug string
	// Deduped reports that the catalog already carried this vacancy, so the row just written
	// was marked a duplicate of it and PublicSlug names the CANONICAL posting rather than the
	// row this import wrote. The row is written rather than skipped because it is what makes
	// the submitted URL resolvable at all: FindOpenJobByURL matches duplicates and answers
	// with the posting they duplicate.
	Deduped bool
}

// BoardResolver detects the ATS board embedded in a page whose host says nothing — a company
// careers site on its own domain. internal/ingest/boardresolve satisfies it. Optional: a nil one
// simply drops the vanity-domain fallback.
type BoardResolver interface {
	Resolve(ctx context.Context, rawURL string) (source, board, canonical string, ok bool)
}

// Importer resolves job URLs and writes what it parses. idx may be nil (no search engine
// configured), which skips the index push.
type Importer struct {
	pool *pgxpool.Pool
	q    *db.Queries
	idx  *search.Client
	reg  []linksource.Source
	// ingest is the provider-keyed crawl registry, kept alongside reg because the
	// vanity-domain fallback reaches a board directly rather than through an adapter's Match.
	ingest map[string]sources.Source
	boards BoardResolver
}

// New builds an Importer over the given database and HTTP transport. ingest is the
// provider-keyed ingest registry, which board coverage reuses to read a vacancy on any ATS
// that has a crawler but no single-page adapter; a nil registry disables that step and is
// what tests with a canned page pass. The resolver order itself lives in
// linksource.ImportRegistry, so it is stated once.
func New(pool *pgxpool.Pool, q *db.Queries, idx *search.Client, c linksource.Client, ingest map[string]sources.Source, boards BoardResolver) *Importer {
	return &Importer{
		pool:   pool,
		q:      q,
		idx:    idx,
		reg:    linksource.ImportRegistry(c, ingest),
		ingest: ingest,
		boards: boards,
	}
}

// Board is an ATS board a caller has already resolved for the link it is importing. The intake
// resolves one on every link before importing it, and it can name a board no URL parse would
// reach — a storefront on the company's own domain whose posting id gives away the greenhouse
// board behind it. A zero Board simply means "unknown".
type Board struct {
	Source string
	Board  string
}

func (b Board) known() bool { return b.Source != "" && b.Board != "" }

// Import resolves raw through the registry and writes the vacancy it parses — see Resolve
// for the resolution sequence and Write for the persistence. A non-nil error is a transient
// fetch/parse failure or a write failure.
func (im *Importer) Import(ctx context.Context, raw string, known Board) (Result, bool, error) {
	resolved, ok, err := im.Resolve(ctx, raw, known)
	if err != nil || !ok {
		return Result{}, ok, err
	}
	return im.Write(ctx, resolved)
}

// Resolve runs Import's resolution sequence — host-scoped adapters and board coverage
// first, the caller's already-known board next, a vanity-domain board fetch, and the
// generic JSON-LD fallback last — WITHOUT writing anything. ok=false means the page is not
// a single vacancy we can read (no adapter matched, or the page carries no posting).
//
// This is the seam a caller needs when a generic-fallback match must be treated
// differently from a recognized-ATS match (see internal/ingest/jdresolve, which writes a
// recognized match as a normal public job via Write, but a generic match — an unverified
// third-party scrape — as a private job instead). Import itself does not make that
// distinction: it always writes through Write, so a caller that needs to branch on
// resolved.Source before deciding how to persist must call Resolve directly.
//
// known is the board the caller already resolved, and it only ever overrides the generic
// resolver — the host-scoped adapters and board coverage already resolve a board's own
// identity, and cost less. Generic reads any page with a JobPosting block and reports it
// under (weblink, <the URL>): correct when nothing better is known, but a second row for a
// posting we crawl under (greenhouse, <board>:<id>) when a board IS known. Preferring the
// board there is what keeps a storefront link from duplicating the posting it points at.
func (im *Importer) Resolve(ctx context.Context, raw string, known Board) (linksource.Resolved, bool, error) {
	resolved, err := linksource.ResolveLinks(ctx, im.reg, []string{raw})
	if err != nil {
		return linksource.Resolved{}, false, err
	}
	if len(resolved) == 0 || resolved[0].Source == linksource.GenericSource {
		if r, ok := im.resolveOnKnownBoard(ctx, raw, known); ok {
			return r, true, nil
		}
	}
	if len(resolved) == 0 {
		if r, ok := im.resolveVanityDomain(ctx, raw); ok {
			return r, true, nil
		}
		return linksource.Resolved{}, false, nil
	}
	return resolved[0], true, nil
}

// resolveOnKnownBoard reads the posting from the board the caller named, so it is stored under
// the identity a crawl of that board would give it. A board we cannot crawl (no ingest adapter)
// or a posting absent from it leaves the caller's own resolution in place.
func (im *Importer) resolveOnKnownBoard(ctx context.Context, raw string, known Board) (linksource.Resolved, bool) {
	if !known.known() || im.ingest == nil {
		return linksource.Resolved{}, false
	}
	job, ok, err := linksource.ResolveOnBoard(ctx, im.ingest, known.Source, known.Board, raw)
	if err != nil {
		log.Printf("linkimport: known board %s/%s for %s: %v", known.Source, known.Board, raw, err)
		return linksource.Resolved{}, false
	}
	if !ok {
		return linksource.Resolved{}, false
	}
	return linksource.Resolved{Source: known.Source, Job: job}, true
}

// resolveVanityDomain is the last thing tried before a link is given up on: a company careers
// page on its OWN domain, whose host tells us nothing but whose markup links to the ATS board
// behind it. Fetching the page reveals (source, board), and from there it is an ordinary board
// read.
//
// It cannot be a link-source adapter, because an adapter is chosen by Match — a pure, offline
// predicate — and ResolveLinks commits to the single adapter Find returns. So the page fetch
// is orchestrated here, after every offline resolver has passed.
func (im *Importer) resolveVanityDomain(ctx context.Context, raw string) (linksource.Resolved, bool) {
	if im.boards == nil || im.ingest == nil {
		return linksource.Resolved{}, false
	}
	source, board, _, ok := im.boards.Resolve(ctx, raw)
	if !ok {
		return linksource.Resolved{}, false
	}
	job, ok, err := linksource.ResolveOnBoard(ctx, im.ingest, source, board, raw)
	if err != nil {
		log.Printf("linkimport: board %s/%s for vanity link %s: %v", source, board, raw, err)
		return linksource.Resolved{}, false
	}
	if !ok {
		return linksource.Resolved{}, false
	}
	return linksource.Resolved{Source: source, Job: job}, true
}

// draftFrom maps a resolved vacancy onto the aggregate's draft. The destination
// adapter's posted date rides the draft rather than being written over the mapped
// params, so the derived columns fingerprint the posted_at that is actually stored.
func draftFrom(r linksource.Resolved) job.Draft {
	return job.Draft{
		Input: jobderive.Input{
			Source:      r.Source,
			ExternalID:  r.Job.ExternalID,
			Title:       r.Job.Title,
			Company:     r.Job.Company,
			Location:    r.Job.Location,
			Description: r.Job.Description,
			WorkMode:    r.Job.WorkMode,
		},
		URL:      r.Job.URL,
		Remote:   r.Job.Remote,
		PostedAt: r.Job.PostedAt,
	}
}

// Write persists one resolved job through the Job aggregate factory and the canonical
// UpsertJob, enqueuing it for enrichment in the same transaction — the same write path as
// ingest and tg-extract, so facets, slugs and the enrichment outbox stay consistent.
// Exported so a caller that resolved via Resolve can write on its own terms (see Resolve).
func (im *Importer) Write(ctx context.Context, r linksource.Resolved) (Result, bool, error) {
	j, err := job.New(draftFrom(r))
	if err != nil {
		return Result{}, false, err
	}
	params := j.Fields().UpsertParams()

	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return Result{}, false, err
	}
	defer tx.Rollback(ctx)
	qtx := im.q.WithTx(tx)
	res, err := qtx.UpsertJob(ctx, params)
	if err != nil {
		return Result{}, false, err
	}
	// Asked AFTER the upsert, because the answer depends on the written row's id. Only a
	// URL-keyed generic import can shadow a posting we already crawled: a board identity
	// is deduplicated by UpsertJob's ON CONFLICT (source, external_id) before it gets here.
	canon, deduped := db.CanonicalJobForRoleRow{}, false
	if params.Source == linksource.GenericSource {
		canon, deduped = jobdedup.CanonicalForRole(ctx, qtx, params, res.Job.ID)
	}
	if deduped {
		if _, err := qtx.MarkJobDuplicateOfRole(ctx, db.MarkJobDuplicateOfRoleParams{
			ID:              res.Job.ID,
			DuplicateOfRole: pgtype.Int8{Int64: canon.ID, Valid: true},
		}); err != nil {
			return Result{}, false, err
		}
	} else if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		// A duplicate never reaches search, so enriching it pays an LLM for an invisible row.
		TargetVersion: int32(enrich.Version),
		JobID:         res.Job.ID,
	}); err != nil {
		return Result{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, false, err
	}
	if deduped {
		im.unindex(ctx, res.Job.ID)
	} else {
		im.index(ctx, res)
	}

	out := Result{
		Source:      res.Job.Source,
		ExternalID:  res.Job.ExternalID,
		PublicSlug:  res.Job.PublicSlug,
		CompanySlug: res.Job.CompanySlug,
		Deduped:     deduped,
	}
	if deduped {
		out.PublicSlug = canon.PublicSlug
	}
	return out, true, nil
}

// unindex drops a row that was just demoted to a duplicate from the live index. A row written
// for the first time was never there, but a re-import of a URL written before its canon existed
// was — and skipping the push alone would leave that stale document searchable until the next
// reindex, which is not on a tight schedule. Best-effort, like the push: the marking is
// committed either way and the batch rebuild reconciles.
func (im *Importer) unindex(ctx context.Context, id int64) {
	if im.idx == nil {
		return
	}
	if err := im.idx.SubmitJobDeletion(ctx, []int64{id}); err != nil {
		log.Printf("linkimport: drop duplicate job %d from the index: %v", id, err)
	}
}

// index pushes a just-written open job to the live search index, best-effort — same doc
// build as cmd/ingest, so the company page and /jobs search show it immediately instead of
// waiting for the next full reindex. Unlike ingest it does NOT gate on Inserted/Changed: an
// import is an explicit act on one URL, so a re-import must (re)index an already-present
// posting rather than silently no-op on an unchanged hash. The Meili upsert is idempotent.
// A build or push failure is logged and swallowed — the job is already persisted and the
// batch reindex reconciles. A non-canonical repost, a closed job, one whose category
// neither the title dictionary nor the LLM ever resolved (search.CategoryUnresolved), or one
// with no posting body at all (search.DescriptionMissing) is never made searchable, matching
// cmd/reindex/cmd/search-drain. The category case is common here specifically: an import is a
// fresh URL, so enrichment has usually not run yet — the job becomes searchable once the next
// full reindex re-evaluates it with a category.
func (im *Importer) index(ctx context.Context, saved db.UpsertJobRow) {
	if im.idx == nil || saved.Job.DuplicateOf.Valid || saved.Job.ClosedAt.Valid ||
		search.CategoryUnresolved(saved.Job) || search.DescriptionMissing(saved.Job) {
		return
	}
	// The job-reality signal needs this role's cluster counts; a lookup failure degrades
	// to a unique role (counts 1) rather than skipping the push.
	repost, mass := int64(1), int64(1)
	if c, err := im.q.RoleClusterCount(ctx, db.RoleClusterCountParams{
		CompanySlug:     saved.Job.CompanySlug,
		RoleFingerprint: saved.Job.RoleFingerprint,
	}); err == nil {
		repost, mass = c.RepostCount, c.MassCount
	}
	doc, err := search.FromJob(saved.Job)
	if err != nil {
		log.Printf("linkimport: build index doc for job %d: %v", saved.Job.ID, err)
		return
	}
	reality := jobview.ClassifyReality(saved.Job, time.Now(), int(repost), int(mass))
	doc.Reality = &reality
	// Widen the row with the geography of everything it represents — the push is a
	// field-level document update, so omitting this replaces the reindex's union with this
	// row's own narrow set. Asked unconditionally: there is no cheap "represents nobody"
	// test, and wanting one is what made the old gate default to asking anyway. The wave
	// query takes a slice, so a single row is a one-element one; an empty result means no
	// widening, which is exactly what a row representing nobody should get.
	if rows, err := im.q.DuplicateClosureGeoFor(ctx, []int64{saved.Job.ID}); err != nil {
		log.Printf("linkimport: duplicate-closure geography for job %d: %v", saved.Job.ID, err)
	} else {
		for _, g := range rows {
			doc.MergeClosureGeography(g.Countries, g.Regions, g.Cities)
		}
	}
	if err := im.idx.SubmitJobs(ctx, []search.JobDocument{doc}); err != nil {
		log.Printf("linkimport: index job %d: %v", saved.Job.ID, err)
	}
}
