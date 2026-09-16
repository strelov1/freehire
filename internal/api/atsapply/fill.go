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

// fillTimeout bounds one field's interaction. Short: a field either responds immediately or
// it was never a real target on the page.
const fillTimeout = 5 * time.Second

// submitVerifyTimeout bounds how long this waits for a confirmation or refusal marker to
// appear after the submit click.
const submitVerifyTimeout = 15 * time.Second

// dispatchChangeEventsJS fires the events a real user interaction would after
// chromedp.SetValue writes a <select>'s value directly — SetValue does not fire them
// itself, and a React-controlled select's onChange (and so its component state) never
// otherwise observes the write. %q (via fmt.Sprintf) is the element's own CSS selector.
const dispatchChangeEventsJS = `(() => {
	const el = document.querySelector(%q);
	if (!el) return;
	el.dispatchEvent(new Event('input', {bubbles: true}));
	el.dispatchEvent(new Event('change', {bubbles: true}));
})()`

// CONFIRMATION_MARKERS are positive acknowledgements only, per the reference
// implementation's own rule: matching none of these means "unconfirmed", never "failed".
// Extend, never invert — a false "confirmed" risks recording an application that never went
// through; a false "unconfirmed" only costs a retry.
var confirmationMarkers = []string{
	"thank you for applying",
	"application submitted",
	"application received",
	"we have received your application",
	"we've received your application",
}

// submitRefusedMarkers is text a board renders when it declined the submit click itself —
// an explicit refusal, safe to act on (unlike inverting confirmationMarkers would be).
var submitRefusedMarkers = []string{
	"please try again",
	"there was an error",
}

// errCaptchaRefused marks the one refusal that is safe to retry. A board that says it could
// not VERIFY the submission is telling us it did not accept one — no application exists, so
// asking again costs the employer nothing. Every other post-submit uncertainty must not be
// retried, because it might mean the opposite.
var errCaptchaRefused = errors.New("captcha refused the submission")

// captchaRefusalMarkers are the boards' own wordings for a failed captcha verification.
// Deliberately narrow: each requires the word "verify"/"verifying" beside the failure, so a
// board objecting to the résumé or a field — which it could only do having READ the
// submission — stays an ordinary refusal.
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
// errCaptchaRefused while reading as the detail alone: the sentinel is a label the client
// dispatches on, and the runner writes its own sentence in front of it when it records the
// row. Wrapping with fmt.Errorf("%w: %w", …) instead made the recorded reason say "captcha
// refused the submission" twice before saying anything a person could act on.
type captchaRefusal struct{ detail error }

func (e captchaRefusal) Error() string   { return e.detail.Error() }
func (e captchaRefusal) Unwrap() []error { return []error{errCaptchaRefused, e.detail} }

// challengeVisibleJS asks the page whether a captcha challenge is actually ON SCREEN. Both
// conditions are load-bearing and both were learned the same way, by reading a live page:
// the provider mounts its frames on EVERY posting at full size and keeps them
// visibility:hidden until it poses a puzzle, so size alone says nothing — an earlier version
// of this check called a hidden frame a challenge and reported a run that had actually
// PASSED as refused.
const challengeVisibleJS = `(() => Array.from(document.querySelectorAll("iframe")).some(f => {
	const src = f.src || "";
	if (!src.includes("hcaptcha.com") && !src.includes("recaptcha")) return false;
	if (window.getComputedStyle(f).visibility === "hidden") return false;
	return f.getBoundingClientRect().height > 100;
}))()`

// challengeVisible reports whether a captcha challenge currently covers the form. A page that
// cannot be asked (the browser is gone, the context is done) answers false: this only ever
// reclassifies a failure that already happened, and guessing "captcha" without evidence would
// hand an ordinary error the captcha's twenty asks.
func challengeVisible(ctx context.Context) bool {
	var visible bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(challengeVisibleJS, &visible)); err != nil {
		return false
	}
	return visible
}

