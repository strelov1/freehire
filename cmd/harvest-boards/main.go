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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"

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

	raw, err := resolveCandidates(ctx, p, client, seedPath)
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

	kept := probeAll(ctx, client, p, candidates)
	log.Printf("harvest-boards: live boards found=%d", len(kept))
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
	if err := os.WriteFile(boardPath, []byte(merged), 0o644); err != nil {
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
func resolveCandidates(ctx context.Context, p prober, c httpClient, seedPath string) ([]string, error) {
	if seedPath == "" {
		d, ok := p.(discoverer)
		if !ok {
			return nil, fmt.Errorf("provider needs a seed file (it has no discovery support)")
		}
		return d.discover(ctx, c)
	}
	seed, err := loadSeed(seedPath)
	if err != nil {
		return nil, err
	}
	return mapSeeds(p, seed), nil
}

// loadSeed reads a JSON array of slug strings.
func loadSeed(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed %s: %w", path, err)
	}
	var slugs []string
	if err := json.Unmarshal(data, &slugs); err != nil {
		return nil, fmt.Errorf("parse seed %s: %w", path, err)
	}
	return slugs, nil
}

// probeAll probes every candidate concurrently (bounded), returning the live boards as
// emit-ready entries sorted by board. A probe error is logged and the candidate skipped, so
// one dead board never aborts the harvest.
func probeAll(ctx context.Context, client httpClient, p prober, candidates []string) []entry {
	sem := make(chan struct{}, probeWorkers)
	var (
		mu   sync.Mutex
		kept []entry
		wg   sync.WaitGroup
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
				return
			}
			if n == 0 {
				return
			}
			mu.Lock()
			kept = append(kept, entry{Company: name, Board: slug})
			mu.Unlock()
		}(slug)
	}
	wg.Wait()
	sort.Slice(kept, func(i, j int) bool { return kept[i].Board < kept[j].Board })
	return kept
}
