package atsapply

import (
	"errors"
	"fmt"
	"testing"
)

// Lever's own page binds the invisible captcha to the LOCATION field's focus: touching it
// calls hcaptcha.execute(), and when the score is not enough the challenge covers the form.
// Typing into a covered field then times out — a failure caused by the captcha, recorded as
// an ordinary transient error. Three of those spent the strict budget and dead-lettered a
// live entry whose captcha budget still had 12 asks left.
func TestFillFailure_UnderAVisibleChallengeIsACaptchaRefusal(t *testing.T) {
	cause := fmt.Errorf("fill %q: %w", "location", errors.New("context deadline exceeded"))

	err := classifyFillFailure(cause, true)

	if !errors.Is(err, errCaptchaRefused) {
		t.Fatalf("err = %v, want it to wrap errCaptchaRefused", err)
	}
	if !contains(err.Error(), "location") {
		t.Errorf("err = %v, want it to still name the field that failed", err)
	}
}

// With no challenge on screen the same timeout is what it looks like: a slow page, which is
// exactly what the ordinary three-attempt budget is for.
func TestFillFailure_WithoutAChallengeStaysOrdinary(t *testing.T) {
	cause := fmt.Errorf("fill %q: %w", "phone", errors.New("context deadline exceeded"))

	if err := classifyFillFailure(cause, false); errors.Is(err, errCaptchaRefused) {
		t.Errorf("err = %v, want an ordinary failure", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
