// Command publish-logo-domains writes the company-name-to-domain map the logo proxy
// consults, then exits.
//
// Company logos are requested from logo.freehire.me by NAME, and the upstream that
// resolves a name answers a confident 200 with a different company's mark often enough to
// matter. The proxy has accepted a ?domain= since it was written and nothing has ever
// supplied one, because most of the ~25 call sites that draw a logo — the experience
// bank, search suggestions, community subjects, the tailored-CV list — never see a
// company slug and could not look a domain up. Publishing a map the proxy consults fixes
// every one of them without touching any.
//
// Unset LOGO_DOMAIN_MAP_OUT and this is a no-op that never opens the pool, which is both
// how the change ships dark and how it is rolled back.
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

// outputPath is the binary's own reading of its knob. It exists so a test can assert what
// THIS worker does with THIS variable rather than re-stating the wiring.
//
// os.Getenv rather than a worker.Env helper: internal/platform/worker only wraps the
// NUMERIC knobs (EnvInt64/EnvInt32), where a set-but-unreadable value must fail the run
// instead of silently taking a default. A path has no such failure mode — it is used
// verbatim or it is empty.
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

	// The websites first: it is the small read, and a catalogue holding none of them
	// means there is nothing this run could publish however the scan goes.
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

	// ~53s and a sequential scan of jobs. See the query's own comment for why narrowing
	// it to the companies above is four times slower.
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
	// The companies' own display names are keys too: /companies, the company header and
	// the company picker ask the proxy with companies.name rather than with a posting's
	// spelling of it. Reading them costs nothing here — the rows are already in hand.
	for _, r := range websiteRows {
		spellings = append(spellings, logodomain.Spelling{Slug: r.Slug, Name: r.Name})
	}

	entries, dropped := logodomain.Build(websites, spellings)

	// A run that would publish an empty map refuses to swap. A catalogue yielding nothing
	// is a failed measurement, not an empty catalogue — the rule cmd/build-suggestions
	// already follows — and here the consequence of believing it is every logo on the
	// site reverting to a name guess at once.
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
