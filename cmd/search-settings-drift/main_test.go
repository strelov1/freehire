package main

import (
	"os"
	"testing"

	"github.com/strelov1/freehire/internal/platform/worker"
)

func TestTextfileNameCannotBeOverwrittenByTheRunMetricsFile(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/search-settings-drift"}

	// worker.Main writes the run-outcome metrics AFTER run() returns, into a file
	// named after the binary. Publishing this worker's payload under that same name
	// would mean every run wrote the drift gauge and then immediately destroyed it —
	// the collector would only ever see the run-outcome file, and an alert on the
	// drift gauge would sit on no data forever while looking correctly configured.
	if textfileName == worker.RunMetricsFilename() {
		t.Fatalf("textfileName %q collides with the run-metrics file; the run metrics are written last and would overwrite this worker's payload", textfileName)
	}
}

func TestRunIsANoOpWithoutPromTextfileDir(t *testing.T) {
	t.Setenv(worker.PromTextfileDirEnv, "")
	if got := run(); got != 0 {
		t.Errorf("run() = %d, want 0 (a no-op with nowhere to publish must not fail)", got)
	}
}

func TestRunIsANoOpWithoutMeiliMasterKey(t *testing.T) {
	t.Setenv(worker.PromTextfileDirEnv, t.TempDir())
	t.Setenv("MEILI_MASTER_KEY", "")
	if got := run(); got != 0 {
		t.Errorf("run() = %d, want 0 (a no-op with nothing to check must not fail)", got)
	}
}
