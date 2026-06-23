// Command harvest-role mines role.com — a job aggregator whose every posting links out
// to the real ATS vacancy — for ATS boards we don't yet ingest. It walks role.com's
// sitemaps newest-first, follows each job page's outbound apply link, and resolves the
// supported ones to a (provider, board) pair via atsdetect.FromURL. The distinct boards
// are written as per-provider seed files that the existing cmd/harvest-boards then
// validates against each platform's API and commits to sources/*.yml — so role.com is a
// discovery layer, never a stored source (no cross-source duplication; jobs are crawled
// under their real ATS identity). Static fetch only; run-once host tool.
//
//	harvest-role [maxDetails]   # cap on job pages fetched (default 20000)
//
// Boards saturate far faster than postings (one board backs many jobs), so a bounded
// sample surfaces most of the catalogue's boards. Re-run with a higher cap to dig deeper.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/sources"
)

const (
	roleBase          = "https://role.com"
	sitemapIndexURL   = roleBase + "/sitemap.xml"
	defaultMaxDetails = 20000
	detailWorkers     = 14
	// maxShards bounds how many sitemap documents (several MB each) a run downloads.
	// The detail budget is spread evenly across this many shards sampled across the
	// whole id-range, so discovery sees the full catalogue's board diversity rather
	// than one retail-heavy id-range.
	maxShards = 150
)

func main() { os.Exit(run()) }

func run() int {
	maxDetails := defaultMaxDetails
	if len(os.Args) >= 2 {
		if n, err := strconv.Atoi(os.Args[1]); err == nil && n > 0 {
			maxDetails = n
		}
	}

	ctx := context.Background()
	client := sources.NewClient()

	idx, err := fetchBytes(sitemapIndexURL)
	if err != nil {
		log.Printf("harvest-role: sitemap index: %v", err)
		return 1
	}
	shards := jobSitemaps(sitemapLocs(idx))
	if len(shards) == 0 {
		log.Printf("harvest-role: no job sitemaps in index")
		return 1
	}
	shards = strideSample(shards, maxShards)
	perShard := maxDetails / len(shards)
	if perShard < 1 {
		perShard = 1
	}
	log.Printf("harvest-role: sampling %d job pages each across %d sitemaps (cap %d)", perShard, len(shards), maxDetails)

	var (
		mu       sync.Mutex
		boards   = map[string]map[string]string{} // provider -> board id -> employer name
		distinct atomic.Int64
		fetched  atomic.Int64
		jobs     = make(chan string)
		wg       sync.WaitGroup
	)
	record := func(provider, board, company string) {
		mu.Lock()
		defer mu.Unlock()
		if boards[provider] == nil {
			boards[provider] = map[string]string{}
		}
		if _, seen := boards[provider][board]; !seen {
			boards[provider][board] = company
			distinct.Add(1)
		} else if boards[provider][board] == "" && company != "" {
			boards[provider][board] = company // backfill a name a later posting supplied
		}
	}
	for i := 0; i < detailWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				body, err := client.GetText(ctx, u)
				n := fetched.Add(1)
				if err == nil {
					if p, b, company, ok := classifyDetail([]byte(body)); ok {
						record(p, b, company)
					}
				}
				if n%500 == 0 {
					log.Printf("harvest-role: fetched %d job pages, %d distinct boards", n, distinct.Load())
				}
			}
		}()
	}

feed:
	for _, sm := range shards {
		body, err := fetchBytes(sm)
		if err != nil {
			log.Printf("harvest-role: sitemap %s: %v", sm, err)
			continue
		}
		for _, jobURL := range strideSample(sitemapLocs(body), perShard) {
			if int(fetched.Load()) >= maxDetails {
				break feed
			}
			jobs <- jobURL
		}
	}
	close(jobs)
	wg.Wait()

	return writeSeeds(boards)
}

// jobSitemapNum captures the integer N of a sitemap_jobs<N>.xml shard URL; the number
// tracks role.com's posting-id range, so higher N holds the freshest postings.
var jobSitemapNum = regexp.MustCompile(`sitemap_jobs(\d+)\.xml`)

// jobSitemaps keeps only the job-shard sitemaps from a sitemap index, ordered newest
// (highest-numbered) first so a capped run covers the freshest postings.
func jobSitemaps(locs []string) []string {
	type shard struct {
		url string
		num int
	}
	var shards []shard
	for _, u := range locs {
		if m := jobSitemapNum.FindStringSubmatch(u); m != nil {
			n, _ := strconv.Atoi(m[1])
			shards = append(shards, shard{u, n})
		}
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].num > shards[j].num })
	out := make([]string, len(shards))
	for i, s := range shards {
		out[i] = s.url
	}
	return out
}

// seedEntry is one board candidate written to a seed file: the board id plus the employer
// name read from role.com's JSON-LD, which harvest-boards uses where the platform API
// exposes no name (see cmd/harvest-boards seedItem).
type seedEntry struct {
	Board   string `json:"board"`
	Company string `json:"company,omitempty"`
}

// writeSeeds writes one sorted <provider>.seed.json per provider and logs a summary.
func writeSeeds(boards map[string]map[string]string) int {
	provs := make([]string, 0, len(boards))
	for p := range boards {
		provs = append(provs, p)
	}
	sort.Strings(provs)

	total := 0
	for _, prov := range provs {
		byBoard := boards[prov]
		out := make([]seedEntry, 0, len(byBoard))
		for b, company := range byBoard {
			out = append(out, seedEntry{Board: b, Company: company})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Board < out[j].Board })
		name := prov + ".seed.json"
		if err := writeJSON(name, out); err != nil {
			log.Printf("harvest-role: write %s: %v", name, err)
			return 1
		}
		log.Printf("harvest-role: %s: %d boards -> %s", prov, len(out), name)
		total += len(out)
	}
	log.Printf("harvest-role done: %d boards across %d providers; run `harvest-boards <provider> <provider>.seed.json` to validate", total, len(provs))
	return 0
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// fetchBytes GETs a sitemap document whole. Sitemaps run several MB (past the sources
// client's GetText cap), so this reads the body uncapped; the detail-page fetch keeps
// using the shared client for its 429 backoff.
func fetchBytes(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; freehire-harvest/1.0)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
