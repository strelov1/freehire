package atsapply

import (
	"context"
	"errors"
	"fmt"
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

// fillAndSubmit fills every field the plan resolved, presses submit, and reports whether
// the submission was confirmed. It runs on an already-navigated page (the same session
// renderedHTML used to scan the form) — config always wins here in the sense that matters
// for v1: this package fills strictly from the plan and never reads back or trusts anything
// the page may have pre-filled itself.
//
// This is the least-verified part of the package — see design.md's Testing section and
// task 7.1: correctness here rests on the 2026-09-02 spike's single live posting and the
// reference implementation's own measured rules, not on this package's own live testing.
func fillAndSubmit(ctx context.Context, plan Plan, layout formLayout) (bool, error) {
	for _, f := range plan.Fields {
		if err := fillOne(ctx, f, layout.addressBy); err != nil {
			return false, fmt.Errorf("fill %q: %w", f.ID, err)
		}
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
		// board, not a guess. Named here as a known, accepted risk rather than
		// worked around blind. A typed value with no matching suggestion, or an
		// unintended early submit, can still surface as an unconfirmed or malformed
		// outcome, which is exactly why StatusUnconfirmed exists as a distinct,
		// non-retried outcome rather than trusting any of this always worked.
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
