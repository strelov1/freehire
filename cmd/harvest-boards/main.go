// Command harvest-boards expands a board file (sources/<provider>.yml) with live boards
// drawn from a seed slug list. The seed (e.g. a public aggregator dump) is a candidate
// worklist only: every new slug is probed against the platform's official public API and
// kept only if it returns jobs, so the committed file is our own validated fact set, not a
// redistributed dataset. Run-once host tool; review the diff before ingesting.
//
//	go run ./cmd/harvest-boards <provider> <seed.json>
//	go run ./cmd/harvest-boards -pace 2 -workers 4 workable seed.json   # rate-limited platform
//
// A seed entry is either a bare slug or an object. The object may claim two things about the
// candidate, and a seed source that knows either should say so:
//
//	{"board": "acme", "company": "Acme Inc"}          # the employer it should belong to
//	{"board": "acme", "expect_id": "4698693006"}      # a posting it must carry
//
// The id is the stronger claim — it identifies the board by evidence rather than by a name
// resemblance — and is what makes a slug derived offline safe to propose at all.
//
// A platform with no prober of its own is probed by running its source adapter, so every
// board-keyed platform we can crawl is one we can also harvest (see proberFor).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"

	"github.com/strelov1/freehire/internal/sources"
)

// defaultProbeWorkers bounds the concurrent probe fan-out. It bounds the BURST only; a
// platform whose budget is per-window needs -pace as well (see pacer.go).
const defaultProbeWorkers = 16

func main() { os.Exit(run()) }

func run() int {
	// Flags precede the positional arguments, which stay as they were: 1 = discovery (the
	// prober must support it), 2 = provider plus seed path.
	pace := flag.Float64("pace", 0, "cap the probe rate at this many requests per second (0 = unpaced)")
	workers := flag.Int("workers", defaultProbeWorkers, "concurrent probes in flight")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 && len(args) != 2 {
		log.Printf("usage: harvest-boards [-pace N] [-workers N] <provider> [seed.json]")
		return 2
	}
	provider := args[0]
	seedPath := ""
	if len(args) == 2 {
		seedPath = args[1]
	}
	if *workers < 1 {
		log.Printf("harvest-boards: -workers must be at least 1")
		return 2
	}

	p, ok := proberFor(provider)
	if !ok {
		log.Printf("harvest-boards: no prober for provider %q, and no board-keyed adapter to fall "+
			"back on", provider)
		return 2
	}
	if a, viaAdapter := p.(adapterProber); viaAdapter {
		// Say so: an adapter probe costs a whole crawl per candidate rather than one request,
		// it publishes no employer name (so the corroboration gate stands down and liveness is
		// the only evidence), and it answers outside the run's paced, counting client.
		log.Printf("harvest-boards: %s has no prober of its own — probing by running its source "+
			"adapter; no name gate, no pacing", a.provider)
	}

	ctx := context.Background()
	client := newCountingClient(paced(sources.NewClient(), *pace))
	if *pace > 0 {
		log.Printf("harvest-boards: pacing probes at %.2f req/s with %d workers", *pace, *workers)
	}

	raw, expected, err := resolveCandidates(ctx, p, client, seedPath)
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

	kept, failures, mismatches := probeAll(ctx, client, p, candidates, expected, *workers)
	log.Printf("harvest-boards: live boards found=%d probe-failures=%d mismatches=%d refused=%d answered=%d",
		len(kept), failures, mismatches, client.refused(), client.answered())
	// A run the platform mostly turned away found nothing because it was not allowed to look.
	// The probers cannot say so — they report a refusal as an absent board — so this is the
	// only guard that can tell a truncated harvest from an exhausted one. Committing the
	// former is how a board file silently loses the boards nobody got to probe.
	if refusalsDominated(client.refused(), client.answered()) {
		log.Printf("harvest-boards: %d requests refused against %d answered — the platform is "+
			"rate-limiting this run; nothing written. Retry later, or lower -pace",
			client.refused(), client.answered())
		return 1
	}
	// Every candidate erroring means the probe itself is broken (an API change, an
	// auth wall, a network outage) — not "no new boards". Fail loudly so the empty
	// result is not mistaken for an exhausted candidate list.
	if failures > 0 && failures == len(candidates) {
		log.Printf("harvest-boards: all %d probes failed", failures)
		return 1
	}
	// Every live board disagreeing with its seed means the gate itself is wrong — a prober
	// that started reporting a platform-wide name, a seed built against the wrong employers,
	// or ids drawn from a different id space than the platform's — not "no new boards". Same
	// reasoning as the all-probes-failed guard: an empty harvest that is really a broken one
	// must not exit 0. A single rejection is the gate working, not failing, so the alarm
	// needs more than one.
	if mismatches > 1 && len(kept) == 0 {
		log.Printf("harvest-boards: every live board (%d) disagreed with what its seed expected", mismatches)
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
func resolveCandidates(ctx context.Context, p prober, c httpClient, seedPath string) ([]string, map[string]expectation, error) {
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
	expected := make(map[string]expectation)
	for i, b := range boards {
		if items[i].Company != "" || items[i].ExpectID != "" {
			expected[b] = expectation{company: items[i].Company, postingID: items[i].ExpectID}
		}
	}
	return boards, expected, nil
}

// probeAll probes every candidate concurrently (bounded), returning the live boards as
// emit-ready entries sorted by board, the number of candidates whose probe errored, and the
// number rejected because the board did not match what the seed expected of it.
// A probe error is logged and the candidate skipped, so one dead board never aborts the
// harvest; the caller uses the failure count to fail the run when EVERY probe errored
// (a broken probe or outage, not a legitimately empty harvest). expected supplies what the
// seed claims about each candidate: the employer it should belong to, which gates the board
// when the platform reports a name of its own and labels it when it reports none
// (chooseCompany), and/or the id of a posting the board should carry.
//
// Mismatches are counted apart from failures because they mean something else entirely — a
// dead board is noise, while a wave of mismatches means the candidates or the prober's name
// extraction are wrong, and burying them in the failure count would hide that.
func probeAll(ctx context.Context, client httpClient, p prober, candidates []string, expected map[string]expectation, workers int) ([]entry, int, int) {
	sem := make(chan struct{}, workers)
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
			// The board is live. It still has to be the board we were looking for.
			want := expected[slug]
			if reason := mismatchReason(ctx, client, p, slug, name, want); reason != "" {
				log.Printf("harvest-boards: %s: %s — skipped", slug, reason)
				mu.Lock()
				mismatches++
				mu.Unlock()
				return
			}
			mu.Lock()
			kept = append(kept, entry{Company: chooseCompany(name, want.company, slug), Board: slug})
			mu.Unlock()
		}(slug)
	}
	wg.Wait()
	sort.Slice(kept, func(i, j int) bool { return kept[i].Board < kept[j].Board })
	return kept, failures, mismatches
}
