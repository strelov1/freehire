package sources

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// proxiedProviders is the opt-in allowlist of providers whose crawl must egress through
// the configured proxy because the prod datacenter IP is anti-bot IP-blocklisted by their
// edge (eightfold 403s every prod-IP request while a residential IP is served normally).
// Membership here is the per-provider opt-in: only these route through the proxy when one
// is configured; every other provider stays on the direct IP. Each value rebuilds the
// adapter over the proxied client, so adding the next blocked provider is one line.
//
// Only providers served by the standard client belong here — the fingerprint-client
// providers (bayt/gulftalent) would need proxy support wired into fingerprintHTTP instead.
var proxiedProviders = map[string]func(HTTPClient) Source{
	// Also paced (pacedEightfoldGetter): the proxy alone recovers an outright IP blocklist,
	// but eightfold's failures are volume rate-limiting — its own ~290-req/window cap, spent
	// by this crawl's own aggregate rate across the shared proxy IP. See pacer.go.
	"eightfold": func(c HTTPClient) Source { return NewEightfold(pacedEightfoldGetter(c)) },
	"wantapply": func(c HTTPClient) Source { return NewWantapply(c) },
	// djinni.co IP-blocklists the prod datacenter IP: a fast crawl escalates to a hard "your
	// IP has been blocked" page, while a residential IP is served the full JSON-LD listing.
	"djinni": func(c HTTPClient) Source { return NewDjinni(c) },
	// 2gis tarpits datacenter IPs (the prod IP times out at 25s); the residential proxy is
	// served in ~2s. It uses the standard HTMLGetter, so the proxied client serves it directly.
	"2gis": func(c HTTPClient) Source { return NewTwoGIS(c) },
	// kontur.ru throttles datacenter IPs the same way: from both hydra and prod every HTML page
	// stalls after ~20 KB and never completes, while the residential proxy is served the whole
	// listing in ~2s (measured 2026-10-15). NewKontur takes an HTMLGetter.
	"kontur": func(c HTTPClient) Source { return NewKontur(c) },
	// careers-page.com rate-limits the prod datacenter IP (429) once the per-board detail
	// fan-out exceeds its window — every large board then under-fills silently (board_health
	// stays green) and even the single-request listing 429s during the cooldown. Unlike the
	// others this is volume rate-limiting, not a hard blocklist (spaced requests from the prod
	// IP pass), so egressing through a fresh proxy IP keeps its crawl off the penalised prod IP.
	// Also rate-paced (pacedHTMLGetter) so a full run stays under the window even on the
	// fresh proxy IP — concurrency limits the burst, pacing limits the total-per-window.
	"careerspage": func(c HTTPClient) Source {
		return NewCareerPage(pacedHTMLGetter(c, careerspageRequestInterval, careerspageRequestBurst))
	},
	// cleverstaff.net is untested from the prod datacenter IP (the spike ran from a residential
	// IP). It is pre-wired here so that, if the prod IP is blocked like djinni's, setting
	// SOURCES_PROXY_URL routes only this provider through the proxy with no code change; while
	// the proxy is unset this entry is inert.
	"cleverstaff": func(c HTTPClient) Source { return NewCleverstaff(c) },
	// peopleforce.io rate-limits the prod datacenter IP (429): a full 61-board run's
	// listing+detail volume poisons the IP after the first few boards, and every later
	// board's single listing GET then 429s. Like careerspage this is volume rate-limiting,
	// not a hard blocklist, so egressing through a fresh proxy IP keeps the crawl off the
	// penalised prod IP; the adapter's narrow detail pool bounds the per-board burst.
	"peopleforce": func(c HTTPClient) Source { return NewPeopleForce(c) },
	// onstrider.com sits behind Cloudflare and is untested from the prod datacenter IP (the
	// spike ran from a residential IP). It is pre-wired here so that, if the prod IP is blocked
	// like djinni's, setting SOURCES_PROXY_URL routes only this provider through the proxy with
	// no code change; while the proxy is unset this entry is inert.
	"onstrider": func(c HTTPClient) Source {
		return NewOnstrider(c, os.Getenv("ONSTRIDER_REFERRAL_HANDLE"))
	},
	// geekjob.ru is a Russian board reached only over the prod datacenter IP in production and is
	// untested from it (the spike ran from a residential IP). Like its RU sibling habr_career it
	// fetches a per-vacancy detail page for the description, so a WAF challenge on the detail HTML
	// would leave jobs with empty descriptions. It is pre-wired here so that, if the prod IP is
	// blocked, setting SOURCES_PROXY_URL routes only this provider through the proxy with no code
	// change; while the proxy is unset this entry is inert. A fixed, trusted host (SSRF caveat).
	"geekjob": func(c HTTPClient) Source { return NewGeekjob(c) },
	// career.habr.com sits behind Qrator, which challenges the per-vacancy detail HTML from the
	// prod datacenter IP (the listing JSON passes, but the description parse fails, leaving jobs
	// with empty descriptions and so no derived skills/geo/enrichment). A residential IP is served
	// the full page, so the whole crawl egresses through the proxy — like the others, listing and
	// detail alike. A fixed, trusted host, so it satisfies the SSRF caveat above.
	"habr_career": func(c HTTPClient) Source { return NewHabrCareer(c) },
	// vagas.com.br blocks the prod datacenter IP (403 on the first listing GET) AND rate-limits
	// by a per-IP request budget (429 once a full crawl's volume hits one IP). So it egresses
	// through the proxy like careerspage AND is rate-paced (pacedHTMLGetter) to hold its
	// aggregate request rate under vagas's window on the single proxy IP — concurrency bounds the
	// burst, pacing bounds the total-per-window. NewVagas takes an HTMLGetter, which HTTPClient
	// satisfies, and the pacer wraps it before NewVagas. This is the FALLBACK: production crawls
	// it through the Scrapling tier (scraplingtier.go), which overrides this entry.
	"vagas": func(c HTTPClient) Source {
		return NewVagas(pacedHTMLGetter(c, vagasRequestInterval, vagasRequestBurst))
	},
	// infojobs.net refuses the crawler host's IP outright (405 on every page, with or without a
	// browser User-Agent, since 2026-10-08). Through the proxy a plain client is challenged from
	// the sixth listing page, so this is only the FALLBACK: production crawls it through the
	// Scrapling tier's browser over the proxy (scraplingtier.go), which overrides this entry.
	"infojobs": func(c HTTPClient) Source { return NewInfoJobs(c) },
	// enlizt.me hard-blocks the prod datacenter IP (403 on the board listing even with a
	// browser User-Agent, so it is IP reputation, not a bot tell), while the residential
	// proxy IP is served 200. NewEnlizt takes an HTMLGetter, which HTTPClient satisfies.
	"enlizt": func(c HTTPClient) Source { return NewEnlizt(c) },
	// naukri.com's Akamai edge tarpits both of our datacenter IPs (HTTP/1.1 hangs, HTTP/2
	// resets with INTERNAL_ERROR — a headless Chrome included), while the residential proxy is
	// served in ~1-2s (measured 2026-10-18 from prod and the crawl host). The app User-Agent
	// the API requires is re-applied over the proxied client, and the pacer wraps it.
	"naukri": func(c HTTPClient) Source {
		return NewNaukri(pacedNaukriGetter(withUserAgent(c, naukriUserAgent)))
	},
}

