package main

import (
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/worker"
)

// progressTextfileName is the collector file the running drain owns. It must differ
// from worker.RunMetricsFilename(): worker.Main writes the run outcome to
// <binary>.prom AFTER run() returns, so sharing that name would let the run file
// replace the counter. progress_test.go holds the two apart.
const progressTextfileName = "freehire-enrich-progress.prom"

// progressInterval is how often the running drain republishes its counter. A minute
// is far below the stall alert's two-hour window and costs one small file write.
const progressInterval = time.Minute

// renderProgress renders the throughput counter in the Prometheus text exposition
// format. It is a COUNTER because it only grows within a process; a restart resets it
// to zero, which increase() already treats as a counter reset.
//
// The name is a cross-repository contract with the alert rules in freehire-ops;
// progress_test.go pins the exact text.
func renderProgress(completed int64) string {
	return fmt.Sprintf(`# HELP freehire_enrich_completed_total Jobs whose enrichment this enrich process has written since it started.
# TYPE freehire_enrich_completed_total counter
freehire_enrich_completed_total %d
`, completed)
}

// publishProgress writes the counter into dir. An empty dir (PROM_TEXTFILE_DIR unset)
// publishes nothing. A failed write is logged, never fatal: the metric must not be the
// reason enrichment stops.
func publishProgress(dir string, completed int64) {
	if dir == "" {
		return
	}
	if err := worker.WriteTextfile(dir, progressTextfileName, renderProgress(completed)); err != nil {
		log.Printf("enrich: %v", err)
	}
}
