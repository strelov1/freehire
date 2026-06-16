package worker

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestHeartbeatFiresUntilStopped(t *testing.T) {
	var calls atomic.Int64
	stop := Heartbeat(15*time.Millisecond, func() { calls.Add(1) })

	time.Sleep(80 * time.Millisecond)
	stop()

	fired := calls.Load()
	if fired < 2 {
		t.Fatalf("expected the heartbeat to fire at least twice in ~80ms, got %d", fired)
	}

	// After stop, the ticker goroutine must be gone — no further calls.
	time.Sleep(50 * time.Millisecond)
	if after := calls.Load(); after != fired {
		t.Fatalf("heartbeat kept firing after stop: %d -> %d", fired, after)
	}
}
