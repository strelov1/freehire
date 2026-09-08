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

// What EVERY vanilla job-boards.greenhouse.io posting actually renders, reduced from the
// live DOM of three postings (garnerhealth 6181651004, exadelinc 6141005004, nimblegravity
// 4721959005) as a headless Chrome on the production host saw them on 2026-09-08. All
// three were byte-for-byte alike in every part reproduced here.
//
// This is score-based, INVISIBLE reCAPTCHA Enterprise, and every piece below is why a
// cheaper test than this one is worthless:
//
//   - grecaptcha-badge — the "protected by reCAPTCHA" corner badge, which is what an
//     invisible reCAPTCHA renders INSTEAD of asking anything.
//   - an enterprise/ANCHOR iframe — present even though nothing is being asked. An
//     invisible reCAPTCHA still mounts one, so "a widget exists" cannot mean "a challenge
//     is being posed"; only a bframe (the image-grid iframe) means that, and none of the
//     three postings had one.
//   - g-recaptcha-RESPONSE, the hidden token field. Its name CONTAINS "g-recaptcha", so a
//     substring test for the widget class matches every page that merely loads reCAPTCHA.
//
// None of it gates reading or filling the form: a live probe found #application-form
// visible on these very pages in 0.8-1.7s from the production host. Treating it as a
// challenge parked every Greenhouse attempt in production — 5 of the 10 entries the queue
// had ever held — while the package's own hand-written fixtures, carrying none of this,
// stayed green.
const greenhouseInvisibleRecaptchaHTML = `
<html><body>
<form id="application-form">
  <input id="first_name" name="first_name" type="text" required>
</form>
<textarea id="g-recaptcha-response-100000" name="g-recaptcha-response" class="g-recaptcha-response"></textarea>
<div class="grecaptcha-error"></div>
<div class="grecaptcha-badge" data-style="bottomright" style="position: fixed; right: -186px;">
  <div class="grecaptcha-logo">
    <iframe title="reCAPTCHA" role="presentation" src="https://www.recaptcha.net/recaptcha/enterprise/anchor?ar=1&amp;k=6LfmcbcpAAAAAChNTbhUShzUOAMj_wY9LQIvLFX0&amp;size=invisible"></iframe>
  </div>
</div>
<script src="https://www.recaptcha.net/recaptcha/enterprise.js?render=6LfmcbcpAAAAAChNTbhUShzUOAMj_wY9LQIvLFX0"></script>
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
	if !hasRecaptchaMarker(`<iframe title="reCAPTCHA" src="https://WWW.RECAPTCHA.NET/RECAPTCHA/ENTERPRISE/ANCHOR?k=abc"></iframe>`) {
		t.Error("hasRecaptchaMarker = false, want true for a differently-cased challenge iframe")
	}
	if hasRecaptchaMarker(`<form id="application-form"></form>`) {
		t.Error("hasRecaptchaMarker = true, want false for an ordinary form")
	}
}

// The bug this whole distinction exists to prevent: every vanilla Greenhouse posting loads
// an invisible, score-based reCAPTCHA, so a marker that fires on the mere PRESENCE of the
// word parks the one provider this package can actually fill.
func TestHasRecaptchaMarker_InvisibleEnterpriseIsNotAChallenge(t *testing.T) {
	if hasRecaptchaMarker(greenhouseInvisibleRecaptchaHTML) {
		t.Error("hasRecaptchaMarker = true for a live vanilla Greenhouse page, want false — " +
			"an invisible score-based reCAPTCHA renders no widget and gates nothing")
	}
}

// The other half of the same distinction, stated on its own so a marker narrowed into
// uselessness fails here rather than silently letting a real challenge through.
func TestHasRecaptchaMarker_RenderedChallengeWidgetIsAChallenge(t *testing.T) {
	if !hasRecaptchaMarker(whitelabelFormWithRecaptchaHTML) {
		t.Error("hasRecaptchaMarker = false for a rendered reCAPTCHA anchor iframe, want true")
	}
	if !hasRecaptchaMarker(`<div class="g-recaptcha" data-sitekey="abc"></div>`) {
		t.Error("hasRecaptchaMarker = false for an explicit g-recaptcha widget, want true")
	}
}