// rotatingProxyProviders are the providers whose PROXY lane uses the ROTATING gateway
// (SOURCES_ROTATING_PROXY_URL, a fresh exit IP per request) instead of SOURCES_PROXY_URL's one
// fixed exit, when the gateway is configured. For a proxiedProvider that lane is every request;
// for a refusalRetryProvider it is only the retry of a request the direct IP was refused.
//
// naukri (proxied): its posting API answered "406 recaptcha required" to every request from the
// fixed exit (0 of 1,500 a run on 2026-10-19), while through the rotating gateway 15 of 20
// answered 200 and the other five timed out, with no refusal at all (2026-10-21). Its search
// answers either way.
//
// workable (refusal-retry): it rate-limits by exit IP with a ban measured in hours. On
// 2026-10-24 a board harvest drove the fixed exit into a 429 with retry-after ~65,000 s (18 h),
// which left the crawl's retry lane with nowhere to go for the rest of the day; the rotating
// gateway was served 5 of 5. Its widget API is one stateless GET per board, so a fresh exit per
// retry costs nothing, and no single caller can exhaust the lane for the others.
//
// Every other provider keeps the fixed exit: a fresh IP per request would break any flow that
// leans on a session.
var rotatingProxyProviders = map[string]bool{"naukri": true, "workable": true}

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

