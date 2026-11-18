package worker

import (
	"context"
	"testing"
	"time"
)

func TestHoldPauseHoldsUntilReleased(t *testing.T) {
	mr, url := newPauseRedis(t)
	release, err := HoldPause(context.Background(), url, SearchDrainJob, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !Paused(context.Background(), url, SearchDrainJob) {
		t.Error("the worker must read as paused while held")
	}
	if got := mr.TTL(pauseKeyPrefix + SearchDrainJob); got <= 0 {
		t.Errorf("a hold must carry a TTL, or a dead holder leaves the worker off: %v", got)
	}
	release()
	release() // safe twice: a deferred release follows the explicit one
	if Paused(context.Background(), url, SearchDrainJob) {
		t.Error("release must lift the hold")
	}
}

func TestHoldPauseExpiresWhenTheHolderIsGone(t *testing.T) {
	mr, url := newPauseRedis(t)
	release, err := HoldPause(context.Background(), url, SearchDrainJob, 3*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	mr.FastForward(4 * time.Hour)
	if Paused(context.Background(), url, SearchDrainJob) {
		t.Error("an unrefreshed hold must expire")
	}
}

func TestHoldPauseRefreshesWhileHeld(t *testing.T) {
	mr, url := newPauseRedis(t)
	const ttl = 3 * time.Second // Redis rounds anything shorter than a second up
	release, err := HoldPause(context.Background(), url, SearchDrainJob, ttl)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	mr.SetTTL(pauseKeyPrefix+SearchDrainJob, time.Hour)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if mr.TTL(pauseKeyPrefix+SearchDrainJob) == ttl {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("a hold must keep refreshing its TTL while the holder runs")
}

func TestHoldPauseReportsAnUnreachableRedis(t *testing.T) {
	_, err := HoldPause(context.Background(), "redis://127.0.0.1:1/0", SearchDrainJob, time.Minute)
	if err == nil {
		t.Error("a failed hold must be reported, so the rebuild can log that the drain still runs")
	}
}
