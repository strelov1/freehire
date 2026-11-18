package main

import (
	"errors"
	"testing"
)

// A cheap write the state fence keeps refusing ends in an error after exactly maxFencedAttempts
// attempts: never a silent skip, and never a forced full write from stale parameters.
func TestRetryFenced_IsBoundedAndReportsAnError(t *testing.T) {
	attempts, refused := 0, 0
	err := retryFenced(func() error { attempts++; return errCheapFenced }, func() { refused++ })
	if !errors.Is(err, errCheapFenced) {
		t.Fatalf("err = %v, want it to wrap errCheapFenced", err)
	}
	if attempts != maxFencedAttempts || refused != maxFencedAttempts {
		t.Errorf("attempts=%d refused=%d, want %d each", attempts, refused, maxFencedAttempts)
	}
}

func TestRetryFenced_StopsAtTheFirstOtherOutcome(t *testing.T) {
	boom := errors.New("boom")
	for name, outcomes := range map[string][]error{
		"success after one refusal": {errCheapFenced, nil},
		"other error is returned":   {errCheapFenced, boom},
	} {
		t.Run(name, func(t *testing.T) {
			calls, refused := 0, 0
			err := retryFenced(func() error { e := outcomes[calls]; calls++; return e }, func() { refused++ })
			if want := outcomes[len(outcomes)-1]; !errors.Is(err, want) && err != want {
				t.Errorf("err = %v, want %v", err, want)
			}
			if calls != 2 || refused != 1 {
				t.Errorf("calls=%d refused=%d, want 2 and 1", calls, refused)
			}
		})
	}
}

func TestWriteTally_ReportsFencedAttemptsBesideTheShare(t *testing.T) {
	w := newWriteTally()
	w.record("lever", true)
	w.recordFenced("lever")
	w.recordFenced("lever")
	w.recordFenced("greenhouse") // fenced only: no persisted posting yet
	if got, want := w.summary(), "greenhouse fenced=1 lever cheap=1/1 (100%) fenced=2"; got != want {
		t.Errorf("summary() = %q, want %q", got, want)
	}
}
