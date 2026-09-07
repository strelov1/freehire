package browser

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Session is one running browser and a small pool of tabs to fetch through.
//
// Fetching happens INSIDE a page rather than by navigating to the URL, and that is the whole
// design. A site that gates its content behind a JavaScript challenge issues a clearance the
// browser then carries; a navigation would re-render the whole page for every request, while
// an in-page fetch reuses the same cleared context and returns just the bytes. Measured
// against the site this was built for: 4.2s a page by navigation, 0.36s by in-page fetch.
//
// The obvious third option does not work and was measured too: taking the browser's clearance
// cookie and handing it to net/http still gets the challenge back. Clearance is bound to more
// than the cookie — a TLS fingerprint is the likely other half — so the request has to leave
// from the browser itself. That is why this package exists at all rather than a cookie jar.
type Session struct {
	cancel context.CancelFunc
	tabs   chan *tab
	closed sync.Once
}

// tab is one page context. chromedp serialises actions on a context, so a tab serves one
// request at a time and concurrency is a matter of how many there are.
type tab struct {
	ctx     context.Context
	cancel  context.CancelFunc
	cleared map[string]bool // origins this tab has already passed the challenge for

	// docs records the document responses seen since the last navigation began. Clearance is
	// detected as ANY of them succeeding, not as the last one succeeding: a challenge page can
	// pull in frames of its own, and a frame refused after the real page loaded would otherwise
	// look like the wall going back up. The full list rides the timeout error, so a failure
	// says what the site actually answered instead of only that it did not work.
	docMu   sync.Mutex
	docSeen []int
}

func (t *tab) resetDocuments() {
	t.docMu.Lock()
	t.docSeen = nil
	t.docMu.Unlock()
}

func (t *tab) recordDocument(status int) {
	t.docMu.Lock()
	t.docSeen = append(t.docSeen, status)
	t.docMu.Unlock()
}

// documentSucceeded reports whether any document response since the navigation was a success,
// and renders what was seen for the error message.
func (t *tab) documentSucceeded() (bool, string) {
	t.docMu.Lock()
	defer t.docMu.Unlock()
	ok := false
	parts := make([]string, 0, len(t.docSeen))
	for _, st := range t.docSeen {
		if st >= 200 && st < 400 {
			ok = true
		}
		parts = append(parts, strconv.Itoa(st))
	}
	if len(parts) == 0 {
		return ok, "no document response at all"
	}
	return ok, strings.Join(parts, ",")
}

// clearanceWait bounds how long a tab keeps retrying a request that still looks challenged. It
// is a CEILING, not a delay: the request is retried and returns the moment it stops being
// refused, so a site that clears in a second costs a second.
const clearanceWait = 20 * time.Second

// clearancePoll is how long to wait between retries of a still-challenged request.
const clearancePoll = 250 * time.Millisecond

// fetchTimeout bounds one in-page fetch. An order of magnitude over the measured 0.36s, so a
// slow response is waited out and a hung one is not.
const fetchTimeout = 45 * time.Second

// NewSession launches a browser with the shared stealth options, egressing through proxyURL
// when one is given, and opens tabs tabs. Close it when done: nothing here is pooled across
// sessions, so one crawl's cookies can never leak into the next.
func NewSession(ctx context.Context, proxyURL string, tabs int) (*Session, error) {
	if tabs < 1 {
		tabs = 1
	}
	opts, err := LaunchOptionsThroughProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	user, pass, err := proxyCredentials(proxyURL)
	if err != nil {
		return nil, err
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	cancel := func() {
		cancelBrowser()
		cancelAlloc()
	}
	// Start the browser process before opening tabs off it, so a launch failure is reported
	// here rather than as a confusing error on the first fetch.
	if err := chromedp.Run(browserCtx); err != nil {
		cancel()
		return nil, fmt.Errorf("browser: launch: %w", err)
	}

	s := &Session{cancel: cancel, tabs: make(chan *tab, tabs)}
	for i := 0; i < tabs; i++ {
		t, err := newTab(browserCtx, user, pass)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.tabs <- t
	}
	return s, nil
}

func newTab(browserCtx context.Context, user, pass string) (*tab, error) {
	tabCtx, cancelTab := chromedp.NewContext(browserCtx)
	if err := chromedp.Run(tabCtx); err != nil {
		cancelTab()
		return nil, fmt.Errorf("browser: open tab: %w", err)
	}
	if user != "" {
		if err := enableProxyAuth(tabCtx, user, pass); err != nil {
			cancelTab()
			return nil, err
		}
	}
	if err := hideHeadlessUserAgent(tabCtx); err != nil {
		cancelTab()
		return nil, err
	}
	t := &tab{ctx: tabCtx, cancel: cancelTab, cleared: map[string]bool{}}
	if err := t.watchDocuments(); err != nil {
		cancelTab()
		return nil, err
	}
	return t, nil
}

// hideHeadlessUserAgent replaces the browser's own "HeadlessChrome/..." user agent with the
// same string minus that word.
//
// It is the loudest tell there is, and unlike navigator.webdriver no launch flag removes it.
// Measured against a real bot wall on 2026-09-07: with the default headless agent the site
// answered 429 and never served a challenge at all, so there was nothing for the browser to
// solve; with the word removed the same navigation cleared in about eight seconds. Everything
// else — the flags, the tab structure, the proxy — was identical in both runs.
//
// The agent is READ from the running browser rather than written as a literal, so it never
// claims a Chrome version this binary is not. A hardcoded string would start lying the day
// Chrome updates, which for a fingerprint is worse than not setting one.
func hideHeadlessUserAgent(ctx context.Context) error {
	var ua string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &ua)); err != nil {
		return fmt.Errorf("browser: read user agent: %w", err)
	}
	honest := strings.ReplaceAll(ua, "HeadlessChrome", "Chrome")
	if honest == ua {
		return nil // not a headless build; nothing to hide
	}
	if err := chromedp.Run(ctx, emulation.SetUserAgentOverride(honest)); err != nil {
		return fmt.Errorf("browser: set user agent: %w", err)
	}
	return nil
}

