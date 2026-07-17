// Package boardresolve is the network fallback for the paste-a-link contribution flow: when a
// URL's host is not a recognized ATS (e.g. a company careers page on its own domain with an
// embedded ATS — company.com/careers?gh_jid=…), it fetches the page and detects the embedded
// board via internal/atsdetect. It satisfies contribution.Resolver.
//
// Only providers whose (provider, board) matches how the ingest pipeline namespaces
// jobs.external_id are accepted, so the resolved board dedups correctly against the catalogue.
// The fetch uses the SSRF-guarded sources client (it refuses internal/metadata targets), since
// the URL is attacker-influenced.
package boardresolve

import (
	"context"
	"net/url"

	"github.com/strelov1/freehire/internal/atsdetect"
	"github.com/strelov1/freehire/internal/sources"
)

// trusted lists the providers whose atsdetect (provider, board) equals the ingest
// external_id namespace, so a resolved board is dedup-correct. Greenhouse/Lever/Ashby/Workable
// embed the board slug directly (verified against prod external_ids).
var trusted = map[string]bool{
	"greenhouse": true,
	"lever":      true,
	"ashby":      true,
	"workable":   true,
}

// textFetcher is the slice of the sources client this package needs (a raw, SSRF-guarded,
// size-capped GET). *sources.Client satisfies it.
type textFetcher interface {
	GetText(ctx context.Context, url string) (string, error)
}

// Resolver fetches an unrecognized careers page and detects the embedded ATS board.
type Resolver struct {
	http textFetcher
}

// New builds a Resolver over the default SSRF-guarded sources client.
func New() *Resolver { return &Resolver{http: sources.NewClient()} }

// Resolve fetches rawURL and detects an embedded ATS board, returning the catalogue
// (source, board) and a canonical URL to store (query/fragment stripped). ok=false when the
// fetch fails, no board is detected, or the detected provider is not one we trust to match the
// ingest namespace.
func (r *Resolver) Resolve(ctx context.Context, rawURL string) (source, board, canonical string, ok bool) {
	html, err := r.http.GetText(ctx, rawURL)
	if err != nil {
		return "", "", "", false
	}
	provider, slug, ok := atsdetect.Detect(html)
	if !ok || !trusted[provider] || slug == "" {
		return "", "", "", false
	}
	return provider, slug, stripTails(rawURL), true
}

// stripTails returns rawURL without its query string or fragment (the identifying part of a
// vanity careers URL is the path; the board itself comes from the page). Falls back to the raw
// string if it does not parse.
func stripTails(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
