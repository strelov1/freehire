// Command serve-probe samples random open postings and checks that every field a
// posting STORES reaches what it SERVES — both through the projection the API runs and
// through the live Meilisearch document search reads — then publishes the gaps as
// Prometheus gauges through the node_exporter textfile collector. Schedule it hourly.
//
// See internal/search/serveprobe for why: a stored field that never reaches a reader
// fails silently, because an empty value is omitted from the JSON and every endpoint
// still answers 200. A gap in the projection layer is a code bug; a gap only in the
// index layer means a reindex is owed.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/serveprobe"
)

// textfileName is the collector file this worker owns. Not named after the binary: the
// worker.Main run-outcome gauge is written to <binary>.prom after run() returns and
// would overwrite it (the same reason queue-metrics and search-settings-drift give).
const textfileName = "freehire-serve-probe.prom"

// windowSize is how many consecutive indexable rows one random draw takes. The sample
// is many small windows rather than one large one so it spreads across the id space —
// old postings and new ones come from different adapters and different code paths.
const windowSize = 25

func main() { worker.Main(run) }

func run() int {
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		log.Printf("serve-probe: %s is unset, nothing to publish", worker.PromTextfileDirEnv)
		return 0
	}
	cfg := config.Load()
	if cfg.MeiliKey == "" {
		log.Print("serve-probe: MEILI_MASTER_KEY is unset, nothing to compare against")
		return 0
	}
	sample, err := worker.EnvInt32("SERVE_PROBE_SAMPLE", 500)
	if err != nil {
		log.Printf("serve-probe: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	rows, err := sampleRows(ctx, q, int(sample))
	if err != nil {
		log.Printf("serve-probe: %v", err)
		return 1
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	docs, err := search.NewClient(cfg.MeiliURL, cfg.MeiliKey).JobViewsByID(ctx, ids)
	if err != nil {
		log.Printf("serve-probe: %v", err)
		return 1
	}

	rep := serveprobe.Check(rows, docs)
	if err := worker.WriteTextfile(dir, textfileName, render(rep)); err != nil {
		log.Printf("serve-probe: %v", err)
		return 1
	}
	log.Printf("serve-probe: sampled=%d unindexed=%d unreadable=%d", rep.Sampled, rep.Unindexed, rep.Unreadable)
	for _, l := range serveprobe.Layers {
		var gaps []string
		for _, f := range serveprobe.Fields() {
			if c := rep.Counts[l][f]; c.Gaps > 0 {
				gaps = append(gaps, fmt.Sprintf("%s=%d/%d", f, c.Gaps, c.Stored))
			}
		}
		if len(gaps) > 0 {
			log.Printf("serve-probe: %s gaps (not served/stored): %s", l, strings.Join(gaps, " "))
		}
	}
	// A gap is what this worker exists to report, not its own failure — the same rule
	// search-settings-drift and llm-probe follow, so a red unit always means the probe
	// itself broke.
	return 0
}

// sampleRows draws random windows of indexable rows until it holds n of them. Each draw
// starts at a uniform id below the table's top; a draw past the last indexable row
// returns nothing and is simply repeated, bounded so an empty catalogue cannot spin.
func sampleRows(ctx context.Context, q *db.Queries, n int) ([]db.Job, error) {
	maxID, err := q.MaxJobID(ctx)
	if err != nil {
		return nil, fmt.Errorf("max job id: %w", err)
	}
	if maxID == 0 {
		return nil, nil
	}
	seen := map[int64]bool{}
	var out []db.Job
	for draws := 0; len(out) < n && draws < 4*n/windowSize+10; draws++ {
		page, err := q.ListIndexableJobsByIDAfter(ctx, db.ListIndexableJobsByIDAfterParams{
			AfterID:   rand.Int64N(maxID),
			BatchSize: windowSize,
		})
		if err != nil {
			return nil, fmt.Errorf("sample window: %w", err)
		}
		for _, r := range page {
			if !seen[r.ID] && len(out) < n {
				seen[r.ID] = true
				out = append(out, r)
			}
		}
	}
	return out, nil
}
