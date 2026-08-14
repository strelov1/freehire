package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunInstance(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, ""},
		{"bare board file", []string{"eightfold.yml"}, "eightfold"},
		{"path-like board file", []string{"/opt/freehire/src/hire-current/sources/eightfold.yml"}, "eightfold"},
		{"trailing flag after board file", []string{"sources/eightfold.yml", "--shard=1/6"}, "eightfold"},
		{"flag only", []string{"--posted-within", "168h"}, ""},
		{"end-of-options marker before board file", []string{"--", "sources/eightfold.yml"}, "eightfold"},
		{"end-of-options marker alone", []string{"--"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runInstance(tt.args); got != tt.want {
				t.Errorf("runInstance(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestWriteRunMetricsDisabledWithoutEnv(t *testing.T) {
	// Explicitly empty (equivalent to unset for the dir == "" check): the
	// write must be a no-op and, critically, must not touch os.Args[0]'s
	// actual directory or panic trying to resolve one.
	t.Setenv(PromTextfileDirEnv, "")
	writeRunMetrics(time.Second, 0)
}

func TestWriteRunMetricsWritesExpectedSeries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	// os.Args[0] drives the job label; restore it after the test.
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/ingest", "/opt/freehire/src/hire-current/sources/eightfold.yml"}

	writeRunMetrics(2500*time.Millisecond, 1)

	data, err := os.ReadFile(filepath.Join(dir, "ingest_eightfold.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		`freehire_worker_last_run_duration_seconds{job="ingest",instance="eightfold"} 2.500000`,
		`freehire_worker_last_run_success{job="ingest",instance="eightfold"} 0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("metrics file missing %q; got:\n%s", want, got)
		}
	}

	// No leftover .tmp file from the write-then-rename.
	if _, err := os.Stat(filepath.Join(dir, "ingest_eightfold.prom.tmp")); !os.IsNotExist(err) {
		t.Errorf("expected the .tmp file to be gone after rename, stat err: %v", err)
	}
}

func TestWriteRunMetricsWithoutInstance(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/embed"}

	writeRunMetrics(time.Millisecond, 0)

	data, err := os.ReadFile(filepath.Join(dir, "embed.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	if !strings.Contains(string(data), `freehire_worker_last_run_success{job="embed"} 1`) {
		t.Errorf("metrics file missing job-only series; got:\n%s", string(data))
	}
}
