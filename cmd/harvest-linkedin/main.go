// Command harvest-linkedin supplies the board harvest with a worklist of companies that are hiring
// right now. It pages LinkedIn's public job-search listing for each query in a worklist file,
// collapses postings to one candidate per employer, drops employers the catalogue already covers, and
// reads two facts off each survivor: the employer's website and, where the posting publishes one, the
// id it carries in the employer's OWN applicant tracking system.
//
// The output is exactly the input `harvest-ats resolve` consumes, so the three tools chain:
//
//	go run ./cmd/harvest-linkedin harvest/linkedin-queries.yml company-slugs.txt > candidates.json
//	go run ./cmd/harvest-ats resolve candidates.json          # -> <provider>.seed.json
//	go run ./cmd/harvest-boards <provider> <provider>.seed.json
//
// Register the same worklist as pending searches for ordinary LinkedIn ingestion:
//
//	go run ./cmd/harvest-linkedin --register-searches harvest/linkedin-queries.yml
//	go run ./cmd/harvest-linkedin --register-searches --apply harvest/linkedin-queries.yml
//	go run ./cmd/harvest-linkedin --register-searches --pages=10 --jobage=30 --apply harvest/linkedin-queries.yml
//
// Registration preserves existing live search configuration. It writes no jobs;
// the normal LinkedIn adapter verifies descriptions and application ownership.
//
// LinkedIn data never reaches the repository: it is a transient worklist; what gets committed is
// boards harvest-boards validated against each platform's own API. Requests go out under this
// project's User-Agent, rate-limited; run-once host tool, not a cron job.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/worker"
	"golang.org/x/time/rate"
)

// defaultPace is the aggregate request rate, deliberately gentle for a discovery pass.
const defaultPace = 1.0

func main() { worker.Main(run) }

func run() int {
	return runArgs(os.Args[1:])
}

func runArgs(args []string) int {
	flags := flag.NewFlagSet("harvest-linkedin", flag.ContinueOnError)
	pace := flags.Float64("pace", defaultPace, "cap the request rate at this many requests per second")
	register := flags.Bool("register-searches", false, "register worklist searches for ordinary LinkedIn ingestion (preview without --apply)")
	apply := flags.Bool("apply", false, "write search registrations; requires --register-searches")
	pages := flags.Int("pages", 0, "page budget for newly registered searches, 1..100 (0 keeps worklist values); requires --register-searches")
	jobAge := flags.Int("jobage", 0, "age window in days for newly registered searches, 1..30 (0 keeps worklist values); requires --register-searches")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	args = flags.Args()
	budgetSpecified := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "pages" || f.Name == "jobage" {
			budgetSpecified = true
		}
	})
	if budgetSpecified && !*register {
		log.Print("harvest-linkedin: --pages and --jobage require --register-searches")
		return 2
	}
	if *pages < 0 || *pages > maxPages || *jobAge < 0 || *jobAge > maxJobAge {
		log.Printf("harvest-linkedin: --pages must be 1..%d and --jobage must be 1..%d, or 0 to keep worklist values", maxPages, maxJobAge)
		return 2
	}
	if *apply && !*register {
		log.Print("harvest-linkedin: --apply requires --register-searches")
		return 2
	}
	if *register && len(args) != 1 {
		log.Print("usage: harvest-linkedin --register-searches [--apply] <queries.yml>")
		return 2
	}
	if len(args) != 1 && len(args) != 2 {
		log.Printf("usage: harvest-linkedin [-pace N] <queries.yml> [company-slugs.txt]")
		return 2
	}
	if *pace <= 0 {
		log.Printf("harvest-linkedin: -pace must be greater than zero")
		return 2
	}

	queries, err := loadQueries(args[0])
	if err != nil {
		log.Printf("harvest-linkedin: %v", err)
		return 1
	}
	if *register {
		for i := range queries {
			if *pages > 0 {
				queries[i].Pages = *pages
			}
			if *jobAge > 0 {
				queries[i].JobAge = *jobAge
			}
		}
		return runRegisterSearches(queries, *apply)
	}
	existing := map[string]bool{}
	if len(args) == 2 {
		if existing, err = readSlugSet(args[1]); err != nil {
			log.Printf("harvest-linkedin: %v", err)
			return 1
		}
	}
	log.Printf("harvest-linkedin: %d queries, %d existing company slugs, pacing at %.2f req/s",
		len(queries), len(existing), *pace)

	candidates, st, err := discover(context.Background(), pacedFetch(*pace), queries, existing)
	if err != nil {
		log.Printf("harvest-linkedin: %v", err)
		return 1
	}
	log.Printf("harvest-linkedin: %d candidate companies from %d queries (%d returned nothing)",
		len(candidates), st.totalQueries, st.emptyQueries)

	// A search with no postings looks like an empty market. If EVERY query did that it is a markup
	// change or a block, and reporting success would pass an empty harvest off as a complete one.
	if st.everyQueryEmpty() {
		log.Printf("harvest-linkedin: every query returned no postings — the listing markup changed, " +
			"or this run is being refused; nothing written")
		return 1
	}
	if err := json.NewEncoder(os.Stdout).Encode(candidates); err != nil {
		log.Printf("harvest-linkedin: encode: %v", err)
		return 1
	}
	log.Printf("harvest-linkedin done: feed the output to `harvest-ats resolve`")
	return 0
}

// pacedFetch returns a fetcher over the shared SSRF-guarded client (project User-Agent, bounded
// retry, Retry-After on 429), gated by a limiter so the AGGREGATE rate holds however work is sequenced.
func pacedFetch(pace float64) fetcher {
	client := sources.NewClient()
	limiter := rate.NewLimiter(rate.Limit(pace), 1)
	return func(ctx context.Context, url string) (string, error) {
		if err := limiter.Wait(ctx); err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(ctx, perRequestTimeout)
		defer cancel()
		return client.GetText(ctx, url)
	}
}

// perRequestTimeout caps a single fetch so one wedged request cannot stall a run.
const perRequestTimeout = 30 * time.Second

// readSlugSet reads the catalogue's company slugs, one per line, so a covered company costs no
// request. A missing file is an error: harvesting against no catalogue would re-propose everything.
func readSlugSet(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			set[s] = true
		}
	}
	return set, nil
}
