// Command harvest-ats is the discovery half of the domain-following harvest. It
// turns the curated-collection datasets into a worklist of companies we don't yet
// ingest, follows each company's website to its careers page, and detects the ATS
// board (greenhouse/lever/ashby) linked there. The detected slugs are written as
// per-provider seed files that the existing cmd/harvest-boards then validates
// against each provider's API and commits to sources/*.yml. Static fetch only —
// JS-only careers pages are skipped. Run-once host tool.
//
//	harvest-ats extract <company-slugs.txt>   # datasets → unmatched {name,website} JSON (stdout)
//	harvest-ats resolve <unmatched.json>      # → <provider>.seed.json per provider
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/sources"
)

// resolveWorkers bounds the concurrent careers-page fetch fan-out. The shared
// client handles per-request timeout and 429 backoff, so this stays polite.
const resolveWorkers = 24

// perPageTimeout is an outer cap on a single careers-page fetch so a worker can't
// wedge; the sources client's own 15s transport timeout is the tighter, usual cap.
const perPageTimeout = 20 * time.Second

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) < 3 {
		log.Printf("usage: harvest-ats extract <company-slugs.txt> | resolve <unmatched.json>")
		return 2
	}
	switch os.Args[1] {
	case "extract":
		return runExtract(os.Args[2])
	case "resolve":
		return runResolve(os.Args[2])
	default:
		log.Printf("harvest-ats: unknown subcommand %q", os.Args[1])
		return 2
	}
}

// runExtract reads the collection datasets, parses each company's (name, website),
// drops those already in our catalogue (slugs from the supplied file), and writes
// the unmatched companies as JSON to stdout.
func runExtract(slugFile string) int {
	existing, err := readSlugSet(slugFile)
	if err != nil {
		log.Printf("harvest-ats: read slug set: %v", err)
		return 1
	}
	log.Printf("harvest-ats: %d existing company slugs", len(existing))

	var all []companySite
	for _, c := range collections.All {
		parse, ok := siteParsers[c.Slug]
		if !ok || c.Dataset == nil {
			continue
		}
		body, err := fetchDataset(c.Dataset.URL)
		if err != nil {
			log.Printf("harvest-ats: fetch %s dataset: %v", c.Slug, err)
			return 1
		}
		sites, err := parse(body)
		if err != nil {
			log.Printf("harvest-ats: parse %s dataset: %v", c.Slug, err)
			return 1
		}
		log.Printf("harvest-ats: %s: %d companies with a website", c.Slug, len(sites))
		all = append(all, sites...)
	}

	unmatched := dedupeByWebsite(filterUnmatched(all, existing))
	log.Printf("harvest-ats: %d unmatched companies with a website (from %d total)", len(unmatched), len(all))
	if err := json.NewEncoder(os.Stdout).Encode(unmatched); err != nil {
		log.Printf("harvest-ats: encode: %v", err)
		return 1
	}
	return 0
}

// runResolve follows each unmatched company's website to its ATS board and writes
// per-provider seed files of the detected slugs.
func runResolve(inputFile string) int {
	sites, err := readSites(inputFile)
	if err != nil {
		log.Printf("harvest-ats: read input: %v", err)
		return 1
	}
	log.Printf("harvest-ats: resolving %d companies (workers=%d)", len(sites), resolveWorkers)

	client := sources.NewClient()
	fetch := func(u string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), perPageTimeout)
		defer cancel()
		return client.GetText(ctx, u)
	}

	type hit struct{ provider, slug string }
	var (
		mu     sync.Mutex
		byProv = map[string]map[string]bool{}
		done   atomic.Int64
		jobs   = make(chan companySite)
		wg     sync.WaitGroup
	)
	record := func(h hit) {
		mu.Lock()
		defer mu.Unlock()
		if byProv[h.provider] == nil {
			byProv[h.provider] = map[string]bool{}
		}
		byProv[h.provider][h.slug] = true
	}
	for i := 0; i < resolveWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for site := range jobs {
				if p, s, ok := resolve(site.Website, fetch); ok {
					record(hit{p, s})
				}
				if n := done.Add(1); n%200 == 0 {
					log.Printf("harvest-ats: resolved %d/%d", n, len(sites))
				}
			}
		}()
	}
	for _, s := range sites {
		jobs <- s
	}
	close(jobs)
	wg.Wait()

	total := 0
	for prov, slugs := range byProv {
		out := make([]string, 0, len(slugs))
		for s := range slugs {
			out = append(out, s)
		}
		sort.Strings(out)
		name := prov + ".seed.json"
		if err := writeJSON(name, out); err != nil {
			log.Printf("harvest-ats: write %s: %v", name, err)
			return 1
		}
		log.Printf("harvest-ats: %s: %d boards -> %s", prov, len(out), name)
		total += len(out)
	}
	log.Printf("harvest-ats done: %d boards across %d providers; run `harvest-boards <provider> <provider>.seed.json` to validate", total, len(byProv))
	return 0
}

// fetchDataset downloads a dataset URL (trusted, possibly several MB — no body cap).
func fetchDataset(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func readSlugSet(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range splitLines(string(data)) {
		if line != "" {
			set[line] = true
		}
	}
	return set, nil
}

func readSites(path string) ([]companySite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sites []companySite
	if err := json.Unmarshal(data, &sites); err != nil {
		return nil, err
	}
	return sites, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
