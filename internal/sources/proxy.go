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
	"eightfold": func(c HTTPClient) Source { return NewEightfold(c) },
	"wantapply": func(c HTTPClient) Source { return NewWantapply(c) },
	// djinni.co IP-blocklists the prod datacenter IP: a fast crawl escalates to a hard "your
	// IP has been blocked" page, while a residential IP is served the full JSON-LD listing.
	"djinni": func(c HTTPClient) Source { return NewDjinni(c) },
	// 2gis tarpits datacenter IPs (the prod IP times out at 25s); the residential proxy is
	// served in ~2s. It uses the standard HTMLGetter, so the proxied client serves it directly.
	"2gis": func(c HTTPClient) Source { return NewTwoGIS(c) },
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
	"onstrider": func(c HTTPClient) Source { return NewOnstrider(c) },
	// geekjob.ru is a Russian board reached only over the prod datacenter IP in production and is
	// untested from it (the spike ran from a residential IP). Like its RU sibling habr_career it
	// fetches a per-vacancy detail page for the description, so a WAF challenge on the detail HTML
	// would leave jobs with empty descriptions. It is pre-wired here so that, if the prod IP is
	// blocked, setting SOURCES_PROXY_URL routes only this provider through the proxy with no code
	// change; while the proxy is unset this entry is inert. A fixed, trusted host (SSRF caveat).
	"geekjob": func(c HTTPClient) Source { return NewGeekjob(c) },
	// hh.ru's detail pages 403 the direct datacenter IP, so on prod it egresses through the proxy;
	// its high-volume per-vacancy detail fan-out then 429s the single proxy IP unless paced (the
	// first prod run landed only ~34% of descriptions unpaced), so — like careerspage/vagas — it is
	// rate-paced (pacedHTMLGetter) to hold the aggregate rate under the proxy window. While
	// SOURCES_PROXY_URL is unset this entry is inert (direct crawl is unpaced, fine for local/dev).
	"hh": func(c HTTPClient) Source { return NewHH(pacedHTMLGetter(c, hhRequestInterval, hhRequestBurst)) },
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
	// satisfies, and the pacer wraps it before NewVagas.
	"vagas": func(c HTTPClient) Source {
		return NewVagas(pacedHTMLGetter(c, vagasRequestInterval, vagasRequestBurst))
	},
	// enlizt.me hard-blocks the prod datacenter IP (403 on the board listing even with a
	// browser User-Agent, so it is IP reputation, not a bot tell), while the residential
	// proxy IP is served 200. NewEnlizt takes an HTMLGetter, which HTTPClient satisfies.
	"enlizt": func(c HTTPClient) Source { return NewEnlizt(c) },
	// wanted.co.kr's job API 403s the prod datacenter IP (again 403 with a browser UA, so
	// IP-level), while the residential proxy IP is served the full JSON 200. NewWantedKR
	// takes a JSONGetter, which HTTPClient satisfies.
	"wantedkr": func(c HTTPClient) Source { return NewWantedKR(c) },
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
// calls, one exit IP), and it is what eightfold, djinni, 2gis and hh have INSTEAD of a direct
// path. Moving ~3000 boards an hour onto it would concentrate the same burst on a weaker IP and
// take the budget from the providers with nowhere else to go.
var refusalRetryProviders = map[string]func(HTTPClient) Source{
	"workable":   func(c HTTPClient) Source { return NewWorkable(c) },
	"teamtailor": func(c HTTPClient) Source { return NewTeamtailor(c) },
}

// IsProxied reports whether provider is on the proxied-egress allowlist — the providers
// ApplyProxyEgress routes through SOURCES_PROXY_URL. Callers outside the board crawl (the
// single-URL resolve path in cmd/resolve-url) use it to route the same providers through
// the same proxy.
func IsProxied(provider string) bool {
	_, ok := proxiedProviders[provider]
	return ok
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
	proxied := NewProxyClient(u)
	for name, build := range proxiedProviders {
		if _, ok := registry[name]; ok {
			registry[name] = build(proxied)
		}
	}
	// The refusal-retry providers keep the direct IP and get the proxy only for what it turned
	// away, so they are rewired over a different client than the wholly-proxied ones above.
	refusalRetry := NewRefusalRetryClient(u)
	for name, build := range refusalRetryProviders {
		if _, ok := registry[name]; ok {
			registry[name] = build(refusalRetry)
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

// proxiedFingerprintProviders are the fingerprint-client providers whose edge blocks the
// direct datacenter IP even with a correct Chrome TLS fingerprint, but serves that same
// fingerprint through the residential proxy — so they need both, unlike proxiedProviders
// (proxy only) and the direct fingerprint providers (fingerprint only). gulftalent is
// spike-verified: via the same proxy, curl's JA3 403s while the Chrome JA3 is 200. bayt is
// deliberately absent — it sits behind a Cloudflare JS challenge that no fingerprint or IP
// passes.
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
