// Command harvest-orphans builds a candidate-board seed from the companies the catalogue
// holds only through aggregators — an open posting from an aggregator and none from any
// first-party ATS. Each company proposes board ids derived from its name alone (no website
// resolution, no search), paired with the employer the board is expected to belong to.
//
// The seed carries no provider: cmd/harvest-boards de-duplicates against each provider's own
// board file and probes what remains, so one seed is run against every provider whose board
// id is a name. Providers keyed by a tenant token or a numeric id (workday, gupy, icims,
// oracle, taleo, cornerstone, pageup, neogov) cannot be seeded this way.
//
//	harvest-orphans [-aggregators a,b] [-out orphans.seed.json]   # needs DATABASE_URL
//	go run ./cmd/harvest-boards <provider> orphans.seed.json      # then, per provider
//
// Run-once host tool; review the resulting board-file diff before ingesting.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// remoteAggregators is the default requested set: the remote-jobs aggregators, whose
// companies are overwhelmingly product employers rather than the local staffing agencies
// that dominate the country-specific boards and rarely run an ATS of their own.
var remoteAggregators = []string{
	"himalayas", "remoteok", "weworkremotely", "jobspresso",
	"workingnomads", "4dayweek", "jobicy", "remotive",
}

// queryTimeout bounds the worklist scan. It is a backstop against a planner flip, not a
// budget: the query runs in seconds on the production catalogue.
const queryTimeout = "10min"

func main() { worker.Main(run) }

func run() int {
	requested := flag.String("aggregators", strings.Join(remoteAggregators, ","),
		"comma-separated aggregator sources to draw companies from")
	out := flag.String("out", "orphans.seed.json", "path to write the candidate seed to")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	from := splitList(*requested)
	if len(from) == 0 {
		log.Printf("harvest-orphans: no aggregators requested")
		return 2
	}
	// The exclusion test always considers EVERY aggregator, whatever this run asked for:
	// narrowing the request must not make another aggregator's posting look like the
	// first-party ATS coverage that disqualifies a company.
	all := allAggregatorProviders()
	if unknown := notIn(from, all); len(unknown) > 0 {
		log.Printf("harvest-orphans: not aggregator sources: %s", strings.Join(unknown, ", "))
		return 2
	}

	// One pinned connection with a bounded statement time. The worklist scan is a single
	// long read over the whole jobs table, and an unbounded one held open against prod is the
	// snapshot hazard that has taken the site down before; measured at ~6s on the production
	// catalogue, so a cap this generous only ever fires on a plan that has gone wrong.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("harvest-orphans: acquire: %v", err)
		return 1
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET statement_timeout = '"+queryTimeout+"'"); err != nil {
		log.Printf("harvest-orphans: set statement_timeout: %v", err)
		return 1
	}

	rows, err := db.New(conn).OrphanAggregatorCompanies(ctx, db.OrphanAggregatorCompaniesParams{
		Requested:   from,
		Aggregators: all,
	})
	if err != nil {
		log.Printf("harvest-orphans: query: %v", err)
		return 1
	}

	orphans := make([]orphan, 0, len(rows))
	for _, r := range rows {
		orphans = append(orphans, orphan{Slug: r.CompanySlug, Company: r.Company})
	}
	entries := seedEntries(orphans)
	log.Printf("harvest-orphans: %d companies over %d aggregators -> %d candidate boards",
		len(orphans), len(from), len(entries))
	if len(entries) == 0 {
		return 0
	}

	blob, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Printf("harvest-orphans: encode: %v", err)
		return 1
	}
	if err := os.WriteFile(*out, blob, 0o644); err != nil {
		log.Printf("harvest-orphans: write %s: %v", *out, err)
		return 1
	}
	log.Printf("harvest-orphans: wrote %s", *out)
	return 0
}

// allAggregatorProviders is every provider the pipeline treats as an aggregator — the set the
// exclusion test is evaluated against, and the set a requested source must belong to. It reads
// the taxonomy, not the crawl registry: this tool needs only DATABASE_URL, so a
// credential-gated aggregator (reed, usajobs, whatjobs) would otherwise be absent and its
// postings would read as the first-party ATS coverage that disqualifies a company.
func allAggregatorProviders() []string {
	return sources.AggregatorProviders(sources.Taxonomy())
}

// splitList parses a comma-separated flag, dropping blanks so a trailing comma is harmless.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// notIn returns the members of want absent from have. A source misspelt on the command line
// would otherwise select nothing and report an empty worklist as a real result.
func notIn(want, have []string) []string {
	known := make(map[string]bool, len(have))
	for _, h := range have {
		known[h] = true
	}
	var missing []string
	for _, w := range want {
		if !known[w] {
			missing = append(missing, w)
		}
	}
	return missing
}
