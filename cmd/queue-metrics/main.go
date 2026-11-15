// Command queue-metrics measures the ingest and indexing pipeline — outbox depth, board
// fleet health, catalogue freshness — and publishes it as Prometheus gauges through the
// node_exporter textfile collector. Schedule it every minute.
//
// The per-run metrics every worker writes (internal/platform/worker/metrics.go) say whether a
// worker is alive; these say whether it is keeping up. Measuring on its own timer, not from a
// collector on the API's /metrics, fixes the cost at one pass per minute.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// textfileName is the collector file this worker owns. It must not be named after the binary:
// worker.Main writes the run outcome to <binary>.prom after run() returns and would overwrite
// the gauges. main_test.go holds the two apart.
const textfileName = "freehire-pipeline.prom"

func main() { worker.Main(run) }

func run() int {
	// Gate before Bootstrap: an unconfigured deployment must not touch the database.
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		log.Printf("queue-metrics: %s is unset, nothing to publish", worker.PromTextfileDirEnv)
		return worker.Skip(worker.PromTextfileDirEnv)
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	snap, err := collect(ctx, db.New(pool))
	if err != nil {
		log.Printf("queue-metrics: %v", err)
		return 1
	}
	if err := worker.WriteTextfile(dir, textfileName, render(snap)); err != nil {
		log.Printf("queue-metrics: %v", err)
		return 1
	}

	fleet := snap.fleet()
	log.Printf("queue-metrics: published %s boards=healthy:%d/failing:%d/cooled:%d/uncrawled:%d",
		strings.Join(depths(snap), " "), fleet.healthy, fleet.failing, fleet.cooled, fleet.uncrawled)
	return 0
}

// depths renders the per-queue depths for the run log.
func depths(s snapshot) []string {
	out := make([]string, 0, len(s.queues))
	for _, q := range s.queues {
		out = append(out, fmt.Sprintf("%s=%d", q.name, q.depth))
	}
	return out
}
