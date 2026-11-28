package sources

import (
	"context"
	"log"
	"net/url"
	"strings"
)

var _ BoardCanonicalizer = icims{}

// CanonicalBoard rewrites a board whose sitemap redirects to another iCIMS host into that host's
// board id. iCIMS keeps an old or brand host alive as a 301 to the live one, and the crawl follows
// it, so an alias stored as its own board ingests every posting a second time under a different
// external_id namespace and company. Rewritten, the alias meets the live board in the catalogue's
// duplicate check instead.
func (s icims) CanonicalBoard(ctx context.Context, board string) (string, error) {
	r, ok := s.http.(XMLResolvedGetter)
	if !ok || board == "" {
		return board, nil // no transport, no check
	}
	var doc sitemapDoc
	final, err := r.GetXMLResolved(ctx, "https://"+IcimsHost(board)+"/sitemap.xml", &doc)
	if err != nil {
		log.Printf("icims: %s: sitemap unavailable (%v); accepting it unchecked", board, err)
		return board, nil
	}
	u, err := url.Parse(final)
	if err != nil {
		return board, nil //nolint:nilerr // an unreadable final URL leaves the board unchecked, not refused
	}
	host := strings.ToLower(u.Hostname())
	// shortcut: only a redirect onto *.icims.com is followed; a move to a client's own domain is
	// rare and could as well be a site that left iCIMS, so it keeps the board as given.
	if host == strings.ToLower(IcimsHost(board)) || !strings.HasSuffix(host, ".icims.com") {
		return board, nil
	}
	if slug, ok := strings.CutPrefix(strings.TrimSuffix(host, ".icims.com"), "careers-"); ok && slug != "" && !strings.Contains(slug, ".") {
		return slug, nil // the classic host folds to its bare slug, as the catalogue stores it
	}
	return host, nil
}
