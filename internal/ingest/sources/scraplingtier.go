package sources

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
)

// scraplingRoute is how one provider's requests reach the Scrapling fetch service: which of its
// two fetchers, and whether through the proxy the service holds.
type scraplingRoute struct {
	mode  scraplingfetch.Mode
	proxy bool
}

// scraplingProvider rebuilds one provider over the tier; fetch is the tier's client for the
// provider's route. A provider that sends only PART of its crawl through the tier builds its own
// plain client for the rest (hh, startupjobs).
type scraplingProvider struct {
	route scraplingRoute
	build func(fetch *Client) Source
}

// scraplingProviders is the allowlist of providers no plain client can reach, crawled through
// services/scrapling-fetch on the crawl host. Each row's route is the cheapest one measured to
// pass on 2026-10-20 (docs/superpowers/specs/2026-10-20-scrapling-fetch-design.md has the
// per-mode table): `http` is one request with Chrome's TLS fingerprint, ~0.2-2 s; `browser` is a
// stealth Chromium navigation that solves Cloudflare challenges, ~5-9 s once the first page on
// a host has paid for the challenge (~110 s on bayt).
//
// It replaced the hosted Firecrawl tier, which carried bayt, gulftalent, wellfound, wantapply's
// sitemap and hh's detail pages at a credit per page and ran out of credits in 2026-10.
//
// It is applied after the proxy and fingerprint tiers and overrides them for these providers;
// without SCRAPLING_FETCH_URL nothing here runs and each keeps the transport those tiers gave it.
var scraplingProviders = map[string]scraplingProvider{
	// 405 to the crawl host's IP; through the proxy, a GeeTest challenge from the sixth listing
	// page on a plain client, and 20 of 20 pages in a browser over the same proxy.
	"infojobs": {scraplingRoute{scraplingfetch.ModeBrowser, true}, func(f *Client) Source {
		return NewInfoJobs(f)
	}},
	// Cloudflare-managed challenge: 403 to every plain request, direct or proxied; the browser
	// solves it from the crawl host's own IP.
	"bayt": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewBayt(f)
	}},
	// Its sitemap answered a Cloudflare challenge to http + proxy; the browser reads sitemap and
	// pages alike.
	"gulftalent": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewGulfTalent(f)
	}},
	// .com's WAF refuses every plain request and the .cy mirror answers most of the catalogue
	// with an empty shell (see the type doc), so sitemap and pages alike come from .com.
	"wantapply": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewWantapplyOnCom(f)
	}},
	// 403 to a plain request; the impersonating fetcher through the proxy is served. Its role
	// slices are crawled concurrently, so the in-flight cap the hosted tier needed is kept.
	"wellfound": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewWellfound(limitedWellfoundGetter(f))
	}},
	// Only detail hydration: through the proxy the detail pages redirect to a DDoS-Guard
	// captcha, and the impersonating fetcher reads them direct. The listing keeps a fresh plain
	// client, never a proxied one: its own pages were never blocked.
	"hh": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewHHWithDetailGetter(NewClient(), f)
	}},
	// Only its Cloudflare-challenged pages, still paced: the sitemap CDN and the public Algolia
	// index have no Cloudflare in front and stay on the plain client.
	"startupjobs": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		plain := NewClient()
		return NewStartupJobs(pacedStartupJobsSite(f), plain, plain, 0)
	}},
	// 403 to the Go client's TLS fingerprint and to every proxied request; Chrome's fingerprint
	// from the crawl host is served.
	"cleverstaff": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewCleverstaff(f)
	}},
	// The crawl host's IP times out; the impersonating fetcher through the proxy is served.
	"kontur": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewKontur(f)
	}},
	// The crawl host's IP times out at TCP connect; through the proxy the impersonating fetcher
	// reads both the XHR listing fragment and the detail pages.
	"neogov": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewNeogov(f)
	}},
	// Vercel's bot firewall: 403 to the crawl host's IP in any client, and through the proxy a
	// plain request draws the 429 "Security Checkpoint" about half the time. The browser over the
	// proxy passes it, 10 of 10 pages measured on 2026-10-21. It replaced the chromedp browser
	// tier, which the checkpoint had started answering with a 403.
	"echojobs": {scraplingRoute{scraplingfetch.ModeBrowser, true}, func(f *Client) Source {
		return NewEchoJobs(f)
	}},
	// Cloudflare's "Just a moment..." challenge on the site and, since 2026-10-20, on
	// api.wuzzuf.net too, to every plain request direct or proxied. The browser from the crawl
	// host's own IP solves it; over the proxy it timed out. Paced: see wuzzufPageInterval.
	"wuzzuf": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewWuzzuf(pacedWuzzufSite(f))
	}},
	// Cloudflare's challenge on the listing to every datacenter IP since 2026-10: through the
	// proxy a plain request passed 4 of 10 pages and the impersonating fetcher 7 of 10, while the
	// browser from the crawl host's own IP read 10 of 10. The posting JSON on oferta.occ.com.mx
	// is not challenged and stays on a plain client with the browser-compatible User-Agent.
	"occ": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewOCC(occSplit{HTMLGetter: f, JSONGetter: NewCompatibleUAClient()})
	}},
	// The fixed proxy exit is 429'd on every detail page once a run has spent its budget — 15 of
	// 15 measured on 2026-10-22, so a run hydrating 600 postings timed out with none stored. The
	// impersonating fetcher over the rotating exit read 18 of 20; the misses are a 403 per bad IP
	// and are retried next run. Still paced, since the site's own budget is unknown.
	"vagas": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewVagas(pacedHTMLGetter(f, vagasRequestInterval, vagasRequestBurst))
	}},
	// Only its posting pages: Cloudflare answers 403 to every plain request for them from the
	// crawl host, and the impersonating fetcher from the crawl host's own IP is served (2026-10-25,
	// ~0.4 s a page). The JSON search is not challenged and stays on a plain client.
	// Cloudflare refuses the crawl host's IP for a plain client, through both proxies, and
	// shows a Go Chrome-fingerprint client its challenge; the impersonating fetcher is served.
	// Through the service's rotating proxy, not from the crawl host's own IP: that IP is
	// rate-limited per address, and the first sharded passes drew 429s on 1-19 boards per shard
	// even at one request every 4 s per shard, while the rotating exits answered 40 of 40 at
	// ~3 s a page (2026-10-26). The code tables on static.104.com.tw have no such edge and stay
	// on a plain client.
	"job104": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewJob104(f, NewClient())
	}},
	"instahyre": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return pacedInstahyre(NewClient(), f)
	}},
}

