//go:build integration

// Integration tests for the durable tailor-then-review Inngest function
// (openspec/changes/auto-apply-inngest-orchestration): a real Inngest dev server
// (testcontainers) plus a fake hire HTTP server standing in for the two auto-apply
// routes. Run with: go test -tags=integration ./internal/application/autoapplyorchestrate/
package autoapplyorchestrate

import (
	"net/http"
	"testing"
	"time"
)

// pollUntil polls cond every 100ms until it reports true or timeout elapses, failing the
// test on timeout. Every assertion in this file waits on an async Inngest run rather than
// a fixed sleep, so a fast run does not pay for a slow one's margin.
func pollUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestOrchestrate_SubmitCallsTailorWithTheSecretAndTheRightPath(t *testing.T) {
	hire := newFakeHire()
	hireServer := hire.server()
	t.Cleanup(hireServer.Close)

	dev := startDevServer(t, Config{HireBaseURL: hireServer.URL, Secret: "test-orchestrator-secret"})

	dev.sendEvent(t, EventSubmit, SubmitEvent{QueueID: "42"})

	pollUntil(t, 20*time.Second, func() bool {
		for _, c := range hire.Calls() {
			if c.path == "/me/auto-apply/42/tailor" {
				return true
			}
		}
		return false
	})

	calls := hire.Calls()
	if len(calls) != 1 {
		t.Fatalf("hire received %d calls, want exactly 1 (the tailor call, before any review)", len(calls))
	}
	if calls[0].auth != "Bearer test-orchestrator-secret" {
		t.Errorf("Authorization = %q, want the configured secret as a Bearer credential", calls[0].auth)
	}
}

// countCalls reports how many recorded calls match path.
func countCalls(calls []recordedCall, path string) int {
	n := 0
	for _, c := range calls {
		if c.path == path {
			n++
		}
	}
	return n
}

func TestOrchestrate_TailorFailureNeverCallsReview(t *testing.T) {
	hire := newFakeHire()
	hire.setTailorStatus("99", http.StatusInternalServerError)
	hireServer := hire.server()
	t.Cleanup(hireServer.Close)

	dev := startDevServer(t, Config{HireBaseURL: hireServer.URL, Secret: "s"})
	dev.sendEvent(t, EventSubmit, SubmitEvent{QueueID: "99"})

	pollUntil(t, 20*time.Second, func() bool {
		return countCalls(hire.Calls(), "/me/auto-apply/99/tailor") == 1
	})

	// The run ends on the failed tailor step; give it a beat to (not) proceed, then
	// confirm review was never called.
	time.Sleep(2 * time.Second)
	if n := countCalls(hire.Calls(), "/me/auto-apply/99/review"); n != 0 {
		t.Errorf("review calls = %d, want 0 — a failed tailor call must never reach the review step", n)
	}
}

// TestOrchestrate_ResumesOnDecisionWithoutCallingReviewAgain guards the bug a code review
// found: PostAutoApplyReview (the only publisher of EventReviewDecided) records the
// decision BEFORE it publishes, so by the time this resumes, the decision is already
// durably recorded — a second call to /review would always 409 (already reviewed). Every
// real run hit exactly this until the fix; the previous version of this test asserted the
// erroneous call as the expected behavior.
func TestOrchestrate_ResumesOnDecisionWithoutCallingReviewAgain(t *testing.T) {
	hire := newFakeHire()
	hireServer := hire.server()
	t.Cleanup(hireServer.Close)

	dev := startDevServer(t, Config{HireBaseURL: hireServer.URL, Secret: "s"})
	dev.sendEvent(t, EventSubmit, SubmitEvent{QueueID: "7"})

	pollUntil(t, 20*time.Second, func() bool {
		return countCalls(hire.Calls(), "/me/auto-apply/7/tailor") == 1
	})

	// No review call yet — the run must be paused, not racing ahead.
	time.Sleep(2 * time.Second)
	if n := countCalls(hire.Calls(), "/me/auto-apply/7/review"); n != 0 {
		t.Fatalf("review calls = %d, want 0 before any decision event is published", n)
	}

	dev.sendEvent(t, EventReviewDecided, ReviewDecidedEvent{QueueID: "7", Decision: "approved"})

	// Give the resumed run a beat to run its course, then confirm it never called
	// /review — the decision it just received was already recorded by whoever
	// published the event.
	time.Sleep(3 * time.Second)
	if n := countCalls(hire.Calls(), "/me/auto-apply/7/review"); n != 0 {
		t.Errorf("review calls after the decision event = %d, want 0", n)
	}
}

func TestOrchestrate_DecisionForADifferentQueueIDDoesNotResumeThisRun(t *testing.T) {
	hire := newFakeHire()
	hireServer := hire.server()
	t.Cleanup(hireServer.Close)

	dev := startDevServer(t, Config{HireBaseURL: hireServer.URL, Secret: "s"})
	dev.sendEvent(t, EventSubmit, SubmitEvent{QueueID: "10"})

	pollUntil(t, 20*time.Second, func() bool {
		return countCalls(hire.Calls(), "/me/auto-apply/10/tailor") == 1
	})
	// Give the executor a moment to actually register the WaitForEvent listener before
	// publishing anything — the tailor HTTP call landing at the fake server doesn't
	// guarantee the executor has advanced its own state machine past it yet.
	time.Sleep(2 * time.Second)

	// A decision for a DIFFERENT entry must not resume entry 10's own paused run.
	dev.sendEvent(t, EventReviewDecided, ReviewDecidedEvent{QueueID: "11", Decision: "approved"})

	time.Sleep(3 * time.Second)
	if n := countCalls(hire.Calls(), "/me/auto-apply/10/review"); n != 0 {
		t.Errorf("review calls for entry 10 = %d, want 0 — a decision for entry 11 must not resume it", n)
	}
	if n := countCalls(hire.Calls(), "/me/auto-apply/11/review"); n != 0 {
		t.Errorf("review calls for entry 11 = %d, want 0 — no run was ever started for it", n)
	}
}

func TestOrchestrate_PauseExceedingItsTimeoutEndsWithoutReviewOrRetry(t *testing.T) {
	hire := newFakeHire()
	hireServer := hire.server()
	t.Cleanup(hireServer.Close)

	dev := startDevServer(t, Config{HireBaseURL: hireServer.URL, Secret: "s", ReviewWaitTimeout: 2 * time.Second})
	dev.sendEvent(t, EventSubmit, SubmitEvent{QueueID: "55"})

	pollUntil(t, 20*time.Second, func() bool {
		return countCalls(hire.Calls(), "/me/auto-apply/55/tailor") == 1
	})

	// Let the (short) wait time out, then confirm the run neither retried tailor nor
	// ever called review.
	time.Sleep(6 * time.Second)

	calls := hire.Calls()
	if n := countCalls(calls, "/me/auto-apply/55/tailor"); n != 1 {
		t.Errorf("tailor calls = %d, want exactly 1 — a timed-out wait must not retry tailoring", n)
	}
	if n := countCalls(calls, "/me/auto-apply/55/review"); n != 0 {
		t.Errorf("review calls = %d, want 0 — a timed-out wait must never call review", n)
	}
}
