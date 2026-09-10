package atsapply

import (
	"errors"
	"testing"
)

// The one refusal a board states so plainly that retrying it is safe: it says it could not
// verify the submission, which means it did not accept one. Lever's own wording, read off a
// live posting on 2026-09-10.
func TestCaptchaRefusal_RecognisesTheBoardSayingItCouldNotVerify(t *testing.T) {
	cases := []string{
		"There was an error verifying your application. Please try again.",
		"there was an error verifying your submission. please try again",
		"We could not verify that you are human. Please try again.",
	}
	for _, body := range cases {
		if !isCaptchaRefusal(body) {
			t.Errorf("isCaptchaRefusal(%q) = false, want true", body)
		}
	}
}

// Everything else stays an ordinary refusal. A board that objected to the RESUME, or to a
// field, has read the submission — treating that as a captcha refusal would retry it twenty
// times against a board that will answer identically twenty times.
func TestCaptchaRefusal_LeavesOtherRefusalsAlone(t *testing.T) {
	cases := []string{
		"There was an error uploading your resume. Please try again.",
		"There was an error. Please try again later.",
		"Thank you for applying",
		"",
	}
	for _, body := range cases {
		if isCaptchaRefusal(body) {
			t.Errorf("isCaptchaRefusal(%q) = true, want false", body)
		}
	}
}

// The refusal travels as an error out of fillAndSubmit, so the client has to recognise it
// through errors.Is rather than by matching text a second time.
func TestCaptchaRefusal_TravelsAsAWrappedSentinel(t *testing.T) {
	err := newRefusalError("please try again", "There was an error verifying your application. Please try again.")

	if !errors.Is(err, errCaptchaRefused) {
		t.Fatalf("err = %v, want it to wrap errCaptchaRefused", err)
	}
	plain := newRefusalError("please try again", "There was an error uploading your resume.")
	if errors.Is(plain, errCaptchaRefused) {
		t.Errorf("a resume refusal wraps errCaptchaRefused, want it to stay an ordinary failure")
	}
}
