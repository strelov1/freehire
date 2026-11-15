package atsapply

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/strelov1/freehire/internal/platform/browser"
)

// stealthAllocatorOptions are this package's launch options. The flags live in
// platform/browser so ingest and this package cannot drift apart on anti-bot fixes. They are
// a measured pair, not a claim of undetectability.
func stealthAllocatorOptions() []chromedp.ExecAllocatorOption {
	return browser.LaunchOptions()
}

// pageLoadTimeout bounds one navigation+render before falling back to classifyTimeout.
// Generous: an application form pulls in its own JS bundle and, on Greenhouse, a client-side
// render pass.
const pageLoadTimeout = 20 * time.Second

// classifyTimeout bounds the one follow-up HTML capture renderedHTML makes to classify why
// the known selector never appeared. It is ADDITIONAL to pageLoadTimeout, not carved out of
// it: a shortened selector wait misclassified fillable postings as unrecognized_form_layout
// under load.
const classifyTimeout = 10 * time.Second

// newBrowser starts one browser process for one attempt. Callers must call the returned
// cancel. Nothing is pooled across attempts, so no state (cookies, a half-filled form) leaks
// into the next.
func (c *Client) newBrowser(ctx context.Context) (context.Context, context.CancelFunc, error) {
	return newBrowserSession(ctx, c.allocatorOpts)
}

// newBrowserSession is newBrowser's body, free of *Client so PreviewClient can start a session
// the same way.
func newBrowserSession(ctx context.Context, allocatorOpts []chromedp.ExecAllocatorOption) (context.Context, context.CancelFunc, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocatorOpts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	cancel := func() {
		cancelBrowser()
		cancelAlloc()
	}
	// Start the browser now, not lazily, so a launch failure surfaces here with a specific
	// error.
	if err := chromedp.Do(browserCtx); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("start browser: %w", err)
	}
	return browserCtx, cancel, nil
}

// renderedHTML navigates to url, waits for readySelector (the application form, proof the
// client-side render pass that reveals fields like Greenhouse's `country` has run), and
// returns the rendered HTML.
//
// If the probe's own pageLoadTimeout elapses, it spends up to classifyTimeout MORE capturing
// the current HTML and classifying it (classifyUnscannableForm), returning an
// *unscannableFormError so Client.Submit can park the attempt. Any OTHER failure (DNS,
// connection refused, crashed tab) propagates as an ordinary error: parking it would hide an
// attempt a retry might have completed.
func renderedHTML(ctx context.Context, url, readySelector string) (string, error) {
	probeCtx, probeCancel := context.WithTimeout(ctx, pageLoadTimeout)
	var pageHTML string
	err := chromedp.Do(probeCtx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(chromedp.ID(readySelector)),
	)
	if err == nil {
		pageHTML, err = chromedp.Run(probeCtx, chromedp.OuterHTML("html"))
	}
	// Checked before cancel: a context's Err() is not overwritten once Done closes, so this
	// tells the probe's own deadline apart from any other failure.
	probeTimedOut := errors.Is(probeCtx.Err(), context.DeadlineExceeded)
	probeCancel()
	if err == nil {
		return pageHTML, nil
	}
	if !probeTimedOut {
		return "", err
	}

	classifyCtx, classifyCancel := context.WithTimeout(ctx, classifyTimeout)
	defer classifyCancel()
	currentHTML, captureErr := chromedp.Run(classifyCtx, chromedp.OuterHTML("html"))
	if captureErr != nil {
		// Couldn't even capture the page's current state — the original probe error is
		// more informative than a failure from a follow-up call on a page that may have
		// navigated away or crashed.
		return "", err
	}
	return "", &unscannableFormError{reason: classifyUnscannableForm(currentHTML)}
}

// unscannableFormReason classifies why a Greenhouse posting's application form could not be
// scanned, when its known selector never appeared.
type unscannableFormReason string

