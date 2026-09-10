package autoapply

import (
	"context"
	"testing"
)

// A board that refuses its own captcha verification has told us, in its own words, that it
// did NOT create an application — which is what makes this the one refusal safe to retry.
// Every other post-submit uncertainty dead-letters immediately, because retrying it risks a
// second real application; this one risks nothing.
//
// It has to be retried generously, not once: a measurement over eight probe runs against a
// live Lever posting (headless and windowed, datacentre and residential IP, with and without
// mouse/scroll warm-up) passed the invisible hCaptcha exactly once. No configuration made it
// reliable, so what decides whether an application ever goes through is how many times we are
// willing to ask.
func TestRunRetriesACaptchaRefusalOnItsOwnGenerousBudget(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 7, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{answers: map[string]string{}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusCaptchaRefused}}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1 — a captcha refusal is a failed attempt, just a retryable one", stats.Failed)
	}
	if len(store.failedCaptcha) != 1 || store.failedCaptcha[0] != 7 {
		t.Fatalf("Store.FailCaptcha calls = %v, want [7]", store.failedCaptcha)
	}
	if store.failCaptchaMax != captchaMaxAttempts {
		t.Errorf("FailCaptcha called with maxAttempts=%d, want captchaMaxAttempts=%d", store.failCaptchaMax, captchaMaxAttempts)
	}
	if store.failCaptchaMax <= opts().MaxAttempts {
		t.Errorf("captchaMaxAttempts=%d is not more generous than the ordinary budget %d, which is the whole point", store.failCaptchaMax, opts().MaxAttempts)
	}
}

// The reason recorded on the row is what a person reads when the budget finally runs out, so
// it must say the captcha refused rather than repeating a raw page fragment.
func TestRunRecordsWhyACaptchaRefusalHappened(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 8, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{answers: map[string]string{}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusCaptchaRefused, Reason: "There was an error verifying your application."}}

	if _, err := Run(context.Background(), store, answers, sidecar, opts()); err != nil {
		t.Fatal(err)
	}
	msg := store.failMsg
	for _, want := range []string{"captcha", "verifying your application"} {
		if !containsFold(msg, want) {
			t.Errorf("recorded reason = %q, want it to mention %q", msg, want)
		}
	}
}

func containsFold(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexFold(haystack, needle) >= 0
}

func indexFold(haystack, needle string) int {
	lower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + 32
		}
		return b
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		ok := true
		for j := range len(needle) {
			if lower(haystack[i+j]) != lower(needle[j]) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
