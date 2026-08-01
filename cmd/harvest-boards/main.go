// Command harvest-boards expands a board file (sources/<provider>.yml) with live boards
// drawn from a seed slug list. The seed (e.g. a public aggregator dump) is a candidate
// worklist only: every new slug is probed against the platform's official public API and
// kept only if it returns jobs, so the committed file is our own validated fact set, not a
// redistributed dataset. Run-once host tool; review the diff before ingesting.
//
//	go run ./cmd/harvest-boards <provider> <seed.json>
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"

	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/sources"
)

// probeWorkers bounds the concurrent probe fan-out. The shared client handles 429 backoff,
// so this stays polite under load without a per-request delay.
const probeWorkers = 16

func main() { os.Exit(run()) }

func run() int {
	// 3 args = seed path; 2 args = discovery (the prober must support it).
	if len(os.Args) != 2 && len(os.Args) != 3 {
		log.Printf("usage: harvest-boards <provider> [seed.json]")
		return 2
	}
	provider := os.Args[1]
	seedPath := ""
	if len(os.Args) == 3 {
		seedPath = os.Args[2]
	}

	p, ok := probers[provider]
	if !ok {
		log.Printf("harvest-boards: no prober for provider %q", provider)
		return 2
	}

	ctx := context.Background()
	client := sources.NewClient()

	raw, companyByBoard, err := resolveCandidates(ctx, p, client, seedPath)
	if err != nil {
		log.Printf("harvest-boards: %v", err)
		return 1
	}

	boardPath := fmt.Sprintf("sources/%s.yml", provider)
	cfg, err := sources.LoadConfig(boardPath)
	if err != nil {
		log.Printf("harvest-boards: %v", err)
		return 1
	}
	existing := make(map[string]bool, len(cfg.Sources))
	for _, e := range cfg.Sources {
		existing[e.Board] = true
	}

	candidates := newBoards(raw, existing, dedupKeyOf(p))
	log.Printf("harvest-boards: %s candidates=%d existing=%d new-candidates=%d",
		provider, len(raw), len(existing), len(candidates))

	kept, failures, mismatches := probeAll(ctx, client, p, candidates, companyByBoard)
	log.Printf("harvest-boards: live boards found=%d probe-failures=%d name-mismatches=%d",
		len(kept), failures, mismatches)
	// Every candidate erroring means the probe itself is broken (an API change, an
	// auth wall, a network outage) — not "no new boards". Fail loudly so the empty
	// result is not mistaken for an exhausted candidate list.
	if failures > 0 && failures == len(candidates) {
		log.Printf("harvest-boards: all %d probes failed", failures)
		return 1
	}
	// Every live board disagreeing with its seed means the gate itself is wrong — a prober
	// that started reporting a platform-wide name, or a seed built against the wrong
	// employers — not "no new boards". Same reasoning as the all-probes-failed guard: an
	// empty harvest that is really a broken one must not exit 0.
	if mismatches > 0 && len(kept) == 0 {
		log.Printf("harvest-boards: every live board (%d) disagreed with its expected employer", mismatches)
		return 1
	}
	if len(kept) == 0 {
		return 0
	}

	current, err := os.ReadFile(boardPath)
	if err != nil {
		log.Printf("harvest-boards: %v", err)
		return 1
	}
	merged, err := appendEntries(string(current), kept)
	if err != nil {
		log.Printf("harvest-boards: %v", err)
		return 1
	}
	// Write via a temp file + rename so a crash mid-write cannot leave a truncated
	// board file (the committed source of truth ingest crawls).
	tmp := boardPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(merged), 0o644); err != nil {
		log.Printf("harvest-boards: %v", err)
		return 1
	}
	if err := os.Rename(tmp, boardPath); err != nil {
		log.Printf("harvest-boards: %v", err)
		return 1
	}
	log.Printf("harvest-boards: appended %d boards to %s", len(kept), boardPath)
	return 0
}

// resolveCandidates supplies a run's candidate board ids. With no seed file the prober must
// support discovery and enumerates its own candidates from the platform API; otherwise the
// candidates come from the seed file, mapped through the prober. This is the only step that
// differs between a discovery provider and a seed-list one — dedup, probe, and append are
// shared downstream.
func resolveCandidates(ctx context.Context, p prober, c httpClient, seedPath string) ([]string, map[string]string, error) {
	if seedPath == "" {
		d, ok := p.(discoverer)
		if !ok {
			return nil, nil, fmt.Errorf("provider needs a seed file (it has no discovery support)")
		}
		boards, err := d.discover(ctx, c)
		return boards, nil, err
	}
	items, err := loadSeedItems(seedPath)
	if err != nil {
		return nil, nil, err
	}
	tokens := make([]string, len(items))
	for i, it := range items {
		tokens[i] = it.Board
	}
	boards := mapSeeds(p, tokens)
	companyByBoard := make(map[string]string)
	for i, b := range boards {
		if items[i].Company != "" {
			companyByBoard[b] = items[i].Company
		}
	}
	return boards, companyByBoard, nil
}

// probeAll probes every candidate concurrently (bounded), returning the live boards as
// emit-ready entries sorted by board, the number of candidates whose probe errored, and the
// number rejected because the board named a different employer than the seed expected.
// A probe error is logged and the candidate skipped, so one dead board never aborts the
// harvest; the caller uses the failure count to fail the run when EVERY probe errored
// (a broken probe or outage, not a legitimately empty harvest). companyByBoard supplies the
// employer each candidate is expected to belong to: it gates the board when the platform
// reports a name of its own, and labels it when the platform reports none (chooseCompany).
//
// Mismatches are counted apart from failures because they mean something else entirely — a
// dead board is noise, while a wave of mismatches means the candidates or the prober's name
// extraction are wrong, and burying them in the failure count would hide that.
func probeAll(ctx context.Context, client httpClient, p prober, candidates []string, companyByBoard map[string]string) ([]entry, int, int) {
	sem := make(chan struct{}, probeWorkers)
	var (
		mu         sync.Mutex
		kept       []entry
		failures   int
		mismatches int
		wg         sync.WaitGroup
	)
	for _, slug := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(slug string) {
			defer wg.Done()
			defer func() { <-sem }()
			name, n, err := p.probe(ctx, client, slug)
			if err != nil {
				log.Printf("harvest-boards: probe %s: %v", slug, err)
				mu.Lock()
				failures++
				mu.Unlock()
				return
			}
			if n == 0 {
				return
			}
			// The board is live. It still has to be the board we were looking for, and only
			// a name the platform publishes itself can confirm that. An expected name that
			// normalizes to nothing (punctuation alone) states no expectation at all, and is
			// treated as such rather than rejecting every board it is paired with.
			expected := companyByBoard[slug]
			if normalize.CompanyKey(expected) != "" && name != "" && !normalize.SameCompany(expected, name) {
				log.Printf("harvest-boards: %s: expected %q, board reports %q — skipped", slug, expected, name)
				mu.Lock()
				mismatches++
				mu.Unlock()
				return
			}
			mu.Lock()
			kept = append(kept, entry{Company: chooseCompany(name, expected, slug), Board: slug})
			mu.Unlock()
		}(slug)
	}
	wg.Wait()
	sort.Slice(kept, func(i, j int) bool { return kept[i].Board < kept[j].Board })
	return kept, failures, mismatches
}
