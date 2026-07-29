// Command resolve-url ingests individual job postings by URL. It is the operator-facing
// surface of internal/linkimport: some vacancies live only as a single detail page that no
// board feed enumerates (a Teamtailor custom-domain site with an empty listing, a Breezy
// private-link posting), so a board entry cannot reach them. resolve-url takes one or more
// job URLs (as arguments or on stdin) and imports each — the per-ATS adapters first
// (greenhouse/ashby/lever/workable/... read the platform's public API), then a last-resort
// generic resolver that maps any page carrying a schema.org JobPosting ld+json block —
// through the canonical UpsertJob (+ enrichment enqueue), exactly as ingest does.
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

	"github.com/strelov1/freehire/internal/boardresolve"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/linkimport"
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
	// until the next full reindex. Best-effort inside the importer; absent the key, nil
	// skips the push.
	var idx *search.Client
	if cfg.MeiliKey != "" {
		idx = search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	}

	// One SSRF-guarded client backs both the single-page resolvers and the ingest registry
	// board coverage falls back to, so a resolve and a crawl share transport and rate limits.
	ingestClient := sources.NewClient()
	im := linkimport.New(pool, q, idx, ingestClient, sources.All(ingestClient), boardresolve.New())

	var saved, skipped, failed int
	for _, raw := range urls {
		// No board hint: this worker takes bare URLs, with no intake to have resolved one.
		res, ok, err := im.Import(ctx, raw, linkimport.Board{})
		switch {
		case err != nil:
			failed++
			log.Printf("resolve-url: %s: %v", raw, err)
		case !ok:
			skipped++
			log.Printf("resolve-url: %s did not resolve to a vacancy", raw)
		case res.Deduped:
			// Written, but demoted: the catalog already carried this vacancy. Saying "saved"
			// with the canonical slug would read as a fresh posting.
			saved++
			log.Printf("resolve-url: %s collapsed onto %s — the catalog already had it", raw, res.PublicSlug)
		default:
			saved++
			log.Printf("resolve-url: saved %s — %s", res.Source, res.PublicSlug)
		}
	}
	log.Printf("resolve-url: done — %d saved, %d unresolved, %d failed, of %d URL(s)",
		saved, skipped, failed, len(urls))
	if saved == 0 || failed > 0 {
		return 1
	}
	return 0
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
