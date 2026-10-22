package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/strelov1/freehire/internal/platform/worker"
)

// The exposition is a cross-repository contract: the stall alert in freehire-ops
// (increase(freehire_enrich_completed_total[2h]) < 300) cannot be compiled against
// this package, so the exact text is pinned here.
func TestRenderProgress(t *testing.T) {
	want := `# HELP freehire_enrich_completed_total Jobs whose enrichment this enrich process has written since it started.
# TYPE freehire_enrich_completed_total counter
freehire_enrich_completed_total 1234
`
	if got := renderProgress(1234); got != want {
		t.Errorf("renderProgress mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestProgressTextfileCannotBeOverwrittenByTheRunMetricsFile(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/enrich"}

	// worker.Main writes the run outcome AFTER run() returns, into a file named after
	// the binary. Sharing that name would let the final run file replace the counter.
	if progressTextfileName == worker.RunMetricsFilename() {
		t.Fatalf("progressTextfileName %q collides with the run-metrics file", progressTextfileName)
	}
}

func TestPublishProgress(t *testing.T) {
	dir := t.TempDir()
	publishProgress(dir, 7)
	b, err := os.ReadFile(filepath.Join(dir, progressTextfileName))
	if err != nil {
		t.Fatalf("read published file: %v", err)
	}
	if string(b) != renderProgress(7) {
		t.Errorf("published %q, want %q", b, renderProgress(7))
	}
}
