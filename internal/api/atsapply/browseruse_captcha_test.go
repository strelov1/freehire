package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/application/autoapply"
)

// The cloud browser solves supported captchas, but not always: a live Lever run reported
// "hCaptcha verification failed and the application was not submitted." Parking that waits
// for data that will never arrive — nothing about the candidate changed, and the very next
// ask might pass. It is the same refusal the Chrome path already retries on its own generous
// budget, and it arrives with the same guarantee: the agent says no application was created.
func TestCloudOutcome_ACaptchaFailureIsRetried(t *testing.T) {
	result := resultForParkedReport("hCaptcha verification failed and the application was not submitted.")

	if result.Status != autoapply.StatusCaptchaRefused {
		t.Errorf("status = %q, want %q so it retries instead of waiting for data nobody can supply",
			result.Status, autoapply.StatusCaptchaRefused)
	}
	if result.Reason == "" {
		t.Error("the agent's own sentence was dropped; it is what a person reads when the asks run out")
	}
}

// Everything else the agent parks on really is missing data — a question nobody answered, a
// field it was not given — and retrying that produces the identical park twenty times.
func TestCloudOutcome_AnOrdinaryParkStillParks(t *testing.T) {
	result := resultForParkedReport("The form asks for a portfolio URL and I was not given one.")

	if result.Status != autoapply.StatusParked {
		t.Errorf("status = %q, want %q", result.Status, autoapply.StatusParked)
	}
}
