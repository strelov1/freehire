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
	}
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
