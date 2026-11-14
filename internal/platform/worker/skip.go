package worker

import "sync/atomic"

// skipReason holds the reason the current run recorded through Skip, or nil when it
// recorded none. It is written on run's goroutine and read once after run returns, so a
// plain variable would do; the atomic is here for the worker whose run() hands the work to
// a goroutine and returns from another.
var skipReason atomic.Pointer[string]

// Skip records that this run deliberately did nothing because the feature it serves is not
// configured, and returns the exit status such a run reports — always zero, see below.
//
// It exists because an unconfigured worker looks exactly like a working one. A worker that
// gates on a credential calls nothing, prints one line into a rotated journal and exits 0,
// so its timer stays green whether the credential is missing or the work is already done —
// and the journal line is the only thing that ever said which. Half this fleet has that
// gate ("a no-op that never opens the pool", in the doc comment above its run), and after
// a redeploy that drops an env var, or a credential that quietly went to an empty wallet,
// the first symptom is a feature nobody receives.
//
// reason names what is MISSING in the operator's own vocabulary — normally the
// environment variable, joined with "+" when the gate needs several — so the label on
// freehire_worker_skipped answers "what do I set?" without a second lookup. It is a bare
// token, never a sentence: it becomes a Prometheus label value, and every distinct value
// is a series.
//
// The exit status stays zero, deliberately. A skip is not a failure — it is how a feature
// ships before its credentials exist and how it is rolled back — so a non-zero code would
// page an operator for a deployment decision they just made. Whether a skip should count
// as health for ALERTING is a different question with a different owner: it belongs to the
// rules in freehire-ops, which can now ask the gauge instead of inferring from silence.
// Do NOT "fix" it by stamping a skip as a success in the last-run family either — that
// family already reported these runs green, which is the whole reason this exists.
func Skip(reason string) int {
	skipReason.Store(&reason)
	return 0
}

// loadSkipReason reports the reason recorded for the run that just finished, or "" when it
// recorded none. Read by the metrics writer, never cleared by it: runOrPause clears the
// mark before the next run, so a worker that runs twice in one process cannot report the
// first run's skip as the second's.
func loadSkipReason() string {
	if p := skipReason.Load(); p != nil {
		return *p
	}
	return ""
}