// formStillPresentJS asks the page whether the given selector still resolves to a control
// the loop could still usably click later — present, enabled, and not hidden. A same-page
// async submit can leave the element mounted but disabled (a "Submitting…" state) or hidden
// behind a confirmation overlay without removing it from the DOM, and a bare
// querySelector(...) !== null would call that "still present" and let the loop click it a
// second time — precisely the duplicate submission this check exists to prevent.
// %q (via fmt.Sprintf) is the submit control's own CSS selector.
const formStillPresentJS = `(() => {
	const el = document.querySelector(%q);
	if (!el) return false;
	if (el.disabled) return false;
	const style = window.getComputedStyle(el);
	if (style.display === "none" || style.visibility === "hidden") return false;
	return true;
})()`

// formStillPresent reports whether the form's submit control is still on the page in a
// state the loop could still usably act on. Both this function and challengeVisible return
// false on an Evaluate error, but that shared literal value means opposite things to each
// caller: challengeVisible's false is neutral ("no evidence of a challenge"), while this
// false is the protective direction ("treat it as gone"). The likeliest reason the call
// itself fails right after a field's trailing Enter keystroke is that the keystroke just
// navigated the page and destroyed the execution context — which is exactly the condition
// this asks about, not an absence of evidence either way. Costing an unnecessary trip
// through verifySubmission (which times out to an unconfirmed, dead-lettered result) is
// preferred over silently continuing to fill a form that may already be gone.
//
// No unit test exercises this function directly, matching challengeVisible beside it — a real
// browser session cannot be faked usefully. The decision this feeds (runFillLoop) is what is
// unit tested, with this function's result taken as a plain bool parameter.
func formStillPresent(ctx context.Context, submitSelector string) bool {
	var present bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(formStillPresentJS, submitSelector), &present)); err != nil {
		return false
	}
	return present
}

// classifyFillFailure decides whether a failure to fill a field was really the captcha.
//
// Lever's own page binds the invisible captcha to the LOCATION field's focus — touching it
// calls hcaptcha.execute() — so when the score is not enough the challenge covers the form
// and the very next keystroke times out. Recorded as an ordinary transient error, three of
// those dead-lettered a live entry whose captcha budget still had twelve asks left. The
// cause is the captcha, so it belongs on the captcha's counter.
func classifyFillFailure(err error, challengeOnScreen bool) error {
	if challengeOnScreen {
		return captchaRefusal{detail: err}
	}
	return err
}

// newRefusalError builds the error a matched refusal marker travels as: the marker that
// fired, the board's own sentence around it, and — when the board declined to verify — the
// errCaptchaRefused sentinel the runner reads with errors.Is.
func newRefusalError(marker, bodyText string) error {
	err := fmt.Errorf("board refused the submission: matched marker %q in %q", marker, refusalEvidence(bodyText, marker))
	if isCaptchaRefusal(bodyText) {
		return captchaRefusal{detail: err}
	}
	return err
}

// refusalEvidenceWindow is how much of the page text either side of a refusal marker is
// carried back. Wide enough for the sentence the marker sits in — which is where a board
// says what it actually objected to — and narrow enough to stay one readable line in a
// queue row's last_error.
const refusalEvidenceWindow = 140

// refusalEvidence returns the page text around a matched refusal marker, collapsed onto one
// line. A marker names this package's own detector; the board's reason is in the sentence
// beside it, and without that sentence the only way to learn it is a second real submission.
func refusalEvidence(bodyText, marker string) string {
	i := strings.Index(strings.ToLower(bodyText), marker)
	if i < 0 {
		return ""
	}
	start := max(0, i-refusalEvidenceWindow)
	end := min(len(bodyText), i+len(marker)+refusalEvidenceWindow)
	return strings.Join(strings.Fields(bodyText[start:end]), " ")
}

// fillLoopOutcome reports how far runFillLoop got: how many fields it actually filled, and
// whether it stopped because the submit control disappeared early rather than reaching the
// end of the plan.
type fillLoopOutcome struct {
	filledCount  int
	stoppedEarly bool
}