// refusalRetryProviders is the second, weaker opt-in: providers that crawl on the DIRECT IP
// and reach for the proxy only on a request the platform refused. proxiedProviders above is for
// an IP that is blocked outright — there the direct path never works, so everything must egress
// through the proxy. These two are the opposite case: the direct IP is served normally, and the
// refusals are ours, produced by the hourly fleet's own burst.
//
// Measured on prod 2026-08-16: a direct request to a "failing" teamtailor board returned 200 on
// demand, and 1207 of 1208 teamtailor board failures carried an error timestamp inside the
// current crawl hour. Workable is the same shape one status code over (429 rather than 403),
// and both grew by ~500 boards in the August harvest, so the burst is getting worse.
//
// Why not route them through the proxy wholesale: it is a single shared address (verified — six
// calls, one exit IP), and it is what eightfold, djinni and 2gis have INSTEAD of a direct
// path. Moving ~3000 boards an hour onto it would concentrate the same burst on a weaker IP and
// take the budget from the providers with nowhere else to go.
var refusalRetryProviders = map[string]func(HTTPClient) Source{
	// Paced as well as refusal-retried, like teamtailor below: rewiring here must not drop
	// the limiter the registry gave it.
	"workable": func(c HTTPClient) Source { return NewWorkable(pacedWorkableGetter(c)) },
	// Paced as well as refusal-retried: the proxy recovers a refused request, the pacer stops
	// producing them. Without the pacer this entry alone moved only a quarter of the failures.
	"teamtailor": func(c HTTPClient) Source {
		return NewTeamtailor(pacedTeamtailorGetter(c))
	},
	// ADP is the same shape as teamtailor, measured on prod 2026-09-09 after the catalogue grew
	// from 2,798 boards to 7,890: one run logged 3,028 boards refused on their listing call, and
	// board_health carried a consecutive failure for 1,962 of them. Asked in isolation minutes
	// later the direct IP served two of three sampled boards 200 and refused the third, while the
	// proxy served all three — the signature of our own burst rather than an address the platform
	// has blocked. Pacing already holds the rate; what it cannot do is shorten a run that is now
	// three times as long, so the same rate spends three times the window.
	//
	// Both ADP products are listed. They are separate platforms with separate limiters (see
	// pacedADPMyJobsGetter), and neither can recover a refusal without this.
	"adp": func(c HTTPClient) Source { return NewADP(pacedADPGetter(c)) },
	"adpmyjobs": func(c HTTPClient) Source {
		return NewADPMyJobs(pacedADPMyJobsGetter(c))
	},
}

