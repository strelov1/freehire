package worker

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// A worker's duration and exit status say that it ran, not whether it was worth running
// or where the time went. These two calls let a worker say so; the gauges land in the
// same per-run file as the rest of its run metrics. Both are optional: a worker that
// never calls them publishes none of these series, which reads as "does not report"
// rather than as a false "did nothing".
var work struct {
	sync.Mutex
	reported           bool
	processed, changed int64
	stages             map[string]float64
}

// ReportWork adds to what this run did: processed is how many items it looked at,
// changed how many of those it wrote. Safe to call repeatedly and concurrently. A run
// that keeps reporting processed > 0 with changed == 0 is doing the reading for nothing.
func ReportWork(processed, changed int64) {
	work.Lock()
	defer work.Unlock()
	work.reported = true
	work.processed += processed
	work.changed += changed
}

// Stage starts timing a named part of this run and returns the call that ends it, which
// also logs the time so a run in progress can be read from the journal. A name used
// twice accumulates. A stage that never ends, because the run failed inside it, is not
// published: its time would be wrong, and the failure is reported by the exit status.
func Stage(name string) (done func()) {
	start := time.Now()
	return func() {
		elapsed := time.Since(start)
		log.Printf("worker: stage %q took %s", name, elapsed.Round(time.Second))
		work.Lock()
		defer work.Unlock()
		if work.stages == nil {
			work.stages = map[string]float64{}
		}
		work.stages[name] += elapsed.Seconds()
	}
}

// resetWork returns the tracker to a clean slate between tests.
func resetWork() {
	work.Lock()
	defer work.Unlock()
	work.reported, work.processed, work.changed, work.stages = false, 0, 0, nil
}

// workMetrics renders what ReportWork and Stage collected; empty when neither was used.
func workMetrics(labels string) string {
	work.Lock()
	defer work.Unlock()
	var b strings.Builder
	if work.reported {
		fmt.Fprintf(&b, `# HELP freehire_worker_last_run_items_processed Items the worker's last run looked at.
# TYPE freehire_worker_last_run_items_processed gauge
freehire_worker_last_run_items_processed{%[1]s} %[2]d
# HELP freehire_worker_last_run_items_changed Items the worker's last run wrote.
# TYPE freehire_worker_last_run_items_changed gauge
freehire_worker_last_run_items_changed{%[1]s} %[3]d
`, labels, work.processed, work.changed)
	}
	if len(work.stages) > 0 {
		names := make([]string, 0, len(work.stages))
		for name := range work.stages {
			names = append(names, name)
		}
		sort.Strings(names)
		b.WriteString("# HELP freehire_worker_stage_seconds How long each stage of the worker's last run took.\n# TYPE freehire_worker_stage_seconds gauge\n")
		for _, name := range names {
			fmt.Fprintf(&b, "freehire_worker_stage_seconds{%s,stage=%q} %f\n", labels, name, work.stages[name])
		}
	}
	return b.String()
}
