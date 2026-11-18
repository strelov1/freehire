package sources

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// proxiedProviders is the opt-in allowlist of providers whose crawl must egress through the
// configured proxy because the prod datacenter IP is anti-bot blocklisted by their edge. Only these
// route through the proxy when one is configured; every other provider stays direct. Each value
// rebuilds the adapter over the proxied client.
//
// Only providers served by the standard client belong here: the fingerprint-client providers
// (bayt/gulftalent) need proxy support wired into fingerprintHTTP instead.
var proxiedProviders = map[string]func(HTTPClient) Source{
	// Optomi's public REST API refuses the crawl host but serves the existing proxy.
	"bullhorn": func(c HTTPClient) Source { return NewBullhorn(c) },
	// Also paced (pacedEightfoldGetter): the proxy alone recovers an outright IP blocklist, but
	// eightfold's failures are volume rate-limiting (a per-window cap spent by this crawl's own
	// aggregate rate across the shared proxy IP). See pacer.go.
	"eightfold":    func(c HTTPClient) Source { return NewEightfold(pacedEightfoldGetter(c)) },
	"wantapply-cy": func(c HTTPClient) Source { return NewWantapplyCy(c) },
	// djinni.co IP-blocklists the prod datacenter IP: a fast crawl escalates to a hard "your IP has
	// been blocked" page, while a residential IP is served the full JSON-LD listing.
	"djinni": func(c HTTPClient) Source { return NewDjinni(c) },
	// 2gis tarpits datacenter IPs (the prod IP times out); the residential proxy is served quickly.
	// It uses the standard HTMLGetter, so the proxied client serves it directly.
	"2gis": func(c HTTPClient) Source { return NewTwoGIS(c) },
	// kontur.ru throttles datacenter IPs the same way: every HTML page stalls after ~20 KB and never
	// completes, while the residential proxy is served the whole listing. NewKontur takes an HTMLGetter.
	"kontur": func(c HTTPClient) Source { return NewKontur(c) },
	// careers-page.com rate-limits the prod datacenter IP (429) once the per-board detail fan-out
	// exceeds its window: large boards then under-fill silently (board_health stays green) and even
	// the single-request listing 429s during the cooldown. This is volume rate-limiting, not a hard
	// blocklist, so a fresh proxy IP keeps the crawl off the penalised prod IP. Also rate-paced
	// (pacedHTMLGetter): concurrency limits the burst, pacing limits the total per window.
	"careerspage": func(c HTTPClient) Source {
		return NewCareerPage(pacedHTMLGetter(c, careerspageRequestInterval, careerspageRequestBurst))
	},
	// cleverstaff.net is untested from the prod datacenter IP. It is pre-wired so that, if the prod IP
	// is blocked like djinni's, setting SOURCES_PROXY_URL routes only this provider through the proxy
	// with no code change; while the proxy is unset this entry is inert.
	"cleverstaff": func(c HTTPClient) Source { return NewCleverstaff(c) },
	// peopleforce.io rate-limits the prod datacenter IP (429): a full run's listing+detail volume
	// poisons the IP after the first few boards. Like careerspage this is volume rate-limiting, not a
	// hard blocklist, so a fresh proxy IP keeps the crawl off the penalised prod IP; the adapter's
	// narrow detail pool bounds the per-board burst.
	"peopleforce": func(c HTTPClient) Source { return NewPeopleForce(c) },
	// onstrider.com sits behind Cloudflare and is untested from the prod datacenter IP. Pre-wired
	// like cleverstaff: while the proxy is unset this entry is inert.
	"onstrider": func(c HTTPClient) Source {
		return NewOnstrider(c, os.Getenv("ONSTRIDER_REFERRAL_HANDLE"))
	},
	// geekjob.ru is untested from the prod datacenter IP. Like its RU sibling habr_career it fetches
	// a per-vacancy detail page for the description, so a WAF challenge there would leave jobs with
	// empty descriptions. Pre-wired like cleverstaff: while the proxy is unset this entry is inert. A
	// fixed, trusted host (SSRF caveat).
	"geekjob": func(c HTTPClient) Source { return NewGeekjob(c) },
	// career.habr.com sits behind Qrator, which challenges the per-vacancy detail HTML from the prod
	// datacenter IP (the listing JSON passes, but the description parse fails, leaving jobs with empty
	// descriptions and so no derived skills/geo/enrichment). A residential IP is served the full page,
	// so listing and detail alike egress through the proxy. A fixed, trusted host, so it satisfies the
	// SSRF caveat above.
	"habr_career": func(c HTTPClient) Source { return NewHabrCareer(c) },
	// vagas.com.br blocks the prod datacenter IP (403 on the first listing GET) AND rate-limits by a
	// per-IP request budget (429 once a full crawl's volume hits one IP). So it egresses through the
	// proxy and is rate-paced (pacedHTMLGetter): concurrency bounds the burst, pacing bounds the total
	// per window. NewVagas takes an HTMLGetter, and the pacer wraps it before NewVagas. This is the
	// FALLBACK: production crawls it through the Scrapling tier (scraplingtier.go), which overrides
	// this entry.
	"vagas": func(c HTTPClient) Source {
		return NewVagas(pacedHTMLGetter(c, vagasRequestInterval, vagasRequestBurst))
	},
	// infojobs.net refuses the crawler host's IP outright (405 on every page). Through the proxy a
	// plain client is challenged from the sixth listing page, so this is only the FALLBACK:
	// production crawls it through the Scrapling tier's browser over the proxy (scraplingtier.go),
	// which overrides this entry.
	"infojobs": func(c HTTPClient) Source { return NewInfoJobs(c) },
	// enlizt.me hard-blocks the prod datacenter IP (403 on the board listing even with a browser
	// User-Agent, so it is IP reputation, not a bot tell), while the residential proxy IP is served.
	// NewEnlizt takes an HTMLGetter, which HTTPClient satisfies.
	"enlizt": func(c HTTPClient) Source { return NewEnlizt(c) },
	// njoyn.com (CGI's careers site) redirects the crawl host's datacenter IP to Radware's bot wall
	// (validate.perfdrive.com, a CAPTCHA page) on the first request, while a residential IP is
	// served. The adapter paces itself per board, so the proxied path is paced too.
	"njoyn": func(c HTTPClient) Source { return NewNjoyn(c) },
	// tenshoku.mynavi.jp's origin answers 403 to both crawl hosts' datacenter IPs (IP reputation
	// rather than a bot tell), while the proxy is served. Paced on the proxy as on the direct path.
	"mynavi": func(c HTTPClient) Source {
		return NewMynavi(pacedHTMLGetter(c, mynaviRequestInterval, mynaviRequestBurst))
	},
	// naukri.com's Akamai edge tarpits both of our datacenter IPs (HTTP/1.1 hangs, HTTP/2 resets with
	// INTERNAL_ERROR, a headless Chrome included), while the residential proxy is served. The app
	// User-Agent the API requires is re-applied over the proxied client, and the pacer wraps it.
	// api.cake.me answers a Cloudflare challenge to both crawl hosts' IPs, and caps one exit at
	// ~40 requests a minute (429 with Retry-After). It rides the rotating exit; see
	// rotatingProxyProviders.
	"cake": func(c HTTPClient) Source { return NewCake(c) },
	// net-empregos.com serves both crawl hosts' datacenter IPs its candidate LOGIN page (HTTP 200) in
	// place of every posting page, whatever the User-Agent, while its feeds still answer. It walls an
	// exit by volume: the fixed proxy exit is served a lone request and walled ~40 requests into a
	// crawl, so the provider rides the rotating gateway (see rotatingProxyProviders). NewNetEmpregos
	// takes a TextGetter.
	"netempregos": func(c HTTPClient) Source { return NewNetEmpregos(c) },
	"naukri": func(c HTTPClient) Source {
		return NewNaukri(pacedNaukriGetter(withUserAgent(c, naukriUserAgent)))
	},
}

