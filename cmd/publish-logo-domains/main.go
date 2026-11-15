// Command publish-logo-domains writes the company-name-to-domain map the logo proxy
// consults, then exits.
//
// Logos are requested from logo.freehire.me by name, and the upstream sometimes answers a
// confident 200 with a different company's mark. Most call sites never see a company slug and
// cannot pass ?domain=, so the proxy consults this published map instead.
//
// With LOGO_DOMAIN_MAP_OUT unset this is a no-op that never opens the pool.
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/logodomain"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// outputPath is the binary's own reading of its knob, so a test can assert it.
// os.Getenv rather than a worker.Env helper: those only wrap numeric knobs, where an unreadable
// value must fail the run; a path is used verbatim or empty.
func outputPath() string {
	return strings.TrimSpace(os.Getenv("LOGO_DOMAIN_MAP_OUT"))
}

func main() { worker.Main(run) }

func run() int {
	out := outputPath()
	if out == "" {
		log.Print("publish-logo-domains: LOGO_DOMAIN_MAP_OUT unset, nothing to publish")
		return 0
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)

	// The small read first: no websites means nothing to publish.
	websiteRows, err := q.ListCompanyWebsites(ctx)
	if err != nil {
		log.Printf("publish-logo-domains: websites: %v", err)
		return 1
	}
	websites := make(map[string]string, len(websiteRows))
	for _, r := range websiteRows {
		websites[r.Slug] = r.Website
	}
	log.Printf("publish-logo-domains: %d companies carry a website", len(websites))

	// A sequential scan of jobs; see the query's comment for why narrowing it is slower.
	started := time.Now()
	spellingRows, err := q.ListOpenJobCompanySpellings(ctx)
	if err != nil {
		log.Printf("publish-logo-domains: spellings: %v", err)
		return 1
	}
	log.Printf("publish-logo-domains: %d spellings scanned in %s",
		len(spellingRows), time.Since(started).Round(time.Second))

	spellings := make([]logodomain.Spelling, 0, len(spellingRows)+len(websites))
	for _, r := range spellingRows {
		spellings = append(spellings, logodomain.Spelling{Slug: r.CompanySlug, Name: r.Company})
	}
	// The companies' own display names are keys too: company pages ask the proxy with
	// companies.name, not a posting's spelling.
	for _, r := range websiteRows {
		spellings = append(spellings, logodomain.Spelling{Slug: r.Slug, Name: r.Name})
	}

	entries, dropped := logodomain.Build(websites, spellings)

	// Refuse to swap in an empty map: an empty result is a failed measurement (as in
	// cmd/build-suggestions), and believing it would revert every logo to a name guess.
	if len(entries) == 0 {
		log.Print("publish-logo-domains: refusing to publish an empty map")
		return 1
	}

	if err := logodomain.WriteFile(out, logodomain.NewSnapshot(entries, time.Now())); err != nil {
		log.Printf("publish-logo-domains: %v", err)
		return 1
	}
	log.Printf("publish-logo-domains: published %d entries to %s (%d names dropped as ambiguous)",
		len(entries), out, dropped)
	return 0
}
