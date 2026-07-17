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
	"regexp"

	"github.com/strelov1/freehire/internal/atsdetect"
	"github.com/strelov1/freehire/internal/contribution"
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

// absURLRe extracts absolute http(s) URLs from markup (href/src or bare URLs in inline JSON),
// stopping at the first quote, angle bracket, or whitespace.
var absURLRe = regexp.MustCompile(`https?://[^\s"'<>)\\]+`)

// Resolve fetches rawURL and finds the ATS board it belongs to, returning the catalogue
// (source, board) and a canonical URL to store. It looks two ways:
//  1. the Greenhouse embed shape (script for=<board>) via atsdetect — which the URL recognizer
//     can't parse (it would read the path word "embed" as the board);
//  2. any supported ATS apply/board URL embedded in the page, run through the full
//     contribution.RecognizeBoard (all ~40 ATS, all modes) — this catches a company careers page
//     that links to its recruitee/peopleforce/zoho/workday board.
//
// ok=false when the fetch fails or no board is found.
func (r *Resolver) Resolve(ctx context.Context, rawURL string) (source, board, canonical string, ok bool) {
	html, err := r.http.GetText(ctx, rawURL)
	if err != nil {
		return "", "", "", false
	}

	// 1. Greenhouse embed (and atsdetect's own gh/lever/ashby URL scan). Trusted set only.
	if provider, slug, ok := atsdetect.Detect(html); ok && trusted[provider] && slug != "" {
		return provider, slug, stripTails(rawURL), true
	}

	// 2. Any supported ATS URL in the page, via the full recognizer. First recognized wins;
	//    a Greenhouse embed URL misparses to board "embed" (step 1 owns Greenhouse), so skip it.
	for _, u := range absURLRe.FindAllString(html, -1) {
		if s, b, _, matched := contribution.RecognizeBoard(u); matched && b != "embed" {
			return s, b, stripTails(rawURL), true
		}
	}
	return "", "", "", false
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
