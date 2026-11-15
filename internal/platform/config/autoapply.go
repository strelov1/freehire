package config

import (
	"os"
	"time"
)

// AutoApply holds the tuning knobs for the unattended application-submission worker
// (cmd/auto-apply). One attempt is a full headless-browser session, so the call timeout is
// far more generous than ApplyForm's and the lease must cover the whole session.
//
// There is no sidecar address: internal/atsapply drives the browser in-process.
// BrowserUseAPIKey is a credential for a cloud REST API, not a second process.
//
// There are no drafting knobs: question drafting uses the shared LLM_*/LLM_ADMIN_*
// environment, attributed per candidate under auto-apply-drafting. Without an LLM, unmapped
// free-text questions stay parked.
type AutoApply struct {
	BatchSize    int // claim wave size
	LeaseSeconds int // how long a claim is held before it can be reclaimed
	MaxAttempts  int // transient failures before an attempt is dead-lettered
	Concurrency  int // how many attempts run at once
	MaxPerRun    int // how much of the queue one run takes; 0 is unbounded
	// CallTimeout bounds a single attempt end to end. It must OUTLAST every path an attempt
	// can take: if it fires first, a still-running submission is reported unconfirmed and
	// dead-lettered although it may have submitted. Cloud runs take up to ~6 minutes; a
	// chromedp session is bounded by its own timeouts.
	CallTimeout time.Duration
	// BrowserUseAPIKey authenticates the browser-use.com cloud fallback (Ashby/Workable
	// only; see internal/api/atsapply/browseruse_fill.go). Empty disables it and those
	// providers park. Its enforce flag and cost caps (AUTO_APPLY_BROWSERUSE_ENFORCE/
	// _MAX_COST_USD/_RUN_CAP_USD) are read directly by that package; the run cap is per
	// cmd/auto-apply invocation, not per calendar day.
	BrowserUseAPIKey string
}

// LoadAutoApply reads the worker's tuning from the environment, all optional with
// defaults. The defaults are conservative: a submission is a real side effect against a
// third party.
func LoadAutoApply() AutoApply {
	a := AutoApply{
		BatchSize:    envInt("AUTO_APPLY_BATCH_SIZE", 20),
		LeaseSeconds: envInt("AUTO_APPLY_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("AUTO_APPLY_MAX_ATTEMPTS", 3),
		Concurrency:  envInt("AUTO_APPLY_CONCURRENCY", 2),
		MaxPerRun:    envInt("AUTO_APPLY_MAX_PER_RUN", 200),
		CallTimeout:  time.Duration(envInt("AUTO_APPLY_CALL_TIMEOUT_SECONDS", 720)) * time.Second,

		BrowserUseAPIKey: os.Getenv("AUTO_APPLY_BROWSERUSE_API_KEY"),
	}
	if a.BatchSize < 1 {
		a.BatchSize = 1
	}
	if a.Concurrency < 1 {
		a.Concurrency = 1
	}
	// The lease must outlast one full browser session, or an in-flight attempt is
	// reclaimable mid-session (and a lease of 0 burns retries in a tight loop).
	if floor := int(a.CallTimeout.Seconds()); a.LeaseSeconds < floor {
		a.LeaseSeconds = floor
	}
	return a
}