// rotatingProxyProviders are the providers whose PROXY lane uses the ROTATING gateway
// (SOURCES_ROTATING_PROXY_URL, a fresh exit IP per request) instead of SOURCES_PROXY_URL's one
// fixed exit, when the gateway is configured. For a proxiedProvider that lane is every request;
// for a refusalRetryProvider it is only the retry of a request the direct IP was refused.
//
// naukri (proxied): its posting API answers "406 recaptcha required" to every request from the
// fixed exit, while through the rotating gateway it answers 200 with no refusal. Its search
// answers either way.
//
// workable (refusal-retry): it rate-limits by exit IP with a ban measured in hours (a 429 with a
// retry-after of ~18 h on the fixed exit), which leaves the crawl's retry lane with nowhere to go.
// Its widget API is one stateless GET per board, so a fresh exit per retry costs nothing, and no
// single caller can exhaust the lane for the others.
//
// cake (proxied): one exit is refused after ~40 requests a minute, and a crawl is ~800 searches
// plus a detail per new posting, all stateless, so each request takes a fresh exit.
//
// netempregos (proxied): the site answers an exit that has read too many postings (~40) with its
// candidate login page instead of the posting. Every request is a stateless GET.
//
// Every other provider keeps the fixed exit: a fresh IP per request would break any flow that
// leans on a session.
var rotatingProxyProviders = map[string]bool{"naukri": true, "workable": true, "cake": true, "netempregos": true}

