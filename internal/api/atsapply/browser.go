package atsapply

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// stealthAllocatorOptions are the launch options the 2026-09-02 spike measured against
// bot.sannysoft.com: headless, plus disable-blink-features=AutomationControlled, which
// alone flipped navigator.webdriver from true to false — matching what Patchright achieved,
// at zero extra dependency. See design.md's "chromedp, not a Python/Patchright sidecar"
// decision for the full comparison and its caveats (untested against real ATS bot-detection,
// datacenter IP reputation unaddressed either way).
func stealthAllocatorOptions() []chromedp.ExecAllocatorOption {
	return append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
}

// pageLoadTimeout bounds how long a single navigation+render may take before this package
// gives up waiting for the known selector and falls back to classifying why it never
// appeared (see classifyTimeout). Generous: an application form pulls in its own JS bundle
// and, on Greenhouse, a client-side render pass.
const pageLoadTimeout = 20 * time.Second

// classifyTimeout bounds the one follow-up HTML capture renderedHTML makes to classify why
// the known Greenhouse selector never appeared. Deliberately ADDITIONAL to pageLoadTimeout,
// not carved out of it: an earlier version shortened the selector wait itself to make room
// for classification, and live verification against a real, ordinary vanilla-template
// posting (openspec/changes/auto-apply-whitelabel-greenhouse task 4.2) caught that
// regression directly — the shortened wait intermittently misclassified a normal,
// fillable posting as unrecognized_form_layout under real load, exactly the false-positive
// risk design.md's Risks anticipated. Keeping the full pageLoadTimeout for the selector
// wait (unchanged from before this change) and only spending extra time on classification
// after it genuinely elapses removes that regression entirely.
const classifyTimeout = 10 * time.Second

// newBrowser starts one browser process for one attempt. Callers must call the returned
// cancel to tear it down — nothing here pools or reuses a browser across attempts, so one
// attempt's session can never leak state (cookies, a half-filled form) into the next.
func (c *Client) newBrowser(ctx context.Context) (context.Context, context.CancelFunc, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, c.allocatorOpts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	cancel := func() {
		cancelBrowser()
		cancelAlloc()
	}
	// Force the browser to actually start now rather than lazily on the first action, so
	// a launch failure surfaces here instead of inside renderedHTML with a less specific
	// error.
	if err := chromedp.Run(browserCtx); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("start browser: %w", err)
	}
	return browserCtx, cancel, nil
}

// renderedHTML navigates to url, waits for readySelector to appear (the application form
// itself — proof the client-side render pass that reveals fields like Greenhouse's
// `country` has actually run), and returns the page's rendered HTML.
//
// If readySelector genuinely never appears — the probe's own pageLoadTimeout elapses, not
// some other failure — this does not treat that as an undifferentiated, retryable error: it
// spends up to classifyTimeout MORE capturing whatever HTML the page currently has and
// classifying it (classifyUnscannableForm) — a white-label custom Greenhouse domain renders
// a different DOM shape entirely, and some such pages are also gated by a reCAPTCHA
// challenge on the form itself. Either classification comes back as an *unscannableFormError
// so Client.Submit can map it to a parked result instead of a plain failure. Any OTHER
// failure (a DNS error, connection refused, a crashed tab — anything that is not "the known
// selector simply never showed up in time") propagates as an ordinary error instead: nothing
// here can safely explain it as an unscannable form, and doing so anyway would silently park
// an attempt that a normal retry might well have succeeded on. See
// openspec/changes/auto-apply-whitelabel-greenhouse/design.md.
func renderedHTML(ctx context.Context, url, readySelector string) (string, error) {
	probeCtx, probeCancel := context.WithTimeout(ctx, pageLoadTimeout)
	var pageHTML string
	err := chromedp.Run(probeCtx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(readySelector, chromedp.ByID),
		chromedp.OuterHTML("html", &pageHTML),
	)
	// Checked before cancel: once a context's Done channel closes for one reason, a later
	// cancel() call cannot overwrite Err() with a different one (see context.WithTimeout),
	// so this reliably tells "the probe's own deadline fired" apart from any other failure
	// — the same errors.Is(callCtx.Err(), context.DeadlineExceeded) idiom
	// internal/embed/runner.go and internal/searchdrain/runner.go already use for the
	// analogous "was it really this call's own budget" question.
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
	var currentHTML string
	captureErr := chromedp.Run(classifyCtx, chromedp.OuterHTML("html", &currentHTML))
	if captureErr != nil {
		// Couldn't even capture the page's current state — the original probe error is
		// more informative than a failure from a follow-up call on a page that may have
		// navigated away or crashed.
		return "", err
	}
	return "", &unscannableFormError{reason: classifyUnscannableForm(currentHTML)}
}

// greenhouseFormReadySelector is Greenhouse's own form element id, confirmed against a
// live posting in the 2026-09-02 spike. Only the vanilla job-boards.greenhouse.io template
// is known to use it — a white-label custom domain may not (see classifyUnscannableForm).
const greenhouseFormReadySelector = "application-form"

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

// classifyUnscannableForm inspects a page's already-rendered HTML (captured after its known
// selector failed to appear within pageLoadTimeout) to tell a reCAPTCHA-gated form apart
// from one whose layout this package simply does not recognize. Pure and fixture-testable,
// the same way ScanGreenhouseForm already is — no further browser interaction needed to
// classify.
//
// Narrow and named, matching resolve.go's "never guess" rule: this looks only for
// reCAPTCHA's own footprint, not a generic "something looks locked" heuristic. A form gated
// by a different challenge vendor still falls back to reasonUnrecognizedLayout — still a
// safe park, just a less specific reason (see design.md's Risks).
func classifyUnscannableForm(pageHTML string) unscannableFormReason {
	if hasRecaptchaMarker(pageHTML) {
		return reasonCaptchaProtected
	}
	return reasonUnrecognizedLayout
}

// hasRecaptchaMarker reports whether pageHTML contains "recaptcha" anywhere at all, a
// deliberately unscoped substring search rather than one parsed against a specific element
// (an injected iframe, or the script tag loading reCAPTCHA's API — both reference
// "recaptcha" in a URL, which is what a real challenge actually leaves behind). Scoping the
// search to those elements specifically would need DOM parsing this classification path is
// meant to avoid (see classifyUnscannableForm's doc comment); an ordinary application page
// mentioning the word incidentally is not a realistic false-positive risk in practice, and
// either outcome still only ever produces a safe park (see design.md's Risks) — at most a
// less specific Reason string, never a wrong fill/submit.
func hasRecaptchaMarker(pageHTML string) bool {
	return strings.Contains(strings.ToLower(pageHTML), "recaptcha")
}