// watchDocuments records the status of every main-document response, which is what clear
// waits on. Nothing is fetched to find out — the browser is already telling us.
func (t *tab) watchDocuments() error {
	chromedp.ListenTarget(t.ctx, func(ev any) {
		if e, ok := ev.(*network.EventResponseReceived); ok && e.Type == network.ResourceTypeDocument {
			t.recordDocument(int(e.Response.Status))
		}
	})
	if err := chromedp.Run(t.ctx, network.Enable()); err != nil {
		return fmt.Errorf("browser: enable network events: %w", err)
	}
	return nil
}

// enableProxyAuth answers Chrome's proxy authentication over the debugging protocol. The
// credentials deliberately never reach the command line (see launchFlags), so this is the
// only path they travel.
//
// Enabling Fetch means every request is paused until something continues it, so the handler
// must answer both events — a paused request that nobody continues hangs the page.
func enableProxyAuth(ctx context.Context, user, pass string) error {
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
					Response: fetch.AuthChallengeResponseResponseProvideCredentials,
					Username: user,
					Password: pass,
				}))
			}()
		case *fetch.EventRequestPaused:
			go func() { _ = chromedp.Run(ctx, fetch.ContinueRequest(e.RequestID)) }()
		}
	})
	if err := chromedp.Run(ctx, fetch.Enable().WithHandleAuthRequests(true)); err != nil {
		return fmt.Errorf("browser: enable proxy auth: %w", err)
	}
	return nil
}

// Close tears the browser down. Safe to call more than once.
func (s *Session) Close() {
	s.closed.Do(func() {
		close(s.tabs)
		for t := range s.tabs {
			t.cancel()
		}
		s.cancel()
	})
}

// Fetch returns the response status and body for rawURL, obtaining clearance for its origin
// first if this tab does not have it yet.
//
// A non-2xx is NOT an error. The caller decides what a status means — for a job catalogue,
// only 404 and 410 may be read as "this posting is gone", while 403 and 429 mean we are
// blocked and nothing may be concluded about the posting at all. Collapsing them here would
// take that decision away.
func (s *Session) Fetch(ctx context.Context, rawURL string) (int, []byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, nil, fmt.Errorf("browser: parse url: %w", err)
	}
	origin := u.Scheme + "://" + u.Host

	t, err := s.take(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer s.put(t)

	if err := t.visit(ctx, origin, false); err != nil {
		return 0, nil, err
	}
	status, body, err := t.fetch(ctx, rawURL)
	if err != nil || !challenged(status) {
		return status, body, err
	}

	// The wall came back mid-crawl: clearance lapsed, or this particular request tripped it.
	// Re-clear ONCE and try again. Deliberately once and not a loop — the obstacle is a
	// rate limit, so repeated refused requests are what keeps it up rather than what gets
	// past it.
	if err := t.visit(ctx, origin, true); err != nil {
		return status, body, nil // report the refusal we have rather than the re-clear failure
	}
	return t.fetch(ctx, rawURL)
}

