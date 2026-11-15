package atsapply

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// fillTimeout bounds one field's interaction; a field responds immediately or was never a real target.
const fillTimeout = 5 * time.Second

// submitVerifyTimeout bounds the wait for a confirmation or refusal marker after the submit click.
const submitVerifyTimeout = 15 * time.Second

// dispatchChangeEventsJS fires the input/change events chromedp.SetValue does not, so a
// React-controlled select's onChange sees the write. %q is the element's CSS selector.
const dispatchChangeEventsJS = `(() => {
	const el = document.querySelector(%q);
	if (!el) return;
	el.dispatchEvent(new Event('input', {bubbles: true}));
	el.dispatchEvent(new Event('change', {bubbles: true}));
})()`

// confirmationMarkers are positive acknowledgements only: matching none means
// "unconfirmed", never "failed". Extend, never invert: a false "confirmed" records an
// application that never went through; a false "unconfirmed" only costs a retry.
var confirmationMarkers = []string{
	"thank you for applying",
	"application submitted",
	"application received",
	"we have received your application",
	"we've received your application",
}

// submitRefusedMarkers is text a board renders when it declined the submit click itself,
// an explicit refusal safe to act on.
var submitRefusedMarkers = []string{
	"please try again",
	"there was an error",
}

// errCaptchaRefused marks the one refusal that is safe to retry: a board that could not
// verify the submission did not accept one, so no application exists. Every other
// post-submit uncertainty must not be retried.
var errCaptchaRefused = errors.New("captcha refused the submission")

// captchaRefusalMarkers are the boards' wordings for a failed captcha verification. Narrow
// on purpose: each needs "verify"/"verifying" beside the failure, so a board objecting to
// the résumé or a field (which it could only do after reading the submission) stays an
// ordinary refusal.
var captchaRefusalMarkers = []string{
	"error verifying your",
	"could not verify",
	"couldn't verify",
	"unable to verify",
	"verification failed",
}

// isCaptchaRefusal reports whether the page text is a board declining to verify rather than
// declining the content of the application.
func isCaptchaRefusal(bodyText string) bool {
	return containsAny(strings.ToLower(bodyText), captchaRefusalMarkers)
}

// captchaRefusal carries a refusal that is safe to retry. It answers errors.Is for
// errCaptchaRefused but reads as the detail alone, since the runner writes its own sentence
// in front; wrapping with %w: %w repeated "captcha refused the submission" in the reason.
type captchaRefusal struct{ detail error }

func (e captchaRefusal) Error() string   { return e.detail.Error() }
func (e captchaRefusal) Unwrap() []error { return []error{errCaptchaRefused, e.detail} }

// challengeVisibleJS asks whether a captcha challenge is actually on screen. Both checks
// are load-bearing: providers mount their frames on every posting at full size and keep
// them visibility:hidden until they pose a puzzle, so size alone calls a passed run
// refused.
const challengeVisibleJS = `(() => Array.from(document.querySelectorAll("iframe")).some(f => {
	const src = f.src || "";
	if (!src.includes("hcaptcha.com") && !src.includes("recaptcha")) return false;
	if (window.getComputedStyle(f).visibility === "hidden") return false;
	return f.getBoundingClientRect().height > 100;
}))()`

// challengeVisible reports whether a captcha challenge currently covers the form. A page
// that cannot be asked answers false: this only reclassifies a failure that already
// happened, and guessing "captcha" without evidence would charge an ordinary error to the
// captcha's retry budget.
func challengeVisible(ctx context.Context) bool {
	visible, err := chromedp.Run(ctx, chromedp.Evaluate[bool](challengeVisibleJS))
	if err != nil {
		return false
	}
	return visible
}

// formStillPresentJS asks whether the selector still resolves to a control that can be
// clicked: present, enabled and not hidden. An async submit can leave the element mounted
// but disabled or hidden, and a bare querySelector !== null would let the loop click it
// again, a duplicate submission. %q is the submit control's CSS selector.
const formStillPresentJS = `(() => {
	const el = document.querySelector(%q);
	if (!el) return false;
	if (el.disabled) return false;
	const style = window.getComputedStyle(el);
	if (style.display === "none" || style.visibility === "hidden") return false;
	return true;
})()`

