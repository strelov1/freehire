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

// PerLinkSource is a Source that serves more than one platform, so the identity a resolved
// job is stored under depends on the link rather than on the adapter. Board coverage — one
// adapter over every recognised multi-tenant ATS — is the only such source; every other
// adapter is host-scoped and its Source() is the answer. ResolveLinks prefers SourceFor when
// an adapter implements this, mirroring how sources opts into StreamingSource/HydratingSource.
type PerLinkSource interface {
	Source
	SourceFor(u *url.URL) string
}

// All assembles the registered link-source adapters, sharing one HTTP client. Adding a
// destination is a new adapter plus one line here.
func All(c Client) []Source {
	return []Source{
		NewHabrCareer(c),
		NewRemoteYeah(c),
		NewGeekjob(c),
		NewGreenhouse(c),
		NewAshby(c),
		NewLever(c),
		NewWorkable(c),
		NewBairesDev(c),
	}
}

// ImportRegistry is the resolver order for importing one job page on demand, and the reason
// the order is stated once here rather than at each call site:
//
//  1. the host-scoped adapters — a platform with a cheap per-job API must keep using it;
//  2. board coverage — every other recognised ATS, answered by fetching that tenant's board
//     through its ingest adapter, which is far more expensive than (1);
//  3. the generic JobPosting resolver — a last resort that matches ANY page, so nothing may
//     follow it (Find picks one adapter and never tries the next).
//
// ingest is the provider-keyed ingest registry; a nil one simply disables step 2. Generic is
// absent from All on purpose — its always-true Match must not leak into the Telegram crawl,
// where every outbound link would then look like a vacancy. Here the URL is a deliberate
// input, which is what makes a last-resort resolver safe.
func ImportRegistry(c Client, ingest map[string]sources.Source) []Source {
	reg := All(c)
	reg = append(reg, NewBoardCoverage(ingest))
	return append(reg, NewGeneric(c))
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