// A browser fetch that meets a challenge on its first page measured ~110 s, and the service
// queues a request for a free browser for up to 120 s; the timeout covers both.
const (
	scraplingBrowserTimeout = 5 * time.Minute
	scraplingHTTPTimeout    = 90 * time.Second
)

// newScraplingClient builds an ordinary Client whose requests go through the fetch service, so
// every getter, the retries and the body cap stay the ones every other provider uses.
func newScraplingClient(cfg scraplingfetch.Config, route scraplingRoute) (*Client, error) {
	timeout := scraplingHTTPTimeout
	if route.mode == scraplingfetch.ModeBrowser {
		timeout = scraplingBrowserTimeout
	}
	tr, err := scraplingfetch.New(cfg, route.mode, route.proxy, timeout)
	if err != nil {
		return nil, err
	}
	c := newClientWithProxy(nil)
	// No outer timeout: the transport's own client already bounds the whole fetch, and a second
	// clock of the same length would cut off the service's error text just before it arrives.
	c.httpClient = &http.Client{Transport: tr}
	c.streamClient = &http.Client{Transport: tr}
	// No retries: the service already retries once, and a browser fetch can take minutes, so the
	// client's own two would turn one dead page into a quarter of an hour.
	c.maxRetries = 0
	return c, nil
}

// ApplyScraplingEgress rebuilds the scraplingProviders in registry over the fetch service named
// by SCRAPLING_FETCH_URL. It is a no-op when that is unset — which is also the rollback — and
// fails startup when it is set but unusable (no SCRAPLING_FETCH_TOKEN, a malformed URL), rather
// than letting every crawl through the tier fail with a 401 that reads like the source's refusal.
func ApplyScraplingEgress(registry map[string]Source) error {
	raw := strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_URL"))
	if raw == "" {
		return nil
	}
	cfg := scraplingfetch.Config{URL: raw, Token: strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_TOKEN"))}
	// Validate once, up front, so a half-configured deploy fails startup even when this worker
	// crawls none of the allowlisted providers.
	if _, err := scraplingfetch.New(cfg, scraplingfetch.ModeHTTP, false, scraplingHTTPTimeout); err != nil {
		return fmt.Errorf("sources: scrapling tier: %w", err)
	}
	for name, p := range scraplingProviders {
		if _, ok := registry[name]; !ok {
			continue
		}
		c, err := newScraplingClient(cfg, p.route)
		if err != nil {
			return fmt.Errorf("sources: scrapling tier: %w", err)
		}
		registry[name] = p.build(c)
	}
	return nil
}
