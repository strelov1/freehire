// Command harvest-linkedin supplies the board harvest with a worklist of companies that are
// demonstrably hiring right now. It pages LinkedIn's public, unauthenticated job-search
// listing for each query in a worklist file, collapses the postings to one candidate per
// employer, drops the employers the catalogue already covers, and reads two facts off each
// survivor: the employer's own website, and — where the posting publishes one — the id that
// posting carries in the employer's OWN applicant tracking system.
//
// The output is exactly the input `harvest-ats resolve` consumes, so the three tools chain:
//
//	go run ./cmd/harvest-linkedin harvest/linkedin-queries.yml company-slugs.txt > candidates.json
//	go run ./cmd/harvest-ats resolve candidates.json          # -> <provider>.seed.json
//	go run ./cmd/harvest-boards <provider> <provider>.seed.json
//
// LinkedIn data never reaches the repository: it is a transient worklist. What gets committed
// to sources/*.yml is boards harvest-boards validated against each platform's own public API.
// Requests go out under this project's own User-Agent (LinkedIn serves these endpoints to it)
// and are rate-limited; run-once host tool, not a cron job.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/sources"
	"golang.org/x/time/rate"
)

// defaultPace is the aggregate request rate. It is deliberately gentle: the run is a discovery
// pass measured in hundreds of requests, and nothing about it is urgent.
const defaultPace = 1.0

func main() { os.Exit(run()) }

func run() int {
	pace := flag.Float64("pace", defaultPace, "cap the request rate at this many requests per second")
	flag.Parse()
	args := flag.Args()
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

	// A search that answers with no postings looks exactly like an empty market. A run where
	// EVERY query did that is not a market with no jobs in it — it is a markup change or a
	// block, and reporting success would leave an empty harvest indistinguishable from a
	// complete one.
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

// pacedFetch returns a fetcher over the shared SSRF-guarded client — which already carries this
// project's User-Agent, a bounded retry, and Retry-After handling on 429 — gated by a limiter
// so the run's AGGREGATE rate stays put regardless of how the work is sequenced.
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

// perRequestTimeout is an outer cap on a single fetch so one wedged request cannot stall a run;
// the client's own transport timeout is the tighter, usual bound.
const perRequestTimeout = 30 * time.Second

// readSlugSet reads the catalogue's company slugs, one per line, so a company already covered
// costs no request. A missing file is an error rather than an empty set: silently harvesting
// against no catalogue would re-propose everything we already have.
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