// runFillLoop is fillAndSubmit's field loop, factored out as pure control flow so it is
// unit-testable without a real browser: fill is called once per field in order, and
// presentAfter is consulted only immediately after a text/textarea field fills — the one
// field kind whose interaction (a trailing Enter, see fillOne) can trigger the form's own
// submit binding. The first time presentAfter reports false, the loop stops: it neither
// fills any remaining field nor lets its caller reach its own submit click, since the
// submission may already be underway.
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
// the submission was confirmed. It runs on an already-navigated page (the same session
// renderedHTML used to scan the form) — config always wins here in the sense that matters
// for v1: this package fills strictly from the plan and never reads back or trusts anything
// the page may have pre-filled itself.
//
// This is the least-verified part of the package — see design.md's Testing section and
// task 7.1: correctness here rests on the 2026-09-02 spike's single live posting and the
// reference implementation's own measured rules, not on this package's own live testing.
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
		// A text/textarea field's own trailing Enter may have already triggered the real
		// submit (see fillOne's doc comment on that branch) — verify what actually
		// happened rather than filling further fields or clicking submit again on a form
		// that may already be gone. Logged unconditionally, not only when unconfirmed or
		// refused: even a CONFIRMED result here may be missing every field ordered after
		// the trigger (an approved résumé included), and that gap is otherwise invisible —
		// nothing else records how many of the plan's fields actually filled.
		log.Printf("atsapply: job %d submit control disappeared after field %d of %d — verifying rather than continuing to fill or clicking submit again", jobID, outcome.filledCount, len(kinds))
		return verifySubmission(ctx)
	}

	if err := chromedp.Run(ctx, chromedp.Click(layout.submitSelector, chromedp.ByQuery)); err != nil {
		return false, fmt.Errorf("click submit: %w", err)
	}

	return verifySubmission(ctx)
}

func fillOne(parent context.Context, f ResolvedField, by addressing) error {
	ctx, cancel := context.WithTimeout(parent, fillTimeout)
	defer cancel()

	// sel and kind are ONE decision, taken once here: a `[name=…]` selector handed to a
	// by-id lookup is silently rewritten to `#[name=…]`, which is invalid CSS and matches
	// nothing. Every branch below MUST pass `kind` — a review caught the file branch
	// hard-coding chromedp.ByID, which would have meant no Lever application carrying a
	// résumé could ever be submitted, on a page where the résumé is required. There is a
	// test asserting no branch reintroduces a literal query kind.
	sel := fieldSelector(f.ID, by)
	kind := by.queryKind()

	switch f.Kind {
	case "textarea", "text":
		// Text and the react-select-backed autocomplete fields (country,
		// candidate-location) are indistinguishable in this package's DOM scan — both
		// render as a plain <input type="text">. The trailing Enter is meant as a no-op
		// on a plain text field and, for an autocomplete field, commits the highlighted
		// suggestion — the same "type, then confirm" interaction a person uses.
		// Unverified beyond the reference implementation's own account of the pattern,
		// and a code review flagged a specific, plausible way that assumption could be
		// wrong: a React-driven SPA form can bind its own Enter-submits-the-form
		// behavior regardless of field count (unlike plain HTML's multi-field implicit-
		// submission exemption), which would trigger a real submit mid-fill-loop rather
		// than a no-op. There is no reliable signal in this package's current scan data
		// to tell a true autocomplete field apart from a plain one (the field's Options
		// are empty for both, since Greenhouse never declares country/location as an
		// enumerated field) — a targeted fix needs live verification against a real
		// board, not a guess. The trigger itself is therefore still a known, accepted
		// risk rather than worked around blind — but its blast radius is bounded:
		// runFillLoop checks the submit control's presence right after this field, and
		// an early submit stops the loop before it fills anything further or clicks
		// submit a second time (see formStillPresent). A typed value with no matching
		// suggestion, or an unintended early submit, can still surface as an
		// unconfirmed or malformed outcome, which is exactly why StatusUnconfirmed
		// exists as a distinct, non-retried outcome rather than trusting any of this
		// always worked.
		return chromedp.Run(ctx,
			chromedp.Clear(sel, kind),
			chromedp.SendKeys(sel, f.Value, kind),
			chromedp.SendKeys(sel, kb.Enter, kind),
		)
	case "select":
		// SetValue sets the DOM .value property directly, which a React-controlled
		// select does not observe as a change — its own onChange handler never fires,
		// so the framework's state (and the value actually posted on submit) can stay
		// unset even though the raw DOM value looks right. Dispatching the events a
		// real interaction would fire is what makes React notice.
		return chromedp.Run(ctx,
			chromedp.SetValue(sel, f.Value, kind),
			chromedp.Evaluate(fmt.Sprintf(dispatchChangeEventsJS, sel), nil),
		)
	case "checkbox_group":
		// f.Value is one option's value — resolveOne (resolve.go) never resolves more
		// than one for a Multi field, since AnswerSource never supplies more than one
		// candidate value per question today. See resolveOne's doc comment.
		optSel := fmt.Sprintf(`input[name=%q][value=%q]`, f.ID, f.Value)
		return chromedp.Run(ctx, chromedp.Click(optSel, chromedp.ByQuery))
	case "file":
		// The only file field resolveOne ever resolves is the résumé/CV upload
		// (isResumeField, resolve.go), and only once client.go's attachApprovedResume has
		// rendered the approved tailored CV and overwritten Value with the temp PDF's
		// path — never a candidate-authored string, so no further validation of Value
		// belongs here.
		return chromedp.Run(ctx, chromedp.SetUploadFiles(sel, []string{f.Value}, kind))
	default:
		return fmt.Errorf("no fill strategy for kind %q", f.Kind)
	}
}