func (s *Session) take(ctx context.Context) (*tab, error) {
	select {
	case t, ok := <-s.tabs:
		if !ok {
			return nil, errors.New("browser: session is closed")
		}
		return t, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Session) put(t *tab) {
	defer func() {
		// A put after Close races with the channel being closed; losing the tab to a closing
		// session is fine, panicking on the way out is not.
		_ = recover()
	}()
	s.tabs <- t
}

// derive builds the context a chromedp action runs under. It must descend from the TAB's
// context, because chromedp carries the browser it belongs to in context values and an
// action run under anything else fails with "invalid context" — deriving from the caller's
// context instead is the obvious mistake and it compiles.
//
// The caller's context still cancels the work: a watcher cancels the derived one when the
// caller's is done. Merging two contexts has no standard form, and doing it in five lines
// here is better than dropping the caller's cancellation on the floor.
func (t *tab) derive(caller context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(t.ctx, timeout)
	stop := make(chan struct{})
	go func() {
		select {
		case <-caller.Done():
			cancel()
		case <-stop:
		}
	}()
	return ctx, func() {
		close(stop)
		cancel()
	}
}

// clear loads the origin once so the tab holds whatever the site issues to a client that ran
// its JavaScript. Whether that clearance is shared across the tabs of one browser profile is
// unmeasured, and nothing here depends on the answer: each tab clears for itself, and if it
// turns out to be shared the later tabs simply find the wait already over.
func (t *tab) visit(ctx context.Context, origin string, force bool) error {
	if t.cleared[origin] && !force {
		return nil
	}
	navCtx, cancel := t.derive(ctx, clearanceWait+fetchTimeout)
	defer cancel()
	t.resetDocuments()
	if err := chromedp.Run(navCtx, chromedp.Navigate(origin)); err != nil {
		return fmt.Errorf("browser: visit %s: %w", origin, err)
	}

	// Wait for the wall to lift, by watching the page rather than by poking the site. An
	// earlier version retried the REQUEST every 250ms instead, and that both failed and made
	// things worse: the obstacle is a rate-limit-flavoured 429, so a burst of refused fetches
	// is exactly what keeps it up. Measured against the real site, hammering never cleared in
	// 20s while simply waiting cleared in about 8.
	//
	// The condition is the main document turning 2xx. A challenge serves the document with a
	// refusal and then reloads itself, so this observes the reload landing — no extra request,
	// and no site-specific marker to recognise.
	deadline := time.Now().Add(clearanceWait)
	for {
		ok, seen := t.documentSucceeded()
		if ok {
			t.cleared[origin] = true
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("browser: visit %s: no document succeeded in %s (saw %s)",
				origin, clearanceWait, seen)
		}
		select {
		case <-time.After(clearancePoll):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// challenged reports whether a status is what a bot wall answers with while it is still in the
// way. Only these two: everything else — including 404 — is the site answering for real, and
// waiting for it to change would hang on a page that is simply not there.
func challenged(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusForbidden
}

// fetchResult is what the in-page script hands back. The body is base64 so a binary or
// non-UTF-8 response survives the trip, and so a body containing the separator cannot be
// mistaken for the framing.
type fetchResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Err    string `json:"err"`
}

// fetchScript issues the request from the page's own context, so it carries the page's
// clearance, cookies and TLS identity. Errors are returned in-band rather than thrown,
// because a rejected promise reaches Go as an opaque evaluation failure.
const fetchScript = `(async () => {
  try {
    const r = await fetch(%s, {credentials: "include"});
    const b = await r.arrayBuffer();
    let s = "";
    const bytes = new Uint8Array(b);
    const chunk = 0x8000;
    for (let i = 0; i < bytes.length; i += chunk) {
      s += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
    }
    return {status: r.status, body: btoa(s), err: ""};
  } catch (e) {
    return {status: 0, body: "", err: String(e)};
  }
})()`

func (t *tab) fetch(ctx context.Context, rawURL string) (int, []byte, error) {
	fetchCtx, cancel := t.derive(ctx, fetchTimeout)
	defer cancel()

	quoted, err := jsString(rawURL)
	if err != nil {
		return 0, nil, err
	}
	var res fetchResult
	if err := chromedp.Run(fetchCtx, chromedp.Evaluate(
		fmt.Sprintf(fetchScript, quoted), &res, awaitPromise,
	)); err != nil {
		return 0, nil, fmt.Errorf("browser: fetch %s: %w", rawURL, err)
	}
	if res.Err != "" {
		return 0, nil, fmt.Errorf("browser: fetch %s: %s", rawURL, res.Err)
	}
	body, err := base64.StdEncoding.DecodeString(res.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("browser: decode body of %s: %w", rawURL, err)
	}
	return res.Status, body, nil
}

// awaitPromise makes chromedp resolve the async expression rather than hand back a Promise.
func awaitPromise(p *runtime.EvaluateParams) *runtime.EvaluateParams {
	return p.WithAwaitPromise(true)
}

// jsString renders a URL as a JavaScript string literal. The URL is interpolated into a
// script, so it is quoted rather than concatenated — a URL is attacker-adjacent input on a
// crawl, since it comes from somebody else's sitemap.
func jsString(s string) (string, error) {
	if strings.ContainsAny(s, "\x00\n\r") {
		return "", fmt.Errorf("browser: url contains a control character: %q", s)
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '<', '>', '&':
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}