// proxyFor is the proxy a provider's proxy lane egresses through: the rotating gateway for the
// rotatingProxyProviders when one is configured, the fixed exit otherwise.
func proxyFor(provider string, fixed, rotating *url.URL) *url.URL {
	if rotating != nil && rotatingProxyProviders[provider] {
		return rotating
	}
	return fixed
}

// rotatingProxy reads SOURCES_ROTATING_PROXY_URL: nil when unset, an error when set but
// unparseable.
func rotatingProxy() (*url.URL, error) {
	raw := strings.TrimSpace(os.Getenv("SOURCES_ROTATING_PROXY_URL"))
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("sources: invalid SOURCES_ROTATING_PROXY_URL %q", redactProxy(raw))
	}
	return u, nil
}

// refusalRetryProviders is the second, weaker opt-in: providers that crawl on the DIRECT IP and
// reach for the proxy only on a request the platform refused. proxiedProviders is for an IP
// blocked outright, where everything must egress through the proxy. These are the opposite case:
// the direct IP is served normally, and the refusals are produced by the hourly fleet's own burst
// (a direct request to a "failing" teamtailor board returns 200 on demand). Workable is the same
// shape with 429 rather than 403.
//
// They are not routed through the proxy wholesale: it is a single shared address, and moving
// thousands of boards an hour onto it would concentrate the same burst on a weaker IP and take
// the budget from the providers with nowhere else to go.
var refusalRetryProviders = map[string]func(HTTPClient) Source{
	// Paced as well as refusal-retried, like teamtailor below: rewiring here must not drop
	// the limiter the registry gave it.
	"workable": func(c HTTPClient) Source { return NewWorkable(pacedWorkableGetter(c)) },
	// Paced as well as refusal-retried: the proxy recovers a refused request, the pacer stops
	// producing them. Without the pacer this entry alone moved only a quarter of the failures.
	"teamtailor": func(c HTTPClient) Source {
		return NewTeamtailor(pacedTeamtailorGetter(c))
	},
	// ADP is the same shape as teamtailor: boards are refused on their listing call by the fleet's
	// own burst, while the direct IP serves most of the same boards in isolation and the proxy serves
	// all of them. Pacing already holds the rate; what it cannot do is shorten a run that grew with
	// the catalogue, so the same rate spends a longer window.
	//
	// Both ADP products are listed. They are separate platforms with separate limiters (see
	// pacedADPMyJobsGetter), and neither can recover a refusal without this.
	"adp": func(c HTTPClient) Source { return NewADP(pacedADPGetter(c)) },
	"adpmyjobs": func(c HTTPClient) Source {
		return NewADPMyJobs(pacedADPMyJobsGetter(c))
	},
}

