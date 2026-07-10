// Command harvest-loxo curates Loxo agency boards from an operator-supplied footprint.
// Loxo exposes no public directory of agencies, so the operator gathers footprint URLs
// (careers pages and /job/<base64> links, e.g. from a `site:app.loxo.co` search) and pipes
// them in; this tool extracts each board's (host, slug), live-validates it via the loxo
// adapter, counts how many postings classify as tech, and emits draft sources/loxo.yml
// entries (all hub: true) for human review — it never edits the board file itself.
//
//	go run ./cmd/harvest-loxo < footprint-urls.txt > candidates.yml
//	go run ./cmd/harvest-loxo footprint-urls.txt
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/sources"
)

// boardPath is the board file the emitted entries are de-duplicated against.
const boardPath = "sources/loxo.yml"

// probeWorkers bounds the concurrent board probes. The shared client handles 429 backoff.
const probeWorkers = 8

type candidate struct{ host, slug string }

type result struct {
	cand        candidate
	company     string
	total, tech int
}

func main() { os.Exit(run()) }

func run() int {
	lines, err := readLines()
	if err != nil {
		log.Printf("harvest-loxo: %v", err)
		return 1
	}
	cands := extractCandidates(lines)
	existing := loadExistingBoards(boardPath)
	var todo []candidate
	for _, c := range cands {
		if !existing[c.host+"/"+c.slug] {
			todo = append(todo, c)
		}
	}
	log.Printf("harvest-loxo: %d candidates, %d new (after de-dup vs %s)", len(cands), len(todo), boardPath)

	client := sources.NewClient()
	adapter := sources.NewLoxo(client)
	kept := probeAll(context.Background(), client, adapter, todo)
	sort.Slice(kept, func(i, j int) bool { return kept[i].cand.slug < kept[j].cand.slug })

	for _, r := range kept {
		fmt.Printf("# %s/%s — %d jobs, %d tech\n", r.cand.host, r.cand.slug, r.total, r.tech)
		fmt.Printf("- company: %s\n  board: %s/%s\n  hub: true\n", r.company, r.cand.host, r.cand.slug)
	}
	log.Printf("harvest-loxo: %d/%d boards validated with open jobs", len(kept), len(todo))
	return 0
}

// readLines reads footprint URLs from a seed-file arg or stdin.
func readLines() ([]string, error) {
	src := os.Stdin
	if len(os.Args) == 2 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			return nil, err
		}
		defer f.Close()
		src = f
	}
	var lines []string
	sc := bufio.NewScanner(src)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// extractCandidates parses footprint URLs into a distinct, order-preserving careers-board
// candidate set.
func extractCandidates(lines []string) []candidate {
	var out []candidate
	seen := map[string]bool{}
	for _, l := range lines {
		c, ok := candidateFromURL(l)
		if !ok {
			continue
		}
		key := c.host + "/" + c.slug
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

// candidateFromURL turns a Loxo footprint URL into a careers-board (host, slug), ok=false
// for job-detail, auth, and non-Loxo URLs. A careers page is a single non-reserved path
// segment; a /job/<base64> URL has two segments and carries no slug.
func candidateFromURL(raw string) (candidate, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !isLoxoHost(u.Host) {
		return candidate{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		return candidate{}, false
	}
	switch parts[0] {
	case "job", "jobs", "login", "logout", "api", "signup", "password", "users":
		return candidate{}, false
	}
	return candidate{host: u.Host, slug: parts[0]}, true
}

// isLoxoHost reports whether host is app.loxo.co or one of its subdomains (agency or pod),
// without matching a lookalike like "notapp.loxo.co".
func isLoxoHost(host string) bool {
	return host == "app.loxo.co" || strings.HasSuffix(host, ".app.loxo.co")
}

// probeAll validates each candidate concurrently, keeping the boards that yield open jobs.
func probeAll(ctx context.Context, client sources.HTTPClient, adapter sources.Source, cands []candidate) []result {
	var (
		mu   sync.Mutex
		kept []result
		wg   sync.WaitGroup
	)
	sem := make(chan struct{}, probeWorkers)
	for _, c := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func(c candidate) {
			defer wg.Done()
			defer func() { <-sem }()
			if r, ok := probe(ctx, client, adapter, c); ok {
				mu.Lock()
				kept = append(kept, r)
				mu.Unlock()
			}
		}(c)
	}
	wg.Wait()
	return kept
}

// probe live-validates one board: the loxo adapter must return at least one posting. It
// counts tech postings by title and reads the agency's display name from the listing title.
func probe(ctx context.Context, client sources.HTTPClient, adapter sources.Source, c candidate) (result, bool) {
	jobs, err := adapter.Fetch(ctx, sources.CompanyEntry{
		Board: c.host + "/" + c.slug, Company: c.slug, Hub: true, Provider: "loxo",
	})
	if err != nil {
		log.Printf("harvest-loxo: %s/%s: %v", c.host, c.slug, err)
		return result{}, false
	}
	if len(jobs) == 0 {
		return result{}, false
	}
	tech := 0
	for _, j := range jobs {
		if isTechCategory(classify.Parse(j.Title).Category) {
			tech++
		}
	}
	company := c.slug
	if txt, err := client.GetText(ctx, "https://"+c.host+"/"+c.slug); err == nil {
		if n := agencyName(txt); n != "" {
			company = n
		}
	}
	return result{cand: c, company: company, total: len(jobs), tech: tech}, true
}

// loxoTitlePattern captures the agency name from a Loxo listing "<title>Job Listing | Agency</title>".
var loxoTitlePattern = regexp.MustCompile(`(?s)<title>\s*Job Listing\s*\|\s*(.+?)\s*</title>`)

// agencyName returns the agency display name from a listing page's title, or "".
func agencyName(listingHTML string) string {
	if m := loxoTitlePattern.FindStringSubmatch(listingHTML); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// isTechCategory reports whether a classified title category counts as tech: a resolved
// category that is not one of the confidently non-tech ones.
func isTechCategory(category string) bool {
	if category == "" {
		return false
	}
	for _, nt := range enrich.NonTechCategories {
		if category == nt {
			return false
		}
	}
	return true
}

// loadExistingBoards returns the set of "host/slug" boards already in the board file, so
// the emitter never re-proposes a known board. A missing file yields an empty set.
func loadExistingBoards(path string) map[string]bool {
	m := map[string]bool{}
	cfg, err := sources.LoadConfig(path)
	if err != nil {
		return m
	}
	for _, e := range cfg.Sources {
		m[e.Board] = true
	}
	return m
}
