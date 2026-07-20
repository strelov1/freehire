// Command reindex rebuilds a Meilisearch jobs index from Postgres. It ensures the
// index settings exist, then scans jobs in batches and upserts their documents.
// Run it on a schedule (e.g. cron); it processes the whole table and exits.
// Indexing is idempotent (upsert by id), so re-runs are safe.
//
// Two passes share this binary:
//
//   - default: the facet/keyword index (no embedder) — the fast, always-fresh
//     production search. A full rebuild is minutes, not hours.
//   - reindex --semantic: the hybrid index (adds the in-engine embedder). Slower
//     (it embeds new/changed documents); run on its own, looser schedule and only
//     while semantic search is enabled — it never blocks the facet pass.
//
// A full --semantic rebuild may be scoped to a fresh posting window with
// --posted-within <duration> (the in-engine embedder cannot embed the whole
// catalogue in reasonable time); every other pass scans the whole table.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// reindexBatchSize bounds how many jobs are read from Postgres and pushed to
// Meilisearch per round. Once the facet index dropped the per-document embedder,
// the per-batch round-trip became the throughput lever, so the batch is sized up
// from 500 to amortize it (Postgres read and the ~7KB-doc payload are both cheap
// at this size). A const for now; promote to config if it needs tuning.
const reindexBatchSize = 2000

// progressInterval is how often reindex emits a heartbeat with its running totals.
// A full reindex pushes hundreds of thousands of docs to Meilisearch and otherwise
// logs only on completion, so the heartbeat distinguishes a slow run from a stalled
// one (the totals stop advancing).
const progressInterval = 60 * time.Second

func main() {
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Bootstrap owns config + pool, so this required-config check lands just after
	// the pool opens rather than before it. The connect is cheap and cleanup closes
	// it on this early return, so the only cost of a missing key is one DB handshake.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	q := db.New(pool)

	semantic := semanticRequested(os.Args[1:])
	target := "facet"
	if semantic {
		target = "semantic"
	}

	postedWithin, scoped, err := postedWithinFrom(os.Args[1:])
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}
	// --posted-within scopes the fresh window the semantic swap rebuild embeds; it is
	// meaningless for the facet index (which holds the whole catalogue). Reject that
	// combo loudly rather than silently ignoring the flag.
	if scoped && !semantic {
		log.Print("reindex: --posted-within applies only to a full --semantic rebuild")
		return 1
	}

	// --from-pg rehydrates the semantic index from the vectors already in Postgres
	// (jobs.semantic_embedding) instead of re-embedding via TEI. It only makes sense for
	// the semantic pass — the facet index carries no vectors.
	fromPG := fromPGRequested(os.Args[1:])
	if fromPG && !semantic {
		log.Print("reindex: --from-pg applies only to a --semantic rebuild")
		return 1
	}

	// Refresh the role-cluster canonical markers before reading jobs, so the collapse
	// (splitJobs drops non-canonical reposts) reflects the current catalogue and a closed
	// canon has failed over. Done per company in short transactions (never a table-wide
	// lock that would stall ingest). Best-effort: a hiccup here must not block the reindex
	// (which also owns settings/compaction), so it degrades to the prior markers.
	if n, err := recomputeRoleDuplicates(ctx, q); err != nil {
		log.Printf("reindex: recompute role duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: recomputed role duplicates (%d rows re-marked)", n)
	}

	// Then suppress aggregator postings that duplicate a first-party ATS posting, so the
	// aggregator copy drops out of this rebuild (and out of embedding/enrichment). Run
	// AFTER the role recompute so ATS reposts have collapsed to their canon first. Same
	// per-company, best-effort discipline as the role pass.
	if n, err := suppressAggregatorDuplicates(ctx, q); err != nil {
		log.Printf("reindex: suppress aggregator duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: suppressed aggregator duplicates (%d rows re-marked)", n)
	}

	var b rebuilder = client.NewFacetRebuild()
	if semantic {
		b = client.NewSemanticRebuild()
		if fromPG {
			b = client.NewSemanticRebuildFromPG()
		}
	}
	// The semantic rebuild optionally scopes to a fresh posting window (--posted-within);
	// every other full pass scans the whole table. A scoped reader returns open jobs only,
	// which the swap rebuild wants anyway (closed jobs are simply absent).
	reader := worker.NewFullScanReader(q)
	scope := "full"
	if scoped {
		reader = worker.NewPostedSinceReader(q, time.Now().Add(-postedWithin))
		scope = "posted-within " + postedWithin.String()
	}
	vectors := "n/a"
	if semantic {
		if vectors = "tei"; fromPG {
			vectors = "pg"
		}
	}
	log.Printf("reindex: target=%s scope=%s mode=swap vectors=%s", target, scope, vectors)
	lookup, err := buildRealityLookup(ctx, q)
	if err != nil {
		log.Printf("reindex: build reality lookup: %v", err)
		return 1
	}
	geo, err := buildClusterGeoLookup(ctx, q)
	if err != nil {
		log.Printf("reindex: build cluster geo lookup: %v", err)
		return 1
	}
	indexed, skipped, err := reindexFull(ctx, reader, b, lookup, geo, time.Now())
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}
	log.Printf("reindex done: target=%s scope=full indexed=%d skipped=%d", target, indexed, skipped)
	return 0
}

