package worker

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestHeartbeatStopWaitsForReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		stop := Heartbeat(time.Second, func() {
			close(started)
			<-release
		})
		<-started

		stopped := make(chan struct{})
		go func() {
			stop()
			close(stopped)
		}()
		synctest.Wait()
		select {
		case <-stopped:
			t.Error("stop returned while the report callback was still running")
		default:
		}

		close(release)
		<-stopped
	})
}

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