// formStillPresent reports whether the form's submit control is still usable on the page.
// An Evaluate error returns false, the protective direction ("treat it as gone"), unlike
// challengeVisible's neutral false: the likeliest cause is that a field's trailing Enter
// navigated the page and destroyed the execution context, which is exactly what this asks
// about. A needless trip through verifySubmission beats filling a form that may be gone.
//
// Not unit tested (a browser cannot be faked usefully); runFillLoop, which consumes it as a
// bool, is.
func formStillPresent(ctx context.Context, submitSelector string) bool {
	present, err := chromedp.Run(ctx, chromedp.Evaluate[bool](fmt.Sprintf(formStillPresentJS, submitSelector)))
	if err != nil {
		return false
	}
	return present
}

// classifyFillFailure decides whether a failed field fill was really the captcha. Lever
// binds the invisible captcha to the LOCATION field's focus, so when the score is too low
// the challenge covers the form and the next keystroke times out. That belongs on the
// captcha's counter, not as an ordinary transient error.
func classifyFillFailure(err error, challengeOnScreen bool) error {
	if challengeOnScreen {
		return captchaRefusal{detail: err}
	}
	return err
}

// newRefusalError builds the error for a matched refusal marker: the marker, the board's
// sentence around it, and errCaptchaRefused when the board declined to verify.
func newRefusalError(marker, bodyText string) error {
	err := fmt.Errorf("board refused the submission: matched marker %q in %q", marker, refusalEvidence(bodyText, marker))
	if isCaptchaRefusal(bodyText) {
		return captchaRefusal{detail: err}
	}
	return err
}

// refusalEvidenceWindow is how much page text either side of a refusal marker is carried
// back: enough for the sentence it sits in, narrow enough for one line in last_error.
const refusalEvidenceWindow = 140

// refusalEvidence returns the page text around a matched refusal marker, collapsed to one
// line. The board's reason is in the sentence beside the marker.
func refusalEvidence(bodyText, marker string) string {
	i := strings.Index(strings.ToLower(bodyText), marker)
	if i < 0 {
		return ""
	}
	start := max(0, i-refusalEvidenceWindow)
	end := min(len(bodyText), i+len(marker)+refusalEvidenceWindow)
	return strings.Join(strings.Fields(bodyText[start:end]), " ")
}

// fillLoopOutcome reports how many fields runFillLoop filled and whether it stopped
// because the submit control disappeared early.
type fillLoopOutcome struct {
	filledCount  int
	stoppedEarly bool
}

// runFillLoop is fillAndSubmit's field loop as pure control flow, testable without a
// browser. fill runs per field in order; presentAfter is consulted only after a
// text/textarea field (whose trailing Enter, see fillOne, can trigger the form's own
// submit). The first false stops the loop: no further fields and no submit click, since
// the submission may be underway.
func runFillLoop(kinds []string, fill func(i int) error, presentAfter func(i int) bool) (fillLoopOutcome, error) {
	for i, kind := range kinds {
		if err := fill(i); err != nil {
			return fillLoopOutcome{filledCount: i}, err
		}
		if (kind == "text" || kind == "textarea") && !presentAfter(i) {
			return fillLoopOutcome{filledCount: i + 1, stoppedEarly: true}, nil
		}
	}
	return fillLoopOutcome{filledCount: len(kinds)}, nil
}

// fillAndSubmit fills every field the plan resolved, presses submit, and reports whether
// the submission was confirmed. It runs on the already-navigated page that renderedHTML
// scanned, and fills strictly from the plan, never trusting what the page pre-filled.
//
// The least-verified part of the package: it rests on one live posting and the reference
// implementation's measured rules.
func fillAndSubmit(ctx context.Context, jobID int64, plan Plan, layout formLayout) (bool, error) {
	kinds := make([]string, len(plan.Fields))
	for i, f := range plan.Fields {
		kinds[i] = f.Kind
	}

	outcome, err := runFillLoop(kinds, func(i int) error {
		f := plan.Fields[i]
		if err := fillOne(ctx, f, layout.addressBy); err != nil {
			return classifyFillFailure(fmt.Errorf("fill %q: %w", f.ID, err), challengeVisible(ctx))
		}
		return nil
	}, func(i int) bool {
		return formStillPresent(ctx, layout.submitSelector)
	})
	if err != nil {
		return false, err
	}
	if outcome.stoppedEarly {
		// A field's trailing Enter may already have submitted (see fillOne): verify instead of
		// filling further or clicking submit again. Logged unconditionally, since even a confirmed
		// result may be missing every field after the trigger (the résumé included) and nothing
		// else records how many fields filled.
		log.Printf("atsapply: job %d submit control disappeared after field %d of %d — verifying rather than continuing to fill or clicking submit again", jobID, outcome.filledCount, len(kinds))
		return verifySubmission(ctx)
	}

	if err := chromedp.Do(ctx, chromedp.Click(chromedp.CSS(layout.submitSelector))); err != nil {
		return false, fmt.Errorf("click submit: %w", err)
	}

	return verifySubmission(ctx)
}

