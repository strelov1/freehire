package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A refusal must hold back EVERY request to that host, not just the one that drew it. This is
// the workable shape measured 2026-10-18: once its budget tripped, eight board workers each
// took a 429 and moved straight on to the next board, so the rest of the run (~1,500 boards)
// failed inside three minutes instead of waiting the window out.
func TestHostBackoffHoldsOtherRequestsToTheRefusingHost(t *testing.T) {
	b := newHostBackoff(40*time.Millisecond, time.Second)
	b.refused("api.example", 0)

	start := time.Now()
	if err := b.wait(context.Background(), "api.example"); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got := time.Since(start); got < 30*time.Millisecond {
		t.Errorf("waited %v, want ~40ms — a refused host must be held back", got)
	}

	start = time.Now()
	if err := b.wait(context.Background(), "other.example"); err != nil {
		t.Fatalf("wait other: %v", err)
	}
	if got := time.Since(start); got > 10*time.Millisecond {
		t.Errorf("an unrelated host waited %v, want no wait", got)
	}
}

// Refusals in a row double the hold, up to the cap; a served response resets the streak so one
// stray 429 hours later is not punished as if the storm never ended.
func TestHostBackoffEscalatesAndResets(t *testing.T) {
	b := newHostBackoff(10*time.Millisecond, 50*time.Millisecond)
	now := time.Now()
	b.now = func() time.Time { return now }

	want := []time.Duration{10, 20, 40, 50, 50}
	for i, w := range want {
		b.refused("h", 0)
		if got := b.until["h"].Sub(now); got != w*time.Millisecond {
			t.Fatalf("refusal %d: hold = %v, want %v", i+1, got, w*time.Millisecond)
		}
		now = now.Add(w * time.Millisecond) // the next refusal arrives as this hold expires
	}

	b.served("h")
	now = now.Add(time.Second) // the previous hold has expired
	b.refused("h", 0)
	if got := b.until["h"].Sub(now); got != 10*time.Millisecond {
		t.Errorf("after a success the hold = %v, want the base 10ms again", got)
	}
}

// The server's own Retry-After wins when it asks for longer than the escalation would, and the
// hold never shortens an earlier, longer one.
func TestHostBackoffHonoursRetryAfterAndNeverShortens(t *testing.T) {
	b := newHostBackoff(10*time.Millisecond, time.Minute)
	now := time.Now()
	b.now = func() time.Time { return now }

	b.refused("h", 5*time.Second)
	if got := b.until["h"].Sub(now); got != 5*time.Second {
		t.Fatalf("hold = %v, want the server's 5s", got)
	}
	b.refused("h", 0) // escalation alone would ask for 20ms
	if got := b.until["h"].Sub(now); got != 5*time.Second {
		t.Errorf("hold = %v, want the earlier 5s kept", got)
	}
}

func TestHostBackoffWaitHonoursCancellation(t *testing.T) {
	b := newHostBackoff(time.Hour, time.Hour)
	b.refused("h", 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.wait(ctx, "h"); err == nil {
		t.Fatal("wait on a cancelled context returned nil")
	}
}

// End to end through the Client: one worker's 429 makes a concurrent worker's request to the
// same host wait, instead of firing straight into the refusal.
func TestClientHoldsConcurrentRequestsAfterA429(t *testing.T) {
	var hits atomic.Int64
	var mu sync.Mutex
	var hitTimes []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hitTimes = append(hitTimes, time.Now())
		mu.Unlock()
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	hold := 60 * time.Millisecond
	c := &Client{httpClient: srv.Client(), maxRetries: 2, backoff: newHostBackoff(hold, time.Second)}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.GetJSON(context.Background(), srv.URL+"/a", &out); err != nil {
		t.Fatalf("first GetJSON: %v", err)
	}
	if err := c.GetJSON(context.Background(), srv.URL+"/b", &out); err != nil {
		t.Fatalf("second GetJSON: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3 (429, retry, second request)", got)
	}
	// The retry was served, which resets the escalation but cannot pull the hold in: every
	// request after the refusal went out at least `hold` after it.
	for i := 1; i < len(hitTimes); i++ {
		if gap := hitTimes[i].Sub(hitTimes[0]); gap < hold-10*time.Millisecond {
			t.Errorf("request %d went out %v after the 429, want >= %v", i+1, gap, hold)
		}
	}
}

// A burst is ONE refusal. Eight board workers with detail requests in flight all draw their
// 429 at the same moment; counting each would jump straight to the cap, and every worker would
// then fire together when it expired. Only a refusal arriving after the hold has run out is
// evidence the host is still refusing, so only that one escalates.
func TestHostBackoffBurstInsideOneHoldIsOneStep(t *testing.T) {
	b := newHostBackoff(10*time.Millisecond, time.Minute)
	now := time.Now()
	b.now = func() time.Time { return now }

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.refused("h", 0)
		}()
	}
	wg.Wait()
	if got := b.until["h"].Sub(now); got != 10*time.Millisecond {
		t.Fatalf("hold after a burst = %v, want the base 10ms", got)
	}

	now = now.Add(10 * time.Millisecond) // the hold expires, and the host refuses again
	b.refused("h", 0)
	if got := b.until["h"].Sub(now); got != 20*time.Millisecond {
		t.Errorf("hold after the next refusal = %v, want one doubling to 20ms", got)
	}
}

// The Client in cmd/server lives for days and meets many hosts; an expired hold must not stay
// in the map for ever.
func TestHostBackoffForgetsAnExpiredHold(t *testing.T) {
	b := newHostBackoff(10*time.Millisecond, time.Minute)
	now := time.Now()
	b.now = func() time.Time { return now }

	b.refused("h", 0)
	now = now.Add(time.Second)
	if err := b.wait(context.Background(), "h"); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if _, ok := b.until["h"]; ok {
		t.Error("an expired hold is still in the map")
	}
}

// The Client must tell the host's backoff when a request was served, or one recovered 429
// leaves a streak that lengthens every later refusal for the life of the process.
func TestClientServedResponseResetsHostEscalation(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), maxRetries: 2, backoff: newHostBackoff(time.Millisecond, time.Second)}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.GetJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2 (429 then 200)", got)
	}
	if got := len(c.backoff.streak); got != 0 {
		t.Errorf("streak entries after a served retry = %d, want 0", got)
	}
}
