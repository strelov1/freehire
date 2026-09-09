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

// stealthAllocatorOptions are this package's launch options. The flags themselves live in
// platform/browser, which is their single home in the repository: ingest also launches a
// browser now (for a source behind a JavaScript challenge), and two copies of the flags would
// mean the next anti-bot fix lands in one and silently not the other.
//
// What that package documents, and what still holds here: the pair is what the 2026-09-02
// spike measured against bot.sannysoft.com, and it is not a claim of undetectability — see
// design.md's "chromedp, not a Python/Patchright sidecar" decision for the caveats.
func stealthAllocatorOptions() []chromedp.ExecAllocatorOption {
	return browser.LaunchOptions()
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
	return newBrowserSession(ctx, c.allocatorOpts)
}

// newBrowserSession is newBrowser's body, free of *Client so PreviewClient (which shares
// the same allocator options but none of Client's LLM/CV dependencies) can start a session
// the same way rather than duplicating it.
func newBrowserSession(ctx context.Context, allocatorOpts []chromedp.ExecAllocatorOption) (context.Context, context.CancelFunc, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocatorOpts...)
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
// the same way ScanForm already is — no further browser interaction needed to
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

// recaptchaWidgetMarkers are the footprints of a reCAPTCHA widget being mounted at all: the
// checkbox/badge iframe, and the widget element's own class before the script replaces it.
// Matched with the class's delimiter attached so `g-recaptcha-response` — the hidden token
// field EVERY reCAPTCHA page carries, invisible ones included — is not read as a widget.
var recaptchaWidgetMarkers = []string{
	"recaptcha/api2/anchor",
	"recaptcha/enterprise/anchor",
	`g-recaptcha"`,
	`g-recaptcha'`,
	"g-recaptcha ",
}

// hasRecaptchaMarker reports whether pageHTML carries a reCAPTCHA CHALLENGE — something a
// candidate would have to pass — as opposed to merely loading reCAPTCHA at all.
//
// The distinction is the whole point of this function, and it is not a nicety. EVERY
// vanilla job-boards.greenhouse.io posting ships an invisible, score-based reCAPTCHA
// Enterprise. The earlier unscoped `strings.Contains(html, "recaptcha")` therefore matched
// every Greenhouse posting there is, and through previewByLayout's own check that parked
// the ONE provider this package can actually fill — before a single application was ever
// attempted (freehire, 2026-09-08: 5 of the 10 entries the queue had ever held, all of them
// on pages whose form scans fine in under two seconds).
//
// Presence of a widget cannot be the test, which is what a first attempt at this fix
// assumed and a capture of the live DOM disproved: an invisible reCAPTCHA mounts an anchor
// iframe too. What actually separates the two:
//
//   - a bframe iframe is the image-grid challenge itself, and is in the DOM only when one
//     is genuinely being posed — decisive on its own;
//   - otherwise, the corner badge means the page's reCAPTCHA is the invisible kind, which
//     asks nothing;
//   - otherwise, a mounted widget with no badge is a checkbox the candidate must tick.
//
// A page gated by a different vendor's challenge is unaffected either way: its form's own
// selector never appears, so it still parks as reasonUnrecognizedLayout.
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

// containsAny reports whether s contains any of markers. Literal scans rather than one
// regexp alternation, for the reason internal/dict/skilltag documents for its own
// dictionary: alternation over a page-sized string is a fraction of the speed, and this
// runs on every scanned posting.
func containsAny(s string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
