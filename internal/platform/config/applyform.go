package config

import "time"

// ApplyForm holds the tuning knobs for the application-form capture worker
// (cmd/capture-apply-form): the queue-drain knobs and how hard the worker may lean on a
// platform.
type ApplyForm struct {
	BatchSize    int           // claim wave size
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before a capture is dead-lettered
	Concurrency  int           // how many postings are fetched at once
	MaxPerRun    int           // how much of the backlog one run takes; 0 is unbounded
	CallTimeout  time.Duration // bounds a single posting's fetch
}

// LoadApplyForm reads the worker's tuning from the environment, all optional with
// defaults. The defaults are modest: a worker that drains the backlog too fast gets the
// crawl blocked.
//
// APPLY_FORM_MAX_PER_RUN is separate from concurrency: concurrency decides how fast, this
// how long. A systemd Type=oneshot unit refuses a second instance while active, so every
// scheduled firing behind a long run is silently dropped. Set it to 0 for a supervised
// one-off catch-up.
func LoadApplyForm() ApplyForm {
	a := ApplyForm{
		BatchSize:    envInt("APPLY_FORM_BATCH_SIZE", 200),
		LeaseSeconds: envInt("APPLY_FORM_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("APPLY_FORM_MAX_ATTEMPTS", 3),
		Concurrency:  envInt("APPLY_FORM_CONCURRENCY", 4),
		MaxPerRun:    envInt("APPLY_FORM_MAX_PER_RUN", 5000),
		CallTimeout:  time.Duration(envInt("APPLY_FORM_CALL_TIMEOUT_SECONDS", 20)) * time.Second,
	}
	// A non-positive batch size makes the claim's LIMIT 0 (no-op) or negative; floor it.
	if a.BatchSize < 1 {
		a.BatchSize = 1
	}
	if a.Concurrency < 1 {
		a.Concurrency = 1
	}
	// The lease must outlast a single fetch, or an in-flight capture is re-claimed (double
	// work) and a lease of 0 re-claims a failed entry in a tight loop, burning its retries.
	// Floor it to the per-call timeout.
	if floor := int(a.CallTimeout.Seconds()); a.LeaseSeconds < floor {
		a.LeaseSeconds = floor
	}
	return a
}