// postedWithinFrom parses an optional --posted-within <duration> / --posted-within=<duration>
// flag (e.g. "168h" for 7 days). It scopes a full --semantic rebuild to jobs posted
// within that window, since the in-engine embedder cannot embed the whole catalogue in
// reasonable time. Reports (duration, true, nil) when present, (0, false, nil) when
// absent, and an error for a missing or unparseable value.
func postedWithinFrom(args []string) (time.Duration, bool, error) {
	for i, a := range args {
		var raw string
		switch {
		case a == "--posted-within":
			if i+1 >= len(args) {
				return 0, false, fmt.Errorf("--posted-within needs a duration (e.g. 168h)")
			}
			raw = args[i+1]
		case strings.HasPrefix(a, "--posted-within="):
			raw = strings.TrimPrefix(a, "--posted-within=")
		default:
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, false, fmt.Errorf("--posted-within %q: %w", raw, err)
		}
		if d <= 0 {
			return 0, false, fmt.Errorf("--posted-within must be positive, got %q", raw)
		}
		return d, true, nil
	}
	return 0, false, nil
}

// semanticRequested reports whether the args ask for the hybrid (embedder) pass.
func semanticRequested(args []string) bool {
	for _, a := range args {
		if a == "--semantic" || a == "semantic" {
			return true
		}
	}
	return false
}

// fromPGRequested reports whether a --semantic rebuild should rehydrate from the
// vectors persisted in Postgres (jobs.semantic_embedding) instead of re-embedding via
// TEI — the fast disaster-recovery path that costs no model calls.
func fromPGRequested(args []string) bool {
	for _, a := range args {
		if a == "--from-pg" {
			return true
		}
	}
	return false
}

// rebuilder builds a brand-new index out of band and atomically swaps it into
// production. A full reindex uses it instead of mutating the live index in place:
// Prepare creates a fresh, empty rebuild index; Push streams document batches into
// it WITHOUT waiting per batch (so Meilisearch auto-batches them — the throughput
// lever); Promote waits for the pushes to finish, swaps the rebuild index over the
// live one in a single atomic step, and drops the old one. Search keeps serving the
// old index untouched until the swap, and merges stay cheap because the rebuild
// index grows from empty rather than re-merging into a full one.
type rebuilder interface {
	Prepare(ctx context.Context) error
	Push(ctx context.Context, docs []search.JobDocument) error
	Promote(ctx context.Context) error
}