// ApplyProxyEgress rewires the proxiedProviders in registry to egress through the proxy named by
// SOURCES_PROXY_URL (form http://user:pass@host:port). It is a no-op when the variable is empty
// (every provider stays direct) and returns an error, for fail-fast at worker startup, when it is
// set but unparseable, rather than silently leaving a provider on the blocked direct IP. The proxy
// endpoint and credentials live entirely in the environment.
func ApplyProxyEgress(registry map[string]Source) error {
	// MojPosao's Croatian market is refused by its CloudFront WAF on both crawl hosts and the
	// fixed exit; its lane pins exits of the ROTATING gateway (mojposaoLane), so it needs only
	// that gateway and is wired whether or not the fixed exit is configured. The Bosnian market
	// stays direct.
	if _, ok := registry["mojposao"]; ok {
		rotating, err := rotatingProxy()
		if err != nil {
			return err
		}
		if rotating != nil {
			registry["mojposao"] = newMojPosaoWithLane(NewClient(), newMojposaoLane(rotating))
		}
	}
	raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL"))
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Redacted() strips any password so the invalid value can be surfaced safely.
		return fmt.Errorf("sources: invalid SOURCES_PROXY_URL %q", redactProxy(raw))
	}
	rotating, err := rotatingProxy()
	if err != nil {
		return err
	}
	proxied := NewProxyClient(u)
	for name, build := range proxiedProviders {
		if _, ok := registry[name]; !ok {
			continue
		}
		client := proxied
		if p := proxyFor(name, u, rotating); p != u {
			client = NewRotatingProxyClient(p)
		}
		registry[name] = build(client)
	}
	// The refusal-retry providers keep the direct IP and get the proxy only for what it turned
	// away, so they are rewired over a different client than the wholly-proxied ones above.
	refusalRetry := NewRefusalRetryClient(u)
	for name, build := range refusalRetryProviders {
		if _, ok := registry[name]; !ok {
			continue
		}
		client := refusalRetry
		if p := proxyFor(name, u, rotating); p != u {
			client = NewRotatingRefusalRetryClient(p)
		}
		registry[name] = build(client)
	}
	// The SEEK network keeps the direct IP and gets a proxy lane for its GraphQL detail only,
	// taken after SEEK refuses the direct one (seekDetailNetwork). Unlike refusalRetryProviders
	// the refusal is a 200 carrying a GraphQL error, which the client cannot see, so the switch
	// is the adapter's. Both providers are rebuilt over ONE network so they share its state.
	seekDirect := NewClient()
	seekNetwork := newSeekDetailNetwork(pacedSeekPoster(seekDirect), pacedSeekProxyPoster(NewProxyClient(u)))
	for name, build := range seekNetworkProviders {
		if _, ok := registry[name]; ok {
			registry[name] = build(seekDirect, seekNetwork)
		}
	}
	// Homerun keeps the direct IP and moves the rest of a run onto the ROTATING gateway after
	// its edge first refuses the direct one (homerunEgress). Never the fixed exit: one address
	// is exactly what Homerun blocks once a run's volume has landed on it.
	if _, ok := registry["homerun"]; ok && rotating != nil {
		registry["homerun"] = NewHomerun(pacedHomerunGetter(newHomerunEgress(NewClient(), NewRotatingProxyClient(rotating))))
	}
	// Fingerprint-proxied providers need BOTH the Chrome fingerprint and the proxy IP, so they
	// are rewired over a proxied fingerprint transport rather than the standard proxied client.
	// Build it once, only when one such provider is actually registered, and fail fast if the
	// transport build fails rather than silently leaving the provider on the blocked direct IP.
	if anyRegistered(registry, proxiedFingerprintProviders) {
		fp, err := newProxiedFingerprintHTTP(raw)
		if err != nil {
			return err
		}
		for name, build := range proxiedFingerprintProviders {
			if _, ok := registry[name]; ok {
				registry[name] = build(fp)
			}
		}
	}
	return nil
}

// ClientFor returns the client the named provider's crawl would use under the configured proxy
// policy: wholly proxied where the platform blocks the direct datacenter IP, refusal-retried where
// it merely rate-limits it, and direct otherwise. It reads the same two allowlists
// ApplyProxyEgress does, so a provider's policy is stated once.
//
// It exists because ApplyProxyEgress can only rewire a provider's ADAPTER, and not every caller
// reaches a platform through one: cmd/harvest-boards probes a candidate board with a client it
// builds itself, and would otherwise crawl workable on the direct IP that the refusalRetryProviders
// entry exists to keep it off.
//
// Like ApplyProxyEgress it is a no-op with SOURCES_PROXY_URL unset, and fails rather than quietly
// falling back when the variable is set but unparseable.
func ClientFor(provider string) (*Client, error) {
	raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL"))
	if raw == "" {
		return NewClient(), nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("sources: invalid SOURCES_PROXY_URL %q", redactProxy(raw))
	}
	rotating, err := rotatingProxy()
	if err != nil {
		return nil, err
	}
	p := proxyFor(provider, u, rotating)
	switch {
	case proxiedProviders[provider] != nil && p != u:
		return NewRotatingProxyClient(p), nil
	case proxiedProviders[provider] != nil:
		return NewProxyClient(u), nil
	case refusalRetryProviders[provider] != nil && p != u:
		return NewRotatingRefusalRetryClient(p), nil
	case refusalRetryProviders[provider] != nil:
		return NewRefusalRetryClient(u), nil
	default:
		return NewClient(), nil
	}
}

