package atsapply

import "testing"

// A trimmed stand-in for the real white-label custom-domain page a live verification run
// found (careers.godaddy) — a form with none of the vanilla template's ids, gated by a
// reCAPTCHA Enterprise widget.
const whitelabelFormWithRecaptchaHTML = `
<html><body>
<form id="form_2_3">
  <input id="form_first_name_2_3_0" name="first_name" type="text" required>
</form>
<iframe title="reCAPTCHA" src="https://www.recaptcha.net/recaptcha/enterprise/anchor?k=abc"></iframe>
</body></html>
`

// A page whose form layout simply does not match the known selector, with no CAPTCHA
// footprint at all — the "we genuinely don't recognize this" fallback case.
const whitelabelFormWithoutRecaptchaHTML = `
<html><body>
<form id="form_2_3">
  <input id="form_first_name_2_3_0" name="first_name" type="text" required>
</form>
</body></html>
`

func TestClassifyUnscannableForm_RecaptchaFootprintWins(t *testing.T) {
	if got := classifyUnscannableForm(whitelabelFormWithRecaptchaHTML); got != reasonCaptchaProtected {
		t.Errorf("classifyUnscannableForm = %q, want %q", got, reasonCaptchaProtected)
	}
}

func TestClassifyUnscannableForm_FallsBackToUnrecognizedLayout(t *testing.T) {
	if got := classifyUnscannableForm(whitelabelFormWithoutRecaptchaHTML); got != reasonUnrecognizedLayout {
		t.Errorf("classifyUnscannableForm = %q, want %q", got, reasonUnrecognizedLayout)
	}
}

func TestClassifyUnscannableForm_VanillaFormFixtureIsNotMisclassifiedAsCaptcha(t *testing.T) {
	// Regression guard: the package's own vanilla-template fixture (domscan_test.go) must
	// never trip the reCAPTCHA marker — this classifier only ever runs on a page whose
	// known selector was NOT found, but a false-positive marker here would still be a bug
	// worth catching directly.
	if got := classifyUnscannableForm(greenhouseFixtureHTML); got != reasonUnrecognizedLayout {
		t.Errorf("classifyUnscannableForm(vanilla fixture) = %q, want %q (no recaptcha marker present)", got, reasonUnrecognizedLayout)
	}
}

func TestHasRecaptchaMarker_CaseInsensitive(t *testing.T) {
	if !hasRecaptchaMarker(`<script src="https://WWW.RECAPTCHA.NET/enterprise.js"></script>`) {
		t.Error("hasRecaptchaMarker = false, want true for a differently-cased URL")
	}
	if hasRecaptchaMarker(`<form id="application-form"></form>`) {
		t.Error("hasRecaptchaMarker = true, want false for an ordinary form")
	}
}