// reindexFull rebuilds the index from scratch and swaps it in. It streams ONLY
// open jobs into the fresh index — closed jobs are simply absent (the rebuild
// index never held them, so unlike the in-place path there is nothing to delete).
// fetch pages by keyset (id > last seen) so rows inserted or re-ordered during the
// run cannot be skipped or repeated.
func reindexFull(ctx context.Context, reader worker.PageReader, b rebuilder, lookup realityLookup, geo clusterGeoLookup, now time.Time) (int, int, error) {
	if err := b.Prepare(ctx); err != nil {
		return 0, 0, err
	}

	var indexed atomic.Int64
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		log.Printf("reindex: progress indexed=%d", indexed.Load())
	})
	defer stopHeartbeat()

	var afterID int64
	var skipped int
	for {
		jobs, lastID, corrupted, err := worker.ResilientPage(ctx, reader, afterID, reindexBatchSize)
		if err != nil {
			return int(indexed.Load()), skipped, err
		}
		skipped += len(corrupted)

		if len(jobs) > 0 {
			docs, _, err := splitJobs(jobs, lookup, geo, now) // closed jobs (deleteIDs) are dropped, not indexed
			if err != nil {
				return int(indexed.Load()), skipped, err
			}
			if err := b.Push(ctx, docs); err != nil {
				return int(indexed.Load()), skipped, err
			}
			indexed.Add(int64(len(docs)))
		}

		// Keyset progress is the exhaustion signal: ResilientPage advances lastID
		// past a skipped (corrupted) row, so a short page from the degrade path does
		// not end the scan early the way a "< batchSize" check would.
		if lastID == afterID {
			break
		}
		afterID = lastID
	}

	if err := b.Promote(ctx); err != nil {
		return int(indexed.Load()), skipped, err
	}
	return int(indexed.Load()), skipped, nil
}

// realityLookup returns a role cluster's repost and concurrent-open counts for the
// job-reality signal. A miss (a role not in the precomputed map, i.e. a singleton)
// yields (1, 1) — a unique, non-reposted role. A nil lookup means the counts default
// to (1, 1) everywhere (used by tests that do not exercise clustering).
type realityLookup func(companySlug, fingerprint string) (repost, mass int)

// clusterGeoLookup returns the union of a role cluster's countries, regions, and cities
// across its open rows, so the canon's search document can be widened beyond its own
// geography. A miss (singleton cluster) yields nil slices — a no-op widening. A nil
// lookup skips widening entirely (tests that do not exercise clustering).
type clusterGeoLookup func(companySlug, fingerprint string) (countries, regions, cities []string)

// recomputeRoleDuplicates refreshes jobs.duplicate_of one company at a time, returning
// the total rows re-marked. Scoping each UPDATE to a single company keeps its lock
// window to that company's rows for a moment, so the pass never holds the table-wide
// lock that would stall concurrent ingest crawls. A per-company failure aborts (the
// caller treats the whole pass as best-effort and continues with the prior markers).
func recomputeRoleDuplicates(ctx context.Context, q *db.Queries) (int64, error) {
	companies, err := q.CompaniesWithRoleClusters(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	var failures int
	var lastErr error
	for _, c := range companies {
		// Companies are independent, so one failure (e.g. a statement timeout on an
		// unusually large cluster) must not starve the rest — log-and-continue.
		n, err := q.RecomputeRoleDuplicatesForCompany(ctx, c)
		if err != nil {
			failures++
			lastErr = fmt.Errorf("company %q: %w", c, err)
			continue
		}
		total += n
	}
	if failures > 0 {
		return total, fmt.Errorf("%d/%d companies failed; last: %w", failures, len(companies), lastErr)
	}
	return total, nil
}

// suppressAggregatorDuplicates marks each open aggregator posting that duplicates a
// first-party ATS posting (same company, normalized title, compatible country) as a
// duplicate of that ATS row, one company at a time. Returns the total rows re-marked.
// The aggregator set comes from the source registry's aggregator() markers. Best-effort
// and lock-scoped exactly like recomputeRoleDuplicates: a per-company failure is logged
// and skipped so it never starves the rest or blocks the reindex.
func suppressAggregatorDuplicates(ctx context.Context, q *db.Queries) (int64, error) {
	// The aggregator set comes from the registry markers. usajobs is the one adapter
	// sources.All only registers when USAJOBS_API_KEY is set, so a reindex without that
	// key classifies existing usajobs rows as ATS. That is harmless here: federal postings
	// have no corporate ATS twin, so they are never suppressed either way and would only
	// ever be a target on an (essentially impossible) exact company+title+country collision.
	aggregators := sources.AggregatorProviders(sources.All(nil))
	companies, err := q.CompaniesWithAggregatorPostings(ctx, aggregators)
	if err != nil {
		return 0, err
	}
	var total int64
	var failures int
	var lastErr error
	for _, c := range companies {
		n, err := q.SuppressAggregatorDuplicatesForCompany(ctx, db.SuppressAggregatorDuplicatesForCompanyParams{
			Company:     c,
			Aggregators: aggregators,
		})
		if err != nil {
			failures++
			lastErr = fmt.Errorf("company %q: %w", c, err)
			continue
		}
		total += n
	}
	if failures > 0 {
		return total, fmt.Errorf("%d/%d companies failed; last: %w", failures, len(companies), lastErr)
	}
	return total, nil
}

// buildRealityLookup precomputes the whole-catalogue role-cluster counts once, so the
// per-job classification during the rebuild is a map read, not N queries.
func buildRealityLookup(ctx context.Context, q *db.Queries) (realityLookup, error) {
	rows, err := q.RoleClusterCountsAll(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string][2]int, len(rows))
	for _, r := range rows {
		m[r.CompanySlug+"\x00"+r.RoleFingerprint.String] = [2]int{int(r.RepostCount), int(r.MassCount)}
	}
	return func(cs, fp string) (int, int) {
		if v, ok := m[cs+"\x00"+fp]; ok {
			return v[0], v[1]
		}
		return 1, 1
	}, nil
}

