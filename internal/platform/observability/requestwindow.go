package observability

import (
	"sync"
	"time"
)

// requestWindow tracks a short rolling window of total/5xx request counts,
// bucketed per minute, so a caller can answer "what fraction of requests
// failed in the last N minutes" without querying Prometheus.
//
// It exists for the public /api/v1/status page: unlike the Prometheus
// counters above (persistent, scraped externally into a separate system),
// this state is deliberately in-process and reset on deploy — the page is
// reporting on the process serving it, not a historical record, and that
// process is by definition live if it's the one answering the request.
type requestWindow struct {
	mu      sync.Mutex
	buckets []requestBucket
}

type requestBucket struct {
	minute time.Time
	total  int64
	errors int64
}

// maxBucketAge bounds how long a bucket is kept, independent of the caller's
// own query window. errorRate() prunes on every call, which is enough as
// long as something polls it — but pruning ONLY there means a window that
// goes unpolled (a misconfigured monitor, a deprioritized endpoint) grows
// buckets forever while requests keep arriving. record() enforces this same
// ceiling on every write so the window is bounded either way. Generous
// relative to the 10-minute window /api/v1/status actually queries, so it
// never trims data a real caller still wants.
const maxBucketAge = time.Hour

// site is the process-wide window the status handler reads. A single
// instance is enough: like the Prometheus counters, request accounting is
// global to the process, not scoped per caller.
var site = &requestWindow{}

// RecordRequest tallies one response by its HTTP status code (>=500 counts
// as an error) into the current minute's bucket of the process-wide window.
func RecordRequest(status int) {
	site.record(status, time.Now())
}

// ErrorRate reports the fraction of requests that were 5xx over the trailing
// window (rounded down to whole minutes) as of now, plus the total request
// count it was computed from, from the process-wide window.
func ErrorRate(window time.Duration) (rate float64, total int64) {
	return site.errorRate(window, time.Now())
}

func (w *requestWindow) record(status int, now time.Time) {
	minute := now.Truncate(time.Minute)

	w.mu.Lock()
	defer w.mu.Unlock()

	w.pruneLocked(now.Add(-maxBucketAge).Truncate(time.Minute))

	if n := len(w.buckets); n == 0 || !w.buckets[n-1].minute.Equal(minute) {
		w.buckets = append(w.buckets, requestBucket{minute: minute})
	}
	b := &w.buckets[len(w.buckets)-1]
	b.total++
	if status >= 500 {
		b.errors++
	}
}

// errorRate folds every bucket at or after (now - window) into a rate. A
// zero total (no traffic yet) reports a zero rate rather than dividing by
// zero.
func (w *requestWindow) errorRate(window time.Duration, now time.Time) (rate float64, total int64) {
	cutoff := now.Add(-window).Truncate(time.Minute)

	w.mu.Lock()
	defer w.mu.Unlock()

	w.pruneLocked(cutoff)
	var errs int64
	for _, b := range w.buckets {
		total += b.total
		errs += b.errors
	}

	if total == 0 {
		return 0, 0
	}
	return float64(errs) / float64(total), total
}

// pruneLocked drops every bucket older than cutoff. Callers must hold w.mu.
// Shared by record() (bounding memory even when nothing ever reads the
// window) and errorRate() (which additionally uses the query's own,
// narrower window as its cutoff).
func (w *requestWindow) pruneLocked(cutoff time.Time) {
	kept := w.buckets[:0]
	for _, b := range w.buckets {
		if b.minute.Before(cutoff) {
			continue
		}
		kept = append(kept, b)
	}
	w.buckets = kept
}
