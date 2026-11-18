package sources

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
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
// services/scrapling-fetch on the crawl host. Each row's route is the cheapest one that passes:
// `http` is one request with Chrome's TLS fingerprint; `browser` is a stealth Chromium
// navigation that solves Cloudflare challenges, slow on the first page of a host.
//
// It is applied after the proxy and fingerprint tiers and overrides them for these providers;
// without SCRAPLING_FETCH_URL nothing here runs and each keeps the transport those tiers gave it.
var scraplingProviders = map[string]scraplingProvider{
	// Public full postings are refused directly; HTTP + proxy exposes recommendations.
	"linkedin": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewLinkedIn(f)
	}},
	// Migrated PageUp sites expose sitemaps directly; real detail bodies require HTTP + proxy.
	"clinch": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		plain := NewClient()
		return NewClinch(plain, pacedHTMLGetter(clinchDetailHTTP{plain: plain, migrated: f}, clinchRequestInterval, clinchRequestBurst))
	}},
	// Only North Memorial's custom PageUp host needs this lane; canonical tenants stay direct.
	"pageup": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewPageUp(pageupCustomHTTP{plain: NewClient(), custom: f})
	}},
	// Direct HTTP returned 498; browser listing pages and vacancy details passed.
	"rwb": {scraplingRoute{mode: scraplingfetch.ModeBrowser, proxy: false}, func(f *Client) Source {
		return NewRWB(f)
	}},
	// The anonymous POST API refuses plain proxy and direct fingerprints; HTTP + proxy passes.
	"senior": {scraplingRoute{mode: scraplingfetch.ModeHTTP, proxy: true}, func(f *Client) Source {
		return NewSenior(f)
	}},
	// The fingerprint tier is refused; Scrapling HTTP direct serves both disciplines and bodies.
	"gradcracker": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewGradcracker(pacedGradcracker(f), 0)
	}},
	// Azure WAF refuses direct egress. The listing's session cookie must survive into fragments.
	"infojobsbr": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		jar, _ := cookiejar.New(nil)
		f.httpClient.Jar = jar
		return NewInfoJobsBR(f)
	}},
	// Njoyn (CGI): Radware walls the crawl host's IP and the site refuses an IP it decides is
	// crawling too fast (XWP10022) — the fixed proxy exit drew that ban within one run. HTTP + proxy
	// leaves through the rotating gateway and served the listing and posting pages (2026-11-18).
	// The adapter keeps its own 5 s pace. proxiedProviders stays the fallback without the tier.
	"njoyn": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewNjoyn(f)
	}},
	// Cloudflare refuses direct egress; HTTP + proxy serves the sitemaps and posting bodies.
	"spainjobs": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewSpainJobs(f)
	}},
	// 405 to the crawl host's IP; through the proxy, a GeeTest challenge from the sixth listing
	// page on a plain client; a browser over the same proxy passes.
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
	// The global catalogue's public listing must come from .com. The Cyprus
	// catalogue is a separate provider, never a fallback for a refused global page.
	"wantapply": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewWantapply(f)
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
	// Cloudflare answers 403 to a plain client for the posting pages and the API. The sitemap and
	// RSS answered 200 plain from a residential IP, but they share the protected host and the
	// datacenter IP is unmeasured, so the one sitemap request rides the tier too. The
	// impersonating fetcher without a proxy served 48 of 50 posting pages (the other two were the
	// site's own 404s), measured 2026-11-17 from a RESIDENTIAL IP and NOT from the crawl host: if
	// it draws challenges there, the next step is the proxy, then the browser mode.
	"cryptojobslist": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewCryptoJobsList(f, f)
	}},
	// Only the posting pages: Akamai bot management holds a plain request open without
	// answering, and the impersonating fetcher is served from the crawl host's own IP. The
	// active-offers sitemap is not behind it and stays on a plain client.
	"turijobs": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewTurijobs(NewClient(), f)
	}},
	// Cloudflare answers 403 to a plain client from the crawl host on every path, sitemap
	// included; the impersonating fetcher is served from the crawl host's own IP, with an
	// occasional 403 the adapter retries.
	"cadremploi": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewCadremploi(f)
	}},
	// Cloudflare answers 403 to a plain client from the crawl host on every path, robots.txt
	// included; the impersonating fetcher is served from the crawl host's own IP, listing and
	// offer pages alike.
	"pracuj": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewPracuj(f)
	}},
	// Only the posting pages: a Cloudflare challenge answers 403 to a plain client and to the
	// impersonating fetcher alike, and the browser passes from the crawl host's own IP. The
	// sitemaps sit on a CDN with no challenge and stay on a plain client.
	"jobteaser": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewJobTeaser(NewClient(), f)
	}},
	// 403 to the crawl host's direct IP, to the plain proxy and to Scrapling http without a
	// proxy; http over the proxy serves the listing and the detail pages. Still paced.
	"levtech": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewLevtech(pacedHTMLGetter(f, levtechRequestInterval, levtechRequestBurst))
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
	// proxy passes it.
	"echojobs": {scraplingRoute{scraplingfetch.ModeBrowser, true}, func(f *Client) Source {
		return NewEchoJobs(f)
	}},
	// Cloudflare's "Just a moment..." challenge on the site and on
	// api.wuzzuf.net too, to every plain request direct or proxied. The browser from the crawl
	// host's own IP solves it; over the proxy it timed out. Paced: see wuzzufPageInterval.
	"wuzzuf": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewWuzzuf(pacedWuzzufSite(f))
	}},
	// Cloudflare's challenge on the listing to every datacenter IP: through the proxy a plain
	// request and the impersonating fetcher pass only partly, while the browser from the crawl
	// host's own IP passes. The posting JSON on oferta.occ.com.mx is not challenged and stays on
	// a plain client with the browser-compatible User-Agent.
	"occ": {scraplingRoute{scraplingfetch.ModeBrowser, false}, func(f *Client) Source {
		return NewOCC(occSplit{HTMLGetter: f, JSONGetter: NewCompatibleUAClient()})
	}},
	// The fixed proxy exit is 429'd on every detail page once a run has spent its budget, so a
	// large hydration run times out with nothing stored. The impersonating fetcher over the
	// rotating exit mostly passes; the misses are a 403 per bad IP and are retried next run.
	// Still paced, since the site's own budget is unknown.
	"vagas": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewVagas(pacedHTMLGetter(f, vagasRequestInterval, vagasRequestBurst))
	}},
	// Only its posting pages: Cloudflare answers 403 to every plain request for them from the
	// crawl host, and the impersonating fetcher from the crawl host's own IP is served. The JSON
	// search is not challenged and stays on a plain client.
	// Cloudflare refuses the crawl host's IP for a plain client, through both proxies, and
	// shows a Go Chrome-fingerprint client its challenge; the impersonating fetcher is served.
	// Through the service's rotating proxy, not from the crawl host's own IP: that IP is
	// rate-limited per address and draws 429s even at one request every 4 s per shard, while the
	// rotating exits answer. The code tables on static.104.com.tw have no such edge and stay on a
	// plain client.
	"job104": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewJob104(f, NewClient())
	}},
	// Imperva's bot wall ("Pardon Our Interruption", HTTP 405) to the crawl host and to the fixed
	// proxy exit after a handful of requests; the browser over the proxy is refused too. The
	// impersonating fetcher over the rotating exits passes about half the requests, and the
	// adapter re-sends a 405, each attempt on a new exit.
	"epreselec": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewEPreselec(f)
	}},
	// Cloudflare's "Just a moment..." challenge (403) to every plain client, from the crawl host,
	// the fixed proxy and a laptop alike; the impersonating fetcher from the crawl host's own IP
	// is served.
	"viterbit": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewViterbit(f)
	}},
	// 403 to a plain request; the impersonating fetcher is served, ~1-2 s a page, sitemaps and
	// posting pages alike (measured 2026-11-17 from a residential IP with no proxy — the crawl
	// host's datacenter range is unmeasured).
	"glints": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return NewGlints(f)
	}},
	"instahyre": {scraplingRoute{scraplingfetch.ModeHTTP, false}, func(f *Client) Source {
		return pacedInstahyre(NewClient(), f)
	}},
	// 403 to a plain Go client on every path, sitemap and robots.txt included; the impersonating
	// fetcher is served 200, sitemap, newest-postings page and posting pages alike (measured
	// 2026-11-17, ~1.9 s a posting page, 39 of 39 pages answered from a residential IP). From the
	// crawl host the first production run drew 403 on 295 posting pages without the proxy; the same
	// pages through it answered 8 of 8 (measured the same day), hence the proxy.
	"clearedjobs": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewClearedJobs(f)
	}},
	// Akamai answers 403 to a plain client on every path, the sitemaps and robots.txt included.
	// From the prod crawl host the impersonating fetcher is refused without the proxy and served
	// through it (the search page, measured 2026-11-17). The posting PAGES through the proxy are
	// UNMEASURED: from a residential IP with no proxy the same fetcher drew 403 on every page and
	// a headless browser passed about one in three. Re-measure the first live run and move the
	// row to the browser if the pages stay refused.
	"foundit": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return NewFoundit(f)
	}},
	// Only the posting pages. The first live run (2026-11-17, crawl host) was refused 1,460 of 1,500
	// reads of the gateway's /job on every route (the Scrapling fetcher with and without the
	// proxy, plain, plain over the proxy), while the posting page answers 200 through the fetcher
	// over the proxy and 403 without it; the page carries the same posting in its server-rendered
	// state. The search answers a plain client, so it stays on one.
	"kariyernet": {scraplingRoute{scraplingfetch.ModeHTTP, true}, func(f *Client) Source {
		return pacedKariyerNet(NewClient(), f)
	}},
}

// A browser fetch that meets a challenge on its first page can take ~110 s, and the service
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
