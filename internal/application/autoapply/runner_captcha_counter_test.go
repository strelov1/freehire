package autoapply

import (
	"context"
	"testing"
)

// A captcha refusal counts on its OWN budget, through its own store call. Sharing the
// ordinary one poisoned it: a live entry collected 12 captcha refusals and was then
// dead-lettered by the first field fill that timed out, without the three tries that error
// was entitled to. The same trap migration 0140 fixed for the preview pass.
func TestRunSpendsACaptchaRefusalOnItsOwnCounter(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 9, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{answers: map[string]string{}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusCaptchaRefused}}

	if _, err := Run(context.Background(), store, answers, sidecar, opts()); err != nil {
		t.Fatal(err)
	}
	if len(store.failedCaptcha) != 1 || store.failedCaptcha[0] != 9 {
		t.Errorf("Store.FailCaptcha calls = %v, want [9]", store.failedCaptcha)
	}
	if len(store.failed) != 0 {
		t.Errorf("Store.Fail calls = %v, want none — a captcha refusal must not touch the ordinary attempts budget", store.failed)
	}
	if store.failCaptchaMax != captchaMaxAttempts {
		t.Errorf("FailCaptcha called with maxAttempts=%d, want %d", store.failCaptchaMax, captchaMaxAttempts)
	}
}
