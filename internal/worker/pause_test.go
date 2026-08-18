package worker

import (
	"bytes"
	"context"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// newPauseRedis starts an in-process Redis and returns its URL in the form
// config.Settings.RedisURL carries, so the tests exercise the same parsing path
// a worker takes.
func newPauseRedis(t *testing.T) (*miniredis.Miniredis, string) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return mr, "redis://" + mr.Addr() + "/0"
}

// hold sets a pause key the way an operator would.
func hold(t *testing.T, mr *miniredis.Miniredis, key string) {
	t.Helper()
	if err := mr.Set(key, "1"); err != nil {
		t.Fatalf("miniredis.Set %s: %v", key, err)
	}
}

func TestPausedRunsWhenNoKeyIsSet(t *testing.T) {
	_, url := newPauseRedis(t)

	if Paused(context.Background(), url, "ingest") {
		t.Error("Paused = true with no key set, want false")
	}
}

func TestPausedHoldsOnTheFleetKey(t *testing.T) {
	mr, url := newPauseRedis(t)
	hold(t, mr, "freehire:pause:all")

	if !Paused(context.Background(), url, "ingest") {
		t.Error("Paused = false while freehire:pause:all is set, want true")
	}
}

func TestPausedPerBinaryKeyHoldsOnlyThatBinary(t *testing.T) {
	mr, url := newPauseRedis(t)
	hold(t, mr, "freehire:pause:ingest")

	if !Paused(context.Background(), url, "ingest") {
		t.Error("ingest: Paused = false while its own key is set, want true")
	}
	if Paused(context.Background(), url, "search-drain") {
		t.Error("search-drain: Paused = true while only ingest is held, want false")
	}
}

func TestPausedOverrideAdmitsAHandStartedRun(t *testing.T) {
	mr, url := newPauseRedis(t)
	hold(t, mr, "freehire:pause:all")
	t.Setenv(IgnorePauseEnv, "1")

	if Paused(context.Background(), url, "backfill-derive") {
		t.Errorf("Paused = true with %s set, want false", IgnorePauseEnv)
	}
}

// A falsy value must not read as "bypass". Under pressure an operator who typed
// FREEHIRE_IGNORE_PAUSE=0 means the opposite of a bypass, and presence-alone
// semantics here would silently run the fleet they just held.
func TestPausedOverrideRejectsAFalsyValue(t *testing.T) {
	mr, url := newPauseRedis(t)
	hold(t, mr, "freehire:pause:all")
	t.Setenv(IgnorePauseEnv, "0")

	if !Paused(context.Background(), url, "backfill-derive") {
		t.Errorf("Paused = false with %s=0, want true", IgnorePauseEnv)
	}
}

// captureLog redirects the standard logger for one test and returns what was
// written, so the fail-open paths can be checked to announce themselves rather
// than degrading in silence.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})
	return &buf
}

func TestPausedRunsWhenRedisIsUnreachable(t *testing.T) {
	mr, url := newPauseRedis(t)
	mr.Close() // the address is now refused
	logs := captureLog(t)

	if Paused(context.Background(), url, "ingest") {
		t.Error("Paused = true against an unreachable Redis, want false (fail open)")
	}
	if logs.Len() == 0 {
		t.Error("an unreachable switch was not logged; a silent degrade is indistinguishable from a healthy read")
	}
}

func TestPausedRunsOnAMalformedURL(t *testing.T) {
	logs := captureLog(t)

	if Paused(context.Background(), "not-a-redis-url", "ingest") {
		t.Error("Paused = true on a malformed URL, want false (fail open)")
	}
	if logs.Len() == 0 {
		t.Error("a malformed switch URL was not logged")
	}
}

// A reachable-but-silent Redis is the case a plain fail-open misses: without a
// bound the worker waits out go-redis's dial/read default on every start.
func TestPausedGivesUpOnASilentRedis(t *testing.T) {
	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		var conns []net.Conn
		defer func() {
			for _, conn := range conns {
				_ = conn.Close()
			}
		}()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conns = append(conns, conn) // accept, and never answer
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-accepted
	})
	captureLog(t)

	start := time.Now()
	paused := Paused(context.Background(), "redis://"+listener.Addr().String()+"/0", "ingest")
	elapsed := time.Since(start)

	if paused {
		t.Error("Paused = true against a silent Redis, want false (fail open)")
	}
	// Well under go-redis's 5s dial default (the bound this guards), and well
	// over the 250ms budget, so the assertion is meaningful without being timing-
	// flaky on a loaded machine.
	if elapsed > 2*time.Second {
		t.Errorf("Paused took %s against a silent Redis; the lookup is not bounded", elapsed)
	}
}

// The per-binary key names the binary and nothing else, so one key holds the
// whole same-binary fleet — the ~140 ingest boards are released together rather
// than needing a key each.
func TestWorkerJobIgnoresInvocationArguments(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/ingest", "sources/greenhouse.yml"}

	if got := workerJob(); got != "ingest" {
		t.Errorf("workerJob() = %q, want %q", got, "ingest")
	}
}
