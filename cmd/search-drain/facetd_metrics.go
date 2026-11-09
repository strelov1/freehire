package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/platform/worker"
)

// facetdTextfile is this worker's node_exporter textfile-collector file. Its own name, not
// the shared run-metrics one, for the reason worker.RunMetricsFilename documents.
const facetdTextfile = "freehire_facetd_mirror.prom"

// renderFacetdMetrics renders the mirror's health: the retry queue's depth and dead letters
// (an alert fires on a depth that does not fall), and the facetd writes that failed in this
// run. All gauges, as the textfile collector has no memory between runs.
func renderFacetdMetrics(depth, deadLetters, failedWrites int64) string {
	return fmt.Sprintf(`# HELP freehire_facetd_outbox_depth Jobs waiting in facetd_outbox for a facetd write retry.
# TYPE freehire_facetd_outbox_depth gauge
freehire_facetd_outbox_depth %d
# HELP freehire_facetd_outbox_dead_letters Retry entries given up on; facetd misses these jobs until a rebuild.
# TYPE freehire_facetd_outbox_dead_letters gauge
freehire_facetd_outbox_dead_letters %d
# HELP freehire_facetd_write_failures Failed facetd writes in the last search-drain run.
# TYPE freehire_facetd_write_failures gauge
freehire_facetd_write_failures %d
`, depth, deadLetters, failedWrites)
}

// publishFacetdMetrics writes the textfile when PROM_TEXTFILE_DIR is set. Best effort: a
// metrics failure is logged and never changes the run's outcome.
func publishFacetdMetrics(ctx context.Context, store facetdRetryStore, failedWrites int64) {
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		return
	}
	depth, dead, err := store.Stats(ctx)
	if err != nil {
		log.Printf("search-drain: facetd outbox stats: %v", err)
		return
	}
	if err := worker.WriteTextfile(dir, facetdTextfile, renderFacetdMetrics(depth, dead, failedWrites)); err != nil {
		log.Printf("search-drain: write %s: %v", facetdTextfile, err)
	}
}
