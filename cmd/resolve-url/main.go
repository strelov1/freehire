// Command resolve-url ingests individual job postings by URL. It is the on-demand
// counterpart to the board crawlers: some vacancies live only as a single detail page
// that no board feed enumerates (a Teamtailor custom-domain site with an empty listing,
// a Breezy private-link posting), so a board entry cannot reach them. resolve-url takes
// one or more job URLs (as arguments or on stdin), resolves each through the same
// internal/linksource registry the Telegram link-following uses — the per-ATS adapters
// first (greenhouse/ashby/lever/workable/... read the platform's public API), then a
// last-resort generic resolver that maps any page carrying a schema.org JobPosting
// ld+json block — and upserts each resolved job through the canonical UpsertJob (+
// enrichment enqueue), exactly as ingest and tg-extract do.
//
// Run once and exit (an operator tool, not a cron worker): needs DATABASE_URL.
//
//	go run ./cmd/resolve-url https://careers.vairix.com/jobs/605143-... https://tekton-labs.breezy.hr/p/...
package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/linksource"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

func main() { worker.Main(run) }

func run() int {
	urls := readURLs()
	if len(urls) == 0 {
		log.Print("resolve-url: no URLs given (pass them as arguments or on stdin, one per line)")
		return 1
	}

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)

	// Push each newly-written job straight to the live search index when the engine is
	// configured (MEILI_MASTER_KEY set), mirroring cmd/ingest — the company page and
	// /jobs search read from Meili, so without this a resolved job stays invisible there
	// until the next full reindex. Best-effort: an index failure never fails the write,
	// and the batch reindex stays the reconciler. Absent the key, nil skips the push.
	var idx *search.Client
	if cfg.MeiliKey != "" {
		idx = search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	}

	// The generic resolver is appended AFTER the host-scoped adapters (so a known ATS is
	// handled by its richer API adapter) and only here, never in the shared registry —
	// its always-true Match must not leak into the Telegram crawl.
	client := sources.NewClient()
	reg := append(linksource.All(client), linksource.NewGeneric(client))

	resolved, err := linksource.ResolveLinks(ctx, reg, urls)
	if err != nil {
		// Non-nil only when links matched but all failed — a transient outcome.
		log.Printf("resolve-url: %v", err)
		return 1
	}
	if len(resolved) == 0 {
		log.Printf("resolve-url: none of the %d URL(s) resolved to a vacancy", len(urls))
		return 1
	}

	var saved, failed int
	for _, r := range resolved {
		if err := upsert(ctx, pool, q, idx, r); err != nil {
			failed++
			log.Printf("resolve-url: %s/%s: %v", r.Source, r.Job.ExternalID, err)
			continue
		}
		saved++
		log.Printf("resolve-url: saved %s — %q at %s", r.Source, r.Job.Title, r.Job.Company)
	}
	log.Printf("resolve-url: done — %d saved, %d failed, %d of %d URL(s) resolved",
		saved, failed, len(resolved), len(urls))
	if failed > 0 {
		return 1
	}
	return 0
}

// upsert writes one resolved job through the Job aggregate factory and the canonical
// UpsertJob, enqueuing it for enrichment in the same transaction — the same write path as
// ingest and tg-extract, so facets, slugs and the enrichment outbox stay consistent.
func upsert(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, idx *search.Client, r linksource.Resolved) error {
	j, err := job.New(job.Draft{
		Source:      r.Source,
		ExternalID:  r.Job.ExternalID,
		URL:         r.Job.URL,
		Title:       r.Job.Title,
		Company:     r.Job.Company,
		Location:    r.Job.Location,
		Remote:      r.Job.Remote,
		Description: r.Job.Description,
		WorkMode:    r.Job.WorkMode,
	})
	if err != nil {
		return err
	}
	params := j.Fields().UpsertParams()
	if r.Job.PostedAt != nil {
		params.PostedAt = pgtype.Timestamptz{Time: *r.Job.PostedAt, Valid: true}
	}
	params.RoleFingerprint = pgtype.Text{String: jobhash.RoleFingerprint(params), Valid: true}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)
	res, err := qtx.UpsertJob(ctx, params)
	if err != nil {
		return err
	}
	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		TargetVersion:     int32(enrich.Version),
		JobID:             res.Job.ID,
		ExcludeCategories: enrich.NonTechCategories,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	indexJob(ctx, q, idx, res)
	return nil
}

// indexJob pushes a just-written open job to the live search index, best-effort — same doc
// build as cmd/ingest, so the company page and /jobs search show it immediately instead of
// waiting for the next full reindex. Unlike ingest it does NOT gate on Inserted/Changed:
// this is an operator tool over a handful of explicit URLs, so a re-run must (re)index an
// already-present posting rather than silently no-op on an unchanged hash. The Meili upsert
// is idempotent. A build or push failure is logged and swallowed — the job is already
// persisted and the batch reindex reconciles. A non-canonical repost or a closed job is
// never made searchable, matching ingest.
func indexJob(ctx context.Context, q *db.Queries, idx *search.Client, saved db.UpsertJobRow) {
	if idx == nil || saved.Job.DuplicateOf.Valid || saved.Job.ClosedAt.Valid {
		return
	}
	// The job-reality signal needs this role's cluster counts; a lookup failure degrades
	// to a unique role (counts 1) rather than skipping the push.
	repost, mass := int64(1), int64(1)
	if c, err := q.RoleClusterCount(ctx, db.RoleClusterCountParams{
		CompanySlug:     saved.Job.CompanySlug,
		RoleFingerprint: saved.Job.RoleFingerprint,
	}); err == nil {
		repost, mass = c.RepostCount, c.MassCount
	}
	doc, err := search.FromJob(saved.Job)
	if err != nil {
		log.Printf("resolve-url: build index doc for job %d: %v", saved.Job.ID, err)
		return
	}
	reality := jobview.ClassifyReality(saved.Job, time.Now(), int(repost), int(mass))
	doc.Reality = &reality
	if err := idx.SubmitJobs(ctx, []search.JobDocument{doc}); err != nil {
		log.Printf("resolve-url: index job %d: %v", saved.Job.ID, err)
	}
}

// readURLs collects job URLs from the command line, falling back to stdin (one per line,
// blank lines ignored) so a list can be piped in.
func readURLs() []string {
	var urls []string
	for _, a := range os.Args[1:] {
		if a = strings.TrimSpace(a); a != "" && !strings.HasPrefix(a, "-") {
			urls = append(urls, a)
		}
	}
	if len(urls) > 0 {
		return urls
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			urls = append(urls, line)
		}
	}
	return urls
}
