package observability

import (
	"testing"
	"time"
)

func TestRequestWindow_NoTrafficReportsZeroRate(t *testing.T) {
	w := &requestWindow{}

	rate, total := w.errorRate(5*time.Minute, time.Now())

	if rate != 0 || total != 0 {
		t.Errorf("errorRate() = (%v, %v), want (0, 0) with no recorded requests", rate, total)
	}
}

func TestRequestWindow_ComputesErrorFractionWithinWindow(t *testing.T) {
	w := &requestWindow{}
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	statuses := []int{200, 200, 200, 200, 200, 200, 500, 503}
	for _, s := range statuses {
		w.record(s, now)
	}

	rate, total := w.errorRate(5*time.Minute, now)

	if total != 8 {
		t.Fatalf("total = %d, want 8", total)
	}
	if want := 0.25; rate != want {
		t.Errorf("rate = %v, want %v (2 of 8 were 5xx)", rate, want)
	}
}

func TestRequestWindow_ExcludesRequestsOutsideWindow(t *testing.T) {
	w := &requestWindow{}
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	// An old 5xx well outside the trailing window must not count against the
	// current rate — otherwise a single past incident would haunt the page forever.
	w.record(500, now.Add(-10*time.Minute))
	w.record(200, now)
	w.record(200, now)
	w.record(200, now)

	rate, total := w.errorRate(5*time.Minute, now)

	if total != 3 {
		t.Fatalf("total = %d, want 3 (old request excluded)", total)
	}
	if rate != 0 {
		t.Errorf("rate = %v, want 0 (old 5xx is outside the window)", rate)
	}
}

func TestRequestWindow_RecordBoundsMemoryWithoutReads(t *testing.T) {
	w := &requestWindow{}
	start := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)

	// Simulate five hours of steady traffic with errorRate() never called (the
	// case that would starve pruning if record() didn't do any of its own):
	// one request per minute for 300 minutes.
	for i := 0; i < 300; i++ {
		w.record(200, start.Add(time.Duration(i)*time.Minute))
	}

	w.mu.Lock()
	got := len(w.buckets)
	w.mu.Unlock()

	if want := int(maxBucketAge/time.Minute) + 1; got > want {
		t.Errorf("buckets held after 300 minutes of traffic = %d, want bounded to roughly maxBucketAge (%v) worth of minutes — record() must prune, not only errorRate()", got, maxBucketAge)
	}
}

func TestRecordRequest_FeedsTheDefaultWindow(t *testing.T) {
	// A convenience-function smoke test against the package-level singleton the
	// HTTP middleware actually calls. Real time.Now() is fine here: both the
	// record and the read land in the same minute bucket.
	RecordRequest(500)
	RecordRequest(200)

	rate, total := ErrorRate(time.Minute)

	if total == 0 {
		t.Fatal("total = 0, want at least the two requests just recorded")
	}
	if rate <= 0 {
		t.Errorf("rate = %v, want > 0 (at least one 5xx was recorded)", rate)
	}
}