// fieldSelector resolves a field's identifier to a DOM selector, the way its own platform
// names it. The identifier is whatever the layout addressed the control by (see
// domscan.identify) — an element id on Greenhouse, a name on Lever.
//
// A byName selector quotes the value because Lever's names carry brackets:
// `urls[LinkedIn]`, and every employer question as `cards[<uuid>][field0]`. Unquoted,
// brackets are selector syntax and the lookup silently matches nothing.
//
// checkbox_group is the exception under either addressing: fillOne selects those by
// name+value directly, since a group's members share a name rather than an identifier.
func fieldSelector(id string, by addressing) string {
	if by == byName {
		return fmt.Sprintf("[name=%q]", id)
	}
	return "#" + id
}

// classifyPollError reports whether a failed poll call should be treated as unconfirmed
// (the same outcome the between-polls ctx.Done() case already reports) rather than a real
// error. A non-nil ctx.Err() means the deadline/cancellation is what actually ended the
// call, regardless of what chromedp's own error text says — the same timeout firing a
// moment earlier or later would have hit the ordinary ctx.Done() branch instead, and this
// must report identically either way. Found by code review: this check did not exist, so a
// deadline firing mid-call surfaced as a plain retryable error.
func pollEndedByDeadline(ctx context.Context) bool {
	return ctx.Err() != nil
}

// verifySubmission waits for either a confirmation or an explicit refusal marker in the
// page text. Neither appearing within the timeout is reported as unconfirmed (false),
// distinct from an error — see the CONFIRMATION_MARKERS doc comment for why the two
// failure directions are not symmetric.
func verifySubmission(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, submitVerifyTimeout)
	defer cancel()

	// One deadline, the context's. The loop used to carry a second time.Now()-based one
	// computed from the same timeout, which could never be the binding limit — ctx is
	// derived from it above and cancels first or together.
	for ctx.Err() == nil {
		var bodyText string
		if err := chromedp.Run(ctx, chromedp.Text("body", &bodyText, chromedp.ByQuery)); err != nil {
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
