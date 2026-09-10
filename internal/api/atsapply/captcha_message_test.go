package atsapply

import (
	"errors"
	"strings"
	"testing"
)

// The refusal is recorded on the queue row and read by a person. Wrapping the sentinel with
// fmt.Errorf("%w: %w", …) put its own sentence in front of the detail, and the runner then
// prefixed the whole thing again — so the row said "captcha refused the submission" three
// times before saying anything useful.
func TestCaptchaRefusalError_SaysItOnce(t *testing.T) {
	err := newRefusalError("please try again", "There was an error verifying your application. Please try again.")

	if n := strings.Count(err.Error(), "captcha refused"); n != 0 {
		t.Errorf("error text = %q, want the sentinel's own words %d times, not %d — the runner adds them", err.Error(), 0, n)
	}
	if !strings.Contains(err.Error(), "matched marker") {
		t.Errorf("error text = %q, want it to carry the detail", err.Error())
	}
	if !errors.Is(err, errCaptchaRefused) {
		t.Errorf("error no longer answers errors.Is for the sentinel, which is what the client dispatches on")
	}
}
