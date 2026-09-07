package config

import (
	"os"
	"time"
)

// AutoApply holds the tuning knobs for the unattended application-submission worker
// (cmd/auto-apply). Unlike ApplyForm's fetch (one HTTP call to a public JSON API), one
// attempt here is a full headless-browser session — scan, reconcile, resolve, and maybe
// fill and submit — so its call timeout is an order of magnitude more generous, and the
// lease has to cover the same session, not a single request.
//
// No sidecar address here: internal/atsapply drives the browser in-process (see design.md's
// "chromedp, not a Python/Patchright sidecar" decision) — there is no second process to
// address. BrowserUseAPIKey is not an exception to that: it is a credential for a cloud
// REST API called via plain Go HTTP (openspec/changes/add-browseruse-atsapply-fallback),
// not a second local process either.
//
// No drafting-specific knobs either: question drafting (openspec/changes/
// auto-apply-llm-drafting) rides the same LLM_*/LLM_ADMIN_* environment every other
// feature reads, attributed per candidate under auto-apply-drafting — an
// unconfigured LLM just means every attempt's unmapped free-text questions stay parked,
// the same degrade every other LLM feature already has.
type AutoApply struct {
	BatchSize    int           // claim wave size
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // transient failures before an attempt is dead-lettered
	Concurrency  int           // how many attempts run at once
	MaxPerRun    int           // how much of the queue one run takes; 0 is unbounded
	CallTimeout  time.Duration // bounds a single attempt's browser session
	// BrowserUseAPIKey authenticates the browser-use.com cloud fallback (Ashby/Workable
	// only — Recruitee has no registered applyform.Fetcher and never reaches this
	// executor; see internal/api/atsapply/browseruse_fill.go). Empty disables the
	// fallback entirely: those providers park exactly as they did before it existed. Its
	// own enforce flag, per-run cost cap and per-process-run spend cap (the latter is NOT
	// a calendar-day cap — a fresh one is built on every cmd/auto-apply invocation)
	// (AUTO_APPLY_BROWSERUSE_ENFORCE/_MAX_COST_USD/_RUN_CAP_USD) are read directly by
	// that package rather than threaded through here, matching AUTO_APPLY_ELIGIBILITY_ENFORCE's
	// own plain-env-read convention in internal/api/handler.
	BrowserUseAPIKey string
}

// LoadAutoApply reads the worker's tuning from the environment, all optional with
// defaults. The defaults are deliberately conservative — this is the first worker in the
// fleet that drives a real browser per item, and a submission is a real side effect against
// a third party, unlike a capture's read-only fetch.
func LoadAutoApply() AutoApply {
	a := AutoApply{
		BatchSize:    envInt("AUTO_APPLY_BATCH_SIZE", 20),
		LeaseSeconds: envInt("AUTO_APPLY_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("AUTO_APPLY_MAX_ATTEMPTS", 3),
		Concurrency:  envInt("AUTO_APPLY_CONCURRENCY", 2),
		MaxPerRun:    envInt("AUTO_APPLY_MAX_PER_RUN", 200),
		CallTimeout:  time.Duration(envInt("AUTO_APPLY_CALL_TIMEOUT_SECONDS", 120)) * time.Second,

		BrowserUseAPIKey: os.Getenv("AUTO_APPLY_BROWSERUSE_API_KEY"),
	}
	if a.BatchSize < 1 {
		a.BatchSize = 1
	}
	if a.Concurrency < 1 {
		a.Concurrency = 1
	}
	// The lease must outlast one full browser session, the same reasoning
	// config.LoadApplyForm applies to a single fetch — a lease shorter than the call
	// timeout makes an in-flight attempt reclaimable mid-session, and a lease of 0
	// re-claims a just-failed entry in a tight loop, burning its retry budget in one run.
	if floor := int(a.CallTimeout.Seconds()); a.LeaseSeconds < floor {
		a.LeaseSeconds = floor
	}
	return a
}
