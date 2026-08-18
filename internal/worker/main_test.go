package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// recordingTransport is a sentry.Transport that captures sent events in memory so
// a test can assert what would have been delivered without any network.
type recordingTransport struct{ events []*sentry.Event }

func (t *recordingTransport) Configure(sentry.ClientOptions)        {}
func (t *recordingTransport) SendEvent(e *sentry.Event)             { t.events = append(t.events, e) }
func (t *recordingTransport) Flush(time.Duration) bool              { return true }
func (t *recordingTransport) FlushWithContext(context.Context) bool { return true }
func (t *recordingTransport) Close()                                {}

// A held switch must stop the work itself, not merely tidy up after it: the run
// function is the thing that opens pools and crawls boards, so "paused" is only
// true if it is never called.
func TestRunOrPauseSkipsTheRunWhenHeld(t *testing.T) {
	mr, url := newPauseRedis(t)
	hold(t, mr, "freehire:pause:all")
	t.Setenv("REDIS_URL", url)
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/ingest", "sources/eightfold.yml"}

	called := false
	code := runOrPause(func() int {
		called = true
		return 3
	})

	if called {
		t.Error("the run function was called while the switch was held")
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0 — a pause is an operator's decision, not a failure", code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ingest_eightfold.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	if !strings.Contains(string(data), `freehire_worker_paused{job="ingest",instance="eightfold"} 1`) {
		t.Errorf("a refused run did not publish the paused gauge; got:\n%s", string(data))
	}
}

func TestRunOrPauseRunsAndReportsWhenClear(t *testing.T) {
	_, url := newPauseRedis(t)
	t.Setenv("REDIS_URL", url)
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/embed"}

	code := runOrPause(func() int { return 3 })

	if code != 3 {
		t.Errorf("exit code = %d, want 3 — the run's own status must survive", code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "embed.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	if !strings.Contains(string(data), `freehire_worker_last_run_success{job="embed"} 0`) {
		t.Errorf("a completed run did not publish its outcome; got:\n%s", string(data))
	}
}

// A panic flowing through the worker guard must be reported to Sentry AND
// re-raised, so the process still crashes with the original stack and non-zero exit.
func TestCapturePanicReportsAndRepanics(t *testing.T) {
	tr := &recordingTransport{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://public@o0.ingest.sentry.io/0",
		Transport: tr,
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("capturePanic swallowed the panic; want it re-raised")
		}
		if len(tr.events) != 1 {
			t.Fatalf("captured %d events, want exactly 1", len(tr.events))
		}
	}()

	func() {
		defer capturePanic()
		panic("boom")
	}()
}
