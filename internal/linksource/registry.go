// Package linksource turns an outbound job link found in a Telegram post into a fully
// parsed vacancy under the destination's own identity. Where internal/sources adapts a
// whole ATS board by id, a Source adapts a single job-detail URL: it matches the
// link's host and resolves that one page. Adding a destination is a new adapter plus one
// line in All — the same shape as sources.All.
package linksource

import (
	"context"
	"net/url"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/sources"
)

// Client is the transport a Source needs: a server-rendered detail page (optionally
// following a shortener redirect to learn the canonical URL) or a structured JSON API
// (multi-tenant ATS adapters read the platform's public per-job endpoint). *sources.Client
// satisfies it.
type Client interface {
	GetHTML(ctx context.Context, url string) (*html.Node, error)
	GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error)
	GetJSON(ctx context.Context, url string, v any) error
}

// Source adapts one destination site reachable from a post link. Source is the key
// stored as jobs.source; Match reports whether this adapter handles a link URL (by host,
// including any shortener that fronts the site); Resolve fetches and parses that one
// vacancy.
type Source interface {
	Source() string
	Match(u *url.URL) bool
	// Resolve fetches and parses the destination vacancy at raw. ok=false means the link
	// is matched but is not a single vacancy (e.g. a listing/search page) and should be
	// skipped — not an error. A non-nil error is a transient/parse failure worth retrying.
	Resolve(ctx context.Context, raw string) (job sources.Job, ok bool, err error)
}

// constructors lists every link-source adapter builder. All and AllWithProxyEgress build
// the registry from this single list, so adding a destination is one line here.
func constructors() []func(Client) Source {
	return []func(Client) Source{
		NewHabrCareer,
		NewRemoteYeah,
		NewGeekjob,
		NewGreenhouse,
		NewAshby,
		NewLever,
		NewWorkable,
		NewBairesDev,
	}
}

// All assembles the registered link-source adapters, sharing one HTTP client.
func All(c Client) []Source {
	ctors := constructors()
	reg := make([]Source, len(ctors))
	for i, ctor := range ctors {
		reg[i] = ctor(c)
	}
	return reg
}

// AllWithProxyEgress assembles the registry like All but routes the providers on the
// sources proxied allowlist (sources.IsProxied) through proxy, leaving every other adapter
// on direct — the single-URL analogue of sources.ApplyProxyEgress. The board crawl proxies
// blocked providers (habr_career/geekjob sit behind Qrator/a WAF that challenges the prod
// datacenter IP); a single-URL resolve of those same hosts must egress the same way or its
// detail fetch is challenged and the description comes back empty. A nil proxy leaves every
// adapter on direct, so a caller can pass the parsed SOURCES_PROXY_URL client or nil
// uniformly.
func AllWithProxyEgress(direct, proxy Client) []Source {
	ctors := constructors()
	reg := make([]Source, len(ctors))
	for i, ctor := range ctors {
		c := direct
		// Source() is a constant method, so a nil-client probe reads the provider name
		// without a request — the same nil-client construction All(nil) already relies on.
		if proxy != nil && sources.IsProxied(ctor(nil).Source()) {
			c = proxy
		}
		reg[i] = ctor(c)
	}
	return reg
}

// Find returns the first adapter that matches u, or nil when no destination handles it.
func Find(reg []Source, u *url.URL) Source {
	for _, ls := range reg {
		if ls.Match(u) {
			return ls
		}
	}
	return nil
}