// proxiedFingerprintProviders are the fingerprint-client providers whose edge blocks the direct
// datacenter IP even with a correct Chrome TLS fingerprint, but serves that fingerprint through
// the residential proxy, so they need both, unlike proxiedProviders (proxy only) and the direct
// fingerprint providers (fingerprint only).
//
// That entry is necessary and not sufficient. The right transport gets ONE request served, not the
// crawl through: gulftalent's sitemap is thousands of detail pages funnelled through one shared
// exit address, and a full run was refused by the same edge partway through (see
// sources/gulftalent.yml and why its timer is off). Validate a provider here by what a whole run
// yields, never by one 200.
//
// bayt is absent, and gulftalent's entry is its FALLBACK rather than its live transport. Both are
// 403 from the prod datacenter IP AND through SOURCES_PROXY_URL, whose exit their edge classifies as
// datacenter too; no fingerprint helps, because the refusal precedes any handshake, and the chromedp
// browser tier was shown a refusal, not a challenge. What answers both is the Scrapling tier's
// browser, which solves the challenge their edge offers a real browser: see scraplingProviders
// (scraplingtier.go), applied after this and overriding gulftalent's entry below. Without
// SCRAPLING_FETCH_URL that override does not happen and gulftalent keeps the fingerprint transport,
// refused but unchanged.
//
// echojobs is not named here: its obstacle is a JS challenge rather than an IP classification, and
// the existing proxy serves that challenge. It needs a browser over that proxy, which the Scrapling
// tier gives it; a fingerprint does not help.
var proxiedFingerprintProviders = map[string]func(*fingerprintHTTP) Source{
	"gulftalent": func(c *fingerprintHTTP) Source { return NewGulfTalent(c) },
}

// anyRegistered reports whether the registry contains at least one of the named providers,
// so a transport is built only when it will actually be used.
func anyRegistered(registry map[string]Source, providers map[string]func(*fingerprintHTTP) Source) bool {
	for name := range providers {
		if _, ok := registry[name]; ok {
			return true
		}
	}
	return false
}

// redactProxy returns a proxy URL string with any credentials removed, so a malformed
// value can appear in an error without leaking the password.
func redactProxy(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Redacted()
	}
	// Unparseable: fall back to the scheme+host prefix so we never echo embedded creds.
	if i := strings.Index(raw, "@"); i >= 0 {
		return "<redacted>" + raw[i:]
	}
	return raw
}

// NewFormClientFromEnv builds the client application-form traffic egresses through: always the
// proxy (the rotating gateway when SOURCES_ROTATING_PROXY_URL is set, else the fixed
// SOURCES_PROXY_URL exit), and the direct client only when neither is configured, which a caller
// reports rather than takes silently.
//
// Always, not on refusal like refusalRetryProviders: the host's own address should not be the one
// the platforms learn to associate with form reads, and Workable bans that address (a 429 with a
// Retry-After of hours) once capture-apply-form has read enough forms from it. The rotating
// gateway is preferred because each form fetch is one stateless GET, so a fresh exit per request
// costs nothing and no ban outlives a request.
//
// Like every proxied client it relaxes the SSRF target check (the proxy resolves the target), so
// it is for URLs that come from a fixed platform API or a stored posting URL, never for following
// an arbitrary link.
func NewFormClientFromEnv() (client *Client, proxied bool, err error) {
	rotating, err := rotatingProxy()
	if err != nil {
		return nil, false, err
	}
	if rotating != nil {
		return NewRotatingProxyClient(rotating), true, nil
	}
	raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL"))
	if raw == "" {
		return NewClient(), false, nil
	}
	fixed, err := url.Parse(raw)
	if err != nil || fixed.Host == "" {
		return nil, false, fmt.Errorf("sources: invalid SOURCES_PROXY_URL %q", redactProxy(raw))
	}
	return NewProxyClient(fixed), true, nil
}
