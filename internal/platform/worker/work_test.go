package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkMetricsEmptyWhenNotReported(t *testing.T) {
	resetWork()
	if got := workMetrics(`job="x"`); got != "" {
		t.Errorf("a worker that reports nothing must publish nothing, got %q", got)
	}
}

func TestReportWorkAccumulates(t *testing.T) {
	resetWork()
	t.Cleanup(resetWork)
	ReportWork(10, 1)
	ReportWork(5, 0)
	got := workMetrics(`job="x"`)
	for _, want := range []string{
		`freehire_worker_last_run_items_processed{job="x"} 15`,
		`freehire_worker_last_run_items_changed{job="x"} 1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "freehire_worker_stage_seconds") {
		t.Errorf("no stage ran, so no stage series:\n%s", got)
	}
}

func TestStageAccumulatesAndSortsAndSkipsUnfinished(t *testing.T) {
	resetWork()
	t.Cleanup(resetWork)
	Stage("load")()
	Stage("load")()
	Stage("build")()
	_ = Stage("never finished")
	work.stages["load"] = 2.5 // elapsed time is not reproducible
	work.stages["build"] = 1
	got := workMetrics(`job="x"`)
	build := strings.Index(got, `stage="build"`)
	load := strings.Index(got, `stage="load"`)
	if build < 0 || load < build {
		t.Errorf("stages must be present and sorted for a stable file:\n%s", got)
	}
	if !strings.Contains(got, `freehire_worker_stage_seconds{job="x",stage="load"} 2.500000`) {
		t.Errorf("wrong value in:\n%s", got)
	}
	if strings.Contains(got, "never finished") {
		t.Errorf("an unfinished stage must not be published:\n%s", got)
	}
	if strings.Count(got, "# TYPE freehire_worker_stage_seconds") != 1 {
		t.Errorf("one TYPE line per metric, or node_exporter rejects the file:\n%s", got)
	}
}

func TestWriteRunMetricsIncludesWork(t *testing.T) {
	resetWork()
	t.Cleanup(resetWork)
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)
	ReportWork(3, 2)
	Stage("load")()
	writeRunMetrics(time.Second, 0)
	body, err := os.ReadFile(filepath.Join(dir, RunMetricsFilename()))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"freehire_worker_last_run_duration_seconds", "items_processed", "freehire_worker_stage_seconds"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
}
