package config

import "time"

// AdzunaDescription holds the tuning knobs for the Adzuna full-description capture worker
// (cmd/hydrate-adzuna-description). It needs nothing but a database and Adzuna's own site,
// so this is the whole configuration surface — mirrors config.ApplyForm's shape.
type AdzunaDescription struct {
	BatchSize    int           // claim wave size
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before a capture is dead-lettered
	Concurrency  int           // how many postings are fetched at once
	MaxPerRun    int           // how much of the backlog one run takes; 0 is unbounded
	CallTimeout  time.Duration // bounds a single posting's fetch
}

// LoadAdzunaDescription reads the worker's tuning from the environment, all optional with
// defaults.
//
// The defaults are deliberately modest and unproven: unlike config.LoadApplyForm, which
// tunes against a measured production backlog, nothing here has been run against Adzuna's
// site at real ingest volume yet — the feasibility spike (2026-08-08) confirmed the ad-
// network tracking-redirect URLs answer their own "Access Denied" page to a plain request,
// but never exercised sustained traffic from the production host. ADZUNA_DESCRIPTION_MAX_PER_RUN
// bounds how much of a run gets spent finding that out the hard way; raise it once a live
// run shows Adzuna tolerating the pace.
func LoadAdzunaDescription() AdzunaDescription {
	a := AdzunaDescription{
		BatchSize:    envInt("ADZUNA_DESCRIPTION_BATCH_SIZE", 50),
		LeaseSeconds: envInt("ADZUNA_DESCRIPTION_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("ADZUNA_DESCRIPTION_MAX_ATTEMPTS", 3),
		Concurrency:  envInt("ADZUNA_DESCRIPTION_CONCURRENCY", 2),
		MaxPerRun:    envInt("ADZUNA_DESCRIPTION_MAX_PER_RUN", 500),
		CallTimeout:  time.Duration(envInt("ADZUNA_DESCRIPTION_CALL_TIMEOUT_SECONDS", 20)) * time.Second,
	}
	if a.BatchSize < 1 {
		a.BatchSize = 1
	}
	if a.Concurrency < 1 {
		a.Concurrency = 1
	}
	// The lease must outlast a single fetch, or an in-flight capture becomes re-claimable
	// mid-flight. Same reasoning as config.LoadApplyForm.
	if floor := int(a.CallTimeout.Seconds()); a.LeaseSeconds < floor {
		a.LeaseSeconds = floor
	}
	return a
}
