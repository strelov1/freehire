package config

import "time"

// ApplyForm holds the tuning knobs for the application-form capture worker
// (cmd/capture-apply-form). It needs nothing but a database and the public ATS endpoints,
// so this is the whole configuration surface — the queue-drain knobs and the one bound
// that matters operationally, how hard the worker is allowed to lean on a platform.
type ApplyForm struct {
	BatchSize    int           // claim wave size
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before a capture is dead-lettered
	Concurrency  int           // how many postings are fetched at once
	MaxPerRun    int           // how much of the backlog one run takes; 0 is unbounded
	CallTimeout  time.Duration // bounds a single posting's fetch
}

// LoadApplyForm reads the worker's tuning from the environment, all optional with
// defaults. The defaults are deliberately modest: the first production drain faces a
// backlog of roughly 185k postings across two platforms, and a worker that empties it in
// one afternoon is a worker that got the crawl blocked.
//
// APPLY_FORM_MAX_PER_RUN is the bound that matters operationally, and it is separate from
// concurrency on purpose: concurrency decides how fast, this decides how long. Nothing in
// this fleet holds a lock, so a run that works for hours does not just take a while — a
// systemd Type=oneshot unit refuses to start a second instance while it is active, and
// every scheduled firing behind it is silently dropped. Set it to 0 for a supervised
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
	// A non-positive batch size would make the claim's LIMIT 0 (silently no-op) or feed a
	// negative LIMIT to Postgres; floor it so the worker always makes progress.
	if a.BatchSize < 1 {
		a.BatchSize = 1
	}
	if a.Concurrency < 1 {
		a.Concurrency = 1
	}
	// The lease must outlast a single fetch, or an in-flight capture becomes re-claimable
	// mid-flight (double work) and a lease of 0 re-claims a just-failed entry in a tight
	// loop, burning its whole retry budget in one run. Floor it to the per-call timeout —
	// the longest one capture can hold the lease. Same reasoning as config.LoadEmbed.
	if floor := int(a.CallTimeout.Seconds()); a.LeaseSeconds < floor {
		a.LeaseSeconds = floor
	}
	return a
}
