package main

import (
	"os"
	"testing"

	"github.com/strelov1/freehire/internal/worker"
)

func TestTextfileNameCannotBeOverwrittenByTheRunMetricsFile(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/queue-metrics"}

	// worker.Main writes the run-outcome metrics AFTER run() returns, into a file
	// named after the binary. Publishing this worker's payload under that same name
	// would mean every run wrote the queue gauges and then immediately destroyed
	// them — the collector would only ever see the run-outcome file, and the queue
	// alerts would sit on no data forever while looking correctly configured.
	if textfileName == worker.RunMetricsFilename() {
		t.Fatalf("textfileName %q collides with the run-metrics file; the run metrics are written last and would overwrite this worker's payload", textfileName)
	}
}
