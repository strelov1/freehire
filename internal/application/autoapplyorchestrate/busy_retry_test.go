package autoapplyorchestrate

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// 409 from the tailoring route means "that session is busy with another run" — hire's own
// guard against two tailoring runs on one session. It is not a failure of the work: the
// earlier run is still going and will finish.
//
// Treating it as an ordinary error is what wasted a live entry's retries on 2026-09-16. A
// tailoring run took 7m44s; Inngest's retry arrived while it was still going, got 409 in 31
// milliseconds, counted that as an attempt, and did it again — every retry spent against a
// door that was always going to be shut for another few minutes.
func TestBusyResponse_AsksToComeBackLaterInsteadOfSpendingARetry(t *testing.T) {
	err := responseError(http.StatusConflict, "/me/auto-apply/19/tailor", `{"error":"this tailoring session is busy with another run"}`)

	if err == nil {
		t.Fatal("a 409 was treated as success")
	}
	at, ok := retryAt(err)
	if !ok {
		t.Fatal("a 409 does not carry a retry time, so Inngest retries immediately into the same busy session")
	}
	wait := time.Until(at)
	if wait < time.Minute {
		t.Errorf("retry in %s — too soon: the run it collided with takes minutes, so this just spends another attempt", wait.Round(time.Second))
	}
	if wait > 30*time.Minute {
		t.Errorf("retry in %s — too far out for an application a candidate is waiting on", wait.Round(time.Minute))
	}
}

// Everything else keeps its meaning. A 500 from the tailoring run is a real failure and must
// spend a retry; a 404 is a bad route and retrying it at all is generous.
func TestOtherFailures_StayOrdinaryErrors(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError, http.StatusNotFound, http.StatusBadRequest} {
		err := responseError(code, "/me/auto-apply/19/tailor", "boom")
		if err == nil {
			t.Fatalf("status %d was treated as success", code)
		}
		if _, ok := retryAt(err); ok {
			t.Errorf("status %d asked to come back later; only a busy session may do that", code)
		}
	}
}

// The cause survives the wrapping, because it is what an operator reads in the run's output.
func TestBusyResponse_KeepsTheBoardsOwnWords(t *testing.T) {
	err := responseError(http.StatusConflict, "/p", `{"error":"this tailoring session is busy with another run"}`)

	if !errors.Is(err, err) || err.Error() == "" {
		t.Fatal("empty error")
	}
	if want := "busy with another run"; !contains(err.Error(), want) {
		t.Errorf("error = %q, want it to carry %q", err.Error(), want)
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
