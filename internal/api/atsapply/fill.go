package atsapply

import (
	"context"
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

// fillAndSubmit fills every field the plan resolved, presses submit, and reports whether
// the submission was confirmed. It runs on an already-navigated page (the same session
// renderedHTML used to scan the form) — config always wins here in the sense that matters
// for v1: this package fills strictly from the plan and never reads back or trusts anything
// the page may have pre-filled itself.
//
// This is the least-verified part of the package — see design.md's Testing section and
// task 7.1: correctness here rests on the 2026-09-02 spike's single live posting and the
// reference implementation's own measured rules, not on this package's own live testing.
func fillAndSubmit(ctx context.Context, jobURL string, plan Plan) (bool, error) {
	for _, f := range plan.Fields {
		if err := fillOne(ctx, f); err != nil {
			return false, fmt.Errorf("fill %q: %w", f.ID, err)
		}
	}

	if err := chromedp.Run(ctx, chromedp.Click(greenhouseSubmitSelector, chromedp.ByQuery)); err != nil {
		return false, fmt.Errorf("click submit: %w", err)
	}

	return verifySubmission(ctx)
}

// greenhouseSubmitSelector is Greenhouse's own submit button id.
const greenhouseSubmitSelector = "#submit_app"

func fillOne(parent context.Context, f ResolvedField) error {
	ctx, cancel := context.WithTimeout(parent, fillTimeout)
	defer cancel()

	sel := fieldSelector(f.ID)

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
			chromedp.Clear(sel, chromedp.ByID),
			chromedp.SendKeys(sel, f.Value, chromedp.ByID),
			chromedp.SendKeys(sel, kb.Enter, chromedp.ByID),
		)
	case "select":
		// SetValue sets the DOM .value property directly, which a React-controlled
		// select does not observe as a change — its own onChange handler never fires,
		// so the framework's state (and the value actually posted on submit) can stay
		// unset even though the raw DOM value looks right. Dispatching the events a
		// real interaction would fire is what makes React notice.
		return chromedp.Run(ctx,
			chromedp.SetValue(sel, f.Value, chromedp.ByID),
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
		return chromedp.Run(ctx, chromedp.SetUploadFiles(sel, []string{f.Value}, chromedp.ByID))
	default:
		return fmt.Errorf("no fill strategy for kind %q", f.Kind)
	}
}

// fieldSelector resolves a merged field's id to a DOM selector. IDs from this package's own
// scan are element ids; `#id` is correct for every kind ScanGreenhouseForm produces except
// checkbox_group, which fillOne selects by name+value directly instead.
func fieldSelector(id string) string {
	return "#" + id
}

// classifyPollError reports whether a failed poll call should be treated as unconfirmed
// (the same outcome the between-polls ctx.Done() case already reports) rather than a real
// error. A non-nil ctx.Err() means the deadline/cancellation is what actually ended the
// call, regardless of what chromedp's own error text says — the same timeout firing a
// moment earlier or later would have hit the ordinary ctx.Done() branch instead, and this
// must report identically either way. Found by code review: this check did not exist, so a
// deadline firing mid-call surfaced as a plain retryable error.
func classifyPollError(ctx context.Context, err error) (unconfirmed bool) {
	return ctx.Err() != nil
}

// verifySubmission waits for either a confirmation or an explicit refusal marker in the
// page text. Neither appearing within the timeout is reported as unconfirmed (false),
// distinct from an error — see the CONFIRMATION_MARKERS doc comment for why the two
// failure directions are not symmetric.
func verifySubmission(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, submitVerifyTimeout)
	defer cancel()

	deadline := time.Now().Add(submitVerifyTimeout)
	for time.Now().Before(deadline) {
		var bodyText string
		if err := chromedp.Run(ctx, chromedp.Text("body", &bodyText, chromedp.ByQuery)); err != nil {
			if classifyPollError(ctx, err) {
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
				return false, fmt.Errorf("board refused the submission: matched marker %q", m)
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