func fillOne(parent context.Context, f ResolvedField, by addressing) error {
	ctx, cancel := context.WithTimeout(parent, fillTimeout)
	defer cancel()

	// sel carries its lookup in its type (chromedp.CSS), so no branch can pair it with a
	// different one; a by-id lookup on a `[name=…]` selector becomes invalid CSS and matches
	// nothing.
	sel := fieldSelector(f.ID, by)

	switch f.Kind {
	case "textarea", "text":
		// Text and react-select autocomplete fields (country, candidate-location) are
		// indistinguishable in the DOM scan: both are a plain <input type="text">. The trailing
		// Enter is a no-op on a plain field and commits the highlighted suggestion on an
		// autocomplete. Known, accepted risk: a React SPA form may bind Enter to submit and
		// trigger a real submit mid-fill. The scan has no signal to tell the two apart (Options
		// are empty for both), so a fix needs live verification. The blast radius is bounded:
		// runFillLoop checks the submit control after this field (see formStillPresent) and stops
		// before filling more or clicking submit again. A stray early submit can still end
		// unconfirmed, which is why StatusUnconfirmed is a distinct, non-retried outcome.
		return chromedp.Do(ctx,
			chromedp.Clear(sel),
			chromedp.SendKeys(sel, f.Value),
			chromedp.SendKeys(sel, kb.Enter),
		)
	case "select":
		// SetValue writes the DOM .value directly, which a React-controlled select does not see:
		// onChange never fires and the posted value can stay unset. Dispatching the events a real
		// interaction fires makes React notice.
		return chromedp.Do(ctx,
			chromedp.SetValue(sel, f.Value),
			chromedp.Evaluate[chromedp.Void](fmt.Sprintf(dispatchChangeEventsJS, sel)),
		)
	case "checkbox_group":
		// f.Value is one option's value: resolveOne (resolve.go) never resolves more than one
		// for a Multi field.
		optSel := fmt.Sprintf(`input[name=%q][value=%q]`, f.ID, f.Value)
		return chromedp.Do(ctx, chromedp.Click(chromedp.CSS(optSel)))
	case "file":
		// The only file field resolveOne resolves is the résumé upload (isResumeField), and
		// attachApprovedResume (client.go) overwrites Value with the temp PDF path, never a
		// candidate-authored string, so Value needs no validation here.
		return chromedp.Do(ctx, chromedp.SetUploadFiles(sel, []string{f.Value}))
	default:
		return fmt.Errorf("no fill strategy for kind %q", f.Kind)
	}
}

// fieldSelector resolves a field's identifier to a CSS selector the way its platform names
// it: an element id on Greenhouse, a name on Lever (see domscan.identify).
//
// A byName selector quotes the value because Lever's names carry brackets (`urls[LinkedIn]`,
// `cards[<uuid>][field0]`); unquoted they are selector syntax and match nothing.
//
// checkbox_group is the exception under either addressing: fillOne selects its members by
// name+value, since they share a name.
func fieldSelector(id string, by addressing) chromedp.CSS {
	if by == byName {
		return chromedp.CSS(fmt.Sprintf("[name=%q]", id))
	}
	return chromedp.CSS("#" + id)
}

// pollEndedByDeadline reports whether ctx's deadline or cancellation ended a failed poll
// call, so it is treated as unconfirmed (like the between-polls ctx.Done() case) rather
// than a retryable error, whatever chromedp's error text says.
func pollEndedByDeadline(ctx context.Context) bool {
	return ctx.Err() != nil
}

// verifySubmission waits for a confirmation or an explicit refusal marker in the page
// text. Neither within the timeout is unconfirmed (false), not an error; see
// confirmationMarkers for why the failure directions are not symmetric.
func verifySubmission(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, submitVerifyTimeout)
	defer cancel()

	// One deadline: the context's.
	for ctx.Err() == nil {
		bodyText, err := chromedp.Run(ctx, chromedp.Text(chromedp.CSS("body")))
		if err != nil {
			if pollEndedByDeadline(ctx) {
				return false, nil
			}
			return false, err
		}
		lower := strings.ToLower(bodyText)
		for _, m := range confirmationMarkers {
			if strings.Contains(lower, m) {
				return true, nil
			}
		}
		for _, m := range submitRefusedMarkers {
			if strings.Contains(lower, m) {
				return false, newRefusalError(m, bodyText)
			}
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return false, nil
		}
	}
	return false, nil
}