// ApplyProxyEgress rewires the proxiedProviders in registry to egress through the proxy
// named by SOURCES_PROXY_URL (form http://user:pass@host:port). It is a no-op when the
// variable is empty (every provider stays direct) and returns an error — for fail-fast at
// worker startup — when the variable is set but unparseable, rather than silently leaving a
// meant-to-be-proxied provider on the blocked direct IP. The proxy endpoint and its
// credentials live entirely in the environment; nothing about the proxy is hardcoded.
func ApplyProxyEgress(registry map[string]Source) error {
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

// ClientFor returns the client the named provider's crawl would use under the configured
// proxy policy: wholly proxied where the platform blocks the direct datacenter IP,
// refusal-retried where it merely rate-limits it, and direct otherwise. It reads the same
// two allowlists ApplyProxyEgress does, so a provider's policy is stated once.
//
// It exists because ApplyProxyEgress can only rewire a provider's ADAPTER, and not every
// caller reaches a platform through one. cmd/harvest-boards probes a candidate board with
// a client it builds itself, so it was crawling workable on exactly the direct IP the
// workable entry in refusalRetryProviders exists to keep it off — measured on prod
// 2026-09-08, where every board probed from the prod IP returned 429 while the same boards
// through the proxy returned 200.
//
// Like ApplyProxyEgress it is a no-op with SOURCES_PROXY_URL unset, and fails rather than
// quietly falling back when the variable is set but unparseable — a caller that asked for
// the proxied policy must not be handed the blocked path instead.
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

// proxiedFingerprintProviders are the fingerprint-client providers whose edge blocks the
// direct datacenter IP even with a correct Chrome TLS fingerprint, but serves that same
// fingerprint through the residential proxy — so they need both, unlike proxiedProviders
// (proxy only) and the direct fingerprint providers (fingerprint only). gulftalent is
// spike-verified and re-verified from prod on 2026-09-01: via the same proxy, curl's JA3
// 403s while the Chrome JA3 is served the sitemap index.
//
// That entry is necessary and not sufficient, and the difference has been mistaken for
// success once. Wiring the right transport gets ONE request served; it does not get the
// crawl through. gulftalent's sitemap is thousands of detail pages funnelled through this
// single shared exit address, and a full run on 2026-09-01 was refused by the same edge
// partway through — see sources/gulftalent.yml for the measurement and why its timer is off.
// Validate a provider here by what a whole run yields, never by one 200.
//
// bayt is absent, and gulftalent's entry here is now its FALLBACK rather than its live
// transport. Both are 403 from the prod datacenter IP AND 403 through SOURCES_PROXY_URL,
// whose exit their edge classifies as datacenter too — no fingerprint helps, because the
// refusal precedes any handshake we could disguise. The chromedp browser tier did not help
// either (2026-09): what it was shown was a refusal, not a challenge.
//
// What answers both is the Scrapling tier's browser, which solves the challenge their edge does
// offer to a real browser: see scraplingProviders (scraplingtier.go), applied after this and
// deliberately overriding gulftalent's entry below. Without SCRAPLING_FETCH_URL that override
// does not happen and gulftalent keeps the fingerprint transport exactly as before — refused,
// but unchanged.
//
// Nothing is waiting for a residential pool any more: wantedkr was the last entry that
// wanted one, and it was retired on 2026-09-07 rather than kept as a permanently failing
// provider (403 from every address, robots.txt itself unreadable behind the same block).
//
// echojobs used to be named here for the same reason and no longer is: measured 2026-09-07,
// its obstacle is a JS challenge rather than an IP classification, and the EXISTING proxy
// serves that challenge rather than refusing outright. What it needs is a browser over that
// proxy, which the Scrapling tier gives it. A fingerprint does not help there; only a browser
// does.
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

// NewFormClientFromEnv builds the client application-form traffic egresses through: always
// the proxy — the rotating gateway when SOURCES_ROTATING_PROXY_URL is set, else the fixed
// SOURCES_PROXY_URL exit — and the direct client only when neither is configured, which a
// caller reports rather than takes silently.
//
// Always, not on refusal like the crawl's refusalRetryProviders: this traffic reads the forms
// candidates apply through, and the host's own address should not be the one the platforms
// learn to associate with it. It is also what had them ban that address — on 2026-10-25 every
// approved Workable auto-apply attempt died on its form fetch with a 429 and a 23-hour
// Retry-After, after cmd/capture-apply-form had read 3,369 Workable forms from it that day.
// The rotating gateway is preferred because each form fetch is one stateless GET, so a fresh
// exit per request costs nothing and no ban outlives a request.
//
// Like every proxied client it relaxes the SSRF target check (the proxy resolves the target),
// so it is for URLs that come from a fixed platform API or a stored posting URL — never for
// following an arbitrary link.
func NewFormClientFromEnv() (client *Client, proxied bool, err error) {
	rotating, err := rotatingProxy()
	if err != nil {
		return nil, false, err
	}
	if rotating != nil {
		return NewProxyClient(rotating), true, nil
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
