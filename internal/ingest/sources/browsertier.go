package sources

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/platform/browser"
)

// browserProviders is the opt-in allowlist of providers whose pages are served only to a
// client that ran JavaScript, so their crawl goes through a headless browser rather than an
// HTTP client. Membership here is the per-provider opt-in, exactly as in proxiedProviders,
// and each value rebuilds the adapter over the browser-backed client — so adding the next
// challenged provider is one line.
//
// A provider here MUST also be proxied, and a test enforces it. Measured 2026-09-07 against
// echojobs, all four combinations: from the prod datacenter IP a plain GET and a headless
// browser BOTH get 403 with x-vercel-mitigated: deny — no challenge is offered at all, so
// there is nothing for a browser to solve. Through SOURCES_PROXY_URL a plain GET gets the
// challenge (429, "Vercel Security Checkpoint") and the browser passes it. Neither half works
// alone; only the pair does.
var browserProviders = map[string]func(*browserClient) Source{
	// echojobs.io moved behind Vercel's bot firewall around 2026-08-19 and every crawl since
	// reported ingested=0 with exit 0 (freehire#2588). Its adapter is unchanged — the sitemap
	// walk and the JobPosting parse always worked, and only the transport broke.
	"echojobs": func(c *browserClient) Source { return NewEchoJobs(c) },
}

// browserTabs is how many pages the session fetches through at once. It follows
// defaultDetailWorkers rather than inventing a second number: the pool exists for the same
// reason the detail-worker pool does, and one knob is easier to reason about than two.
const browserTabs = defaultDetailWorkers

// ApplyBrowserEgress rewires the browserProviders in registry onto a headless browser that
// egresses through SOURCES_PROXY_URL, and returns the function that shuts that browser down.
// The returned function is always safe to call, including after an error.
//
// It is a no-op without a proxy: a browser on the direct IP is refused before any challenge
// is offered, so launching one would spend seconds to fail. A set-but-unparseable proxy is an
// error, the same fail-fast contract ApplyProxyEgress has — for a provider that cannot work
// without it, a quiet fallback looks exactly like a crawl that found nothing, which is the
// failure this whole tier exists to end.
//
// No browser starts here. The session is built on the first fetch, so an ingest run for any
// other provider — which is almost every run, since cmd/ingest crawls one provider — never
// pays for a Chrome it will not use.
func ApplyBrowserEgress(registry map[string]Source) (func(), error) {
	raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL"))
	if raw == "" {
		return func() {}, nil
	}
	if !anyBrowserProviderRegistered(registry) {
		return func() {}, nil
	}
	// Parse now rather than on the first fetch, so a typo fails the run at startup instead of
	// halfway through a crawl.
	if _, err := browser.LaunchOptionsThroughProxy(raw); err != nil {
		return func() {}, fmt.Errorf("sources: browser tier: %w", err)
	}

	lazy := &lazySession{proxyURL: raw}
	for name, build := range browserProviders {
		if _, ok := registry[name]; ok {
			registry[name] = build(newBrowserClient(lazy))
		}
	}
	return lazy.Close, nil
}

func anyBrowserProviderRegistered(registry map[string]Source) bool {
	for name := range browserProviders {
		if _, ok := registry[name]; ok {
			return true
		}
	}
	return false
}

// lazySession starts the browser on first use and hands the same one to every later fetch.
// One browser per run, not per request: the clearance a tab holds is the expensive part, and
// throwing it away between fetches would pay for it over and over.
type lazySession struct {
	proxyURL string

	once    sync.Once
	session *browser.Session
	err     error
}

func (l *lazySession) get(ctx context.Context) (*browser.Session, error) {
	l.once.Do(func() {
		// Deliberately NOT the caller's ctx: the session outlives the one fetch that happened
		// to start it, and tying its lifetime to that request's context would tear the browser
		// down the moment that fetch returned.
		l.session, l.err = browser.NewSession(context.WithoutCancel(ctx), l.proxyURL, browserTabs)
	})
	return l.session, l.err
}

func (l *lazySession) Close() {
	if l.session != nil {
		l.session.Close()
	}
}