// buildClusterGeoLookup precomputes the whole-catalogue role-cluster geography union once
// (RoleClusterGeoAll returns only open multi-row clusters), so widening each canon during
// the rebuild is a map read, not N queries. A singleton cluster is absent from the map and
// resolves to nil slices — a no-op widening.
func buildClusterGeoLookup(ctx context.Context, q *db.Queries) (clusterGeoLookup, error) {
	rows, err := q.RoleClusterGeoAll(ctx)
	if err != nil {
		return nil, err
	}
	type geo struct{ countries, regions, cities []string }
	m := make(map[string]geo, len(rows))
	for _, r := range rows {
		m[r.CompanySlug+"\x00"+r.RoleFingerprint.String] = geo{r.Countries, r.Regions, r.Cities}
	}
	return func(cs, fp string) ([]string, []string, []string) {
		g := m[cs+"\x00"+fp]
		return g.countries, g.regions, g.cities
	}, nil
}

// splitJobs partitions a batch from the (deliberately unfiltered) reindex feed:
// open jobs become index documents (each carrying its reality signal, classified
// against `now` and its cluster counts), closed jobs become deletions so they leave
// the index (the index contains only open jobs — see the job-search spec).
func splitJobs(jobs []db.Job, lookup realityLookup, geo clusterGeoLookup, now time.Time) ([]search.JobDocument, []int64, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	deleteIDs := make([]int64, 0, len(jobs))
	for _, j := range jobs {
		// A closed job or a non-canonical repost (duplicate_of set) leaves the index:
		// only the open canonical row of each role cluster is searchable. Deleting (not
		// just skipping) removes a row that was indexed before it was closed or demoted.
		if j.ClosedAt.Valid || j.DuplicateOf.Valid {
			deleteIDs = append(deleteIDs, j.ID)
			continue
		}
		repost, mass := 1, 1
		if lookup != nil {
			repost, mass = lookup(j.CompanySlug, j.RoleFingerprint.String)
		}
		doc, err := search.FromJob(j)
		if err != nil {
			return nil, nil, err
		}
		reality := jobview.ClassifyReality(j, now, repost, mass)
		doc.Reality = &reality
		// Widen the canon's geography with its cluster's union, so a collapsed
		// multi-country role stays findable by every country its reposts hold. A miss
		// (singleton cluster or no lookup) leaves the canon's own geography untouched.
		if geo != nil {
			doc.MergeClusterGeography(geo(j.CompanySlug, j.RoleFingerprint.String))
		}
		docs = append(docs, doc)
	}
	return docs, deleteIDs, nil
}