const (
	// reasonCaptchaProtected means the page's HTML carries a reCAPTCHA footprint — the
	// form is gated by a challenge this package cannot pass, regardless of whether its
	// layout would otherwise be recognized.
	reasonCaptchaProtected unscannableFormReason = "form_captcha_protected"
	// reasonUnrecognizedLayout is the fallback: no known selector, no reCAPTCHA
	// footprint either — the page's form does not match any layout this package knows
	// how to read (e.g. a white-label custom domain's own bespoke DOM shape).
	reasonUnrecognizedLayout unscannableFormReason = "unrecognized_form_layout"
)

// unscannableFormError is renderedHTML's classified outcome for a form whose known selector
// never appeared, distinct from a plain error so callers can map it to a parked result
// instead of the ordinary retryable-failure path.
type unscannableFormError struct {
	reason unscannableFormReason
}

func (e *unscannableFormError) Error() string {
	return fmt.Sprintf("application form not scannable: %s", e.reason)
}

// classifyUnscannableForm inspects already-rendered HTML (captured after the known selector
// failed to appear within pageLoadTimeout) to tell a reCAPTCHA-gated form from one whose
// layout this package does not recognize. Pure, so fixture-testable like ScanForm.
//
// Narrow and named, matching resolve.go's "never guess" rule: only reCAPTCHA's own footprint
// counts. A form gated by another vendor falls back to reasonUnrecognizedLayout, still a safe
// park.
func classifyUnscannableForm(pageHTML string) unscannableFormReason {
	if hasRecaptchaMarker(pageHTML) {
		return reasonCaptchaProtected
	}
	return reasonUnrecognizedLayout
}

// recaptchaChallengeFrames are the iframes Google injects for the image-grid challenge —
// the one part of reCAPTCHA that is only ever in the DOM because a human is being asked to
// solve something. Both URL shapes, classic and Enterprise.
var recaptchaChallengeFrames = []string{
	"recaptcha/api2/bframe",
	"recaptcha/enterprise/bframe",
}

// recaptchaInvisibleBadge is the "protected by reCAPTCHA" corner badge. An invisible,
// score-based reCAPTCHA renders it INSTEAD of asking anything, so it is the positive
// evidence that a page's reCAPTCHA is one nobody has to pass.
const recaptchaInvisibleBadge = "grecaptcha-badge"

// recaptchaWidgetMarkers is the footprint of a MOUNTED reCAPTCHA widget: the anchor iframe
// Google injects for the checkbox and the invisible badge alike. Both URL shapes.
//
// The widget's class name is deliberately NOT here: a CSS selector carries the same text
// (Lever's stylesheet ships `.g-recaptcha div` on every posting), so a class match parked
// pages with no reCAPTCHA at all. An iframe URL is only in the page text if the script put it
// there.
var recaptchaWidgetMarkers = []string{
	"recaptcha/api2/anchor",
	"recaptcha/enterprise/anchor",
}

// hasRecaptchaMarker reports whether pageHTML carries a reCAPTCHA CHALLENGE a candidate
// would have to pass, not merely a loaded reCAPTCHA. Every vanilla job-boards.greenhouse.io
// posting ships an invisible, score-based reCAPTCHA Enterprise, so a bare "recaptcha" match
// would park the one provider this package can fill. What separates the cases:
//
//   - a bframe iframe is the image-grid challenge itself, decisive on its own;
//   - otherwise the corner badge means the invisible kind, which asks nothing;
//   - otherwise a mounted widget with no badge is a checkbox the candidate must tick.
//
// A page gated by a different vendor's challenge never shows the form selector, so it still
// parks as reasonUnrecognizedLayout.
func hasRecaptchaMarker(pageHTML string) bool {
	lower := strings.ToLower(pageHTML)
	if containsAny(lower, recaptchaChallengeFrames) {
		return true
	}
	if strings.Contains(lower, recaptchaInvisibleBadge) {
		return false
	}
	return containsAny(lower, recaptchaWidgetMarkers)
}

// containsAny reports whether s contains any of markers. Literal scans beat one regexp
// alternation on a page-sized string, and this runs on every scanned posting.
func containsAny(s string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
