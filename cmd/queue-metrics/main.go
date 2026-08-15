// Command queue-metrics measures the ingest and indexing pipeline — outbox depth, board
// fleet health, catalogue freshness — and publishes it as Prometheus gauges through the
// node_exporter textfile collector. Schedule it every minute.
//
// The per-run metrics every worker already writes (internal/worker/metrics.go) answer
// whether a worker is alive; these answer whether it is keeping up. Measuring here, on
// its own timer, rather than from a collector on the API's /metrics listener, keeps the
// cost fixed at one pass per minute instead of one per scrape per deployed colour — see
// openspec/changes/pipeline-queue-metrics/design.md.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

// textfileName is the collector file this worker owns. It deliberately does NOT follow
// the name-after-the-binary convention writeRunMetrics uses: worker.Main writes the run
// outcome to <binary>.prom AFTER run() returns, so publishing here under "queue-metrics"
// would have every run emit the queue gauges and then immediately overwrite them. The
// prefix marks the file as a pipeline measurement rather than one worker's run outcome;
// main_test.go holds the two apart.
const textfileName = "freehire-pipeline.prom"

func main() { worker.Main(run) }

func run() int {
	// Gate before Bootstrap, not after: with nowhere to publish there is nothing worth
	// opening a connection pool for, and the spec requires an unconfigured deployment
	// to run without touching the database at all.
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		log.Printf("queue-metrics: %s is unset, nothing to publish", worker.PromTextfileDirEnv)
		return 0
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

	log.Printf("queue-metrics: published %s boards=healthy:%d/failing:%d/cooled:%d",
		strings.Join(depths(snap), " "), snap.healthyBoards, snap.failingBoards, snap.cooledBoards)
	return 0
}

// depths renders the per-queue depths for the run log. Built by iterating rather than
// indexing so adding a fourth queue to collect cannot turn this line into a panic.
func depths(s snapshot) []string {
	out := make([]string, 0, len(s.queues))
	for _, q := range s.queues {
		out = append(out, fmt.Sprintf("%s=%d", q.name, q.depth))
	}
	return out
}
