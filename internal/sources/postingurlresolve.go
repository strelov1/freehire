package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// PostingURLResolver canonicalises a posting link the way CanonicalPostingURL does, plus
// the shapes that cannot be rewritten offline because the link identifies the posting by
// something the catalogue does not store.
//
// SmartRecruiters is the case that forced it: its Apply button leaves the posting page for
// /oneclick-ui/company/<board>/publication/<uuid>, and that uuid is a second identifier the
// crawl never keeps — the catalogue holds the numeric posting id and the detail URL. Only
// SmartRecruiters can say which posting a publication is, so this asks, and asks for the
// posting's own URL rather than its id: the id would have to be re-namespaced by board, and
// the board's spelling in a URL ("Blend360") is not the spelling the board file carries
// ("blend360"), so an id match would miss where a URL match lands.
//
// The zero value is usable and stays offline — it degrades to the pure rewrite instead of
// panicking, so a caller with no client is merely limited, never broken.
type PostingURLResolver struct {
	http JSONGetter
}

// NewPostingURLResolver builds a resolver that may call out through c.
func NewPostingURLResolver(c JSONGetter) PostingURLResolver { return PostingURLResolver{http: c} }

// CanonicalPostingURL returns the posting's own detail URL for raw, and raw unchanged when
// it already is one, is unrecognised, or the platform does not answer. A failed lookup is
// deliberately not an error: the caller is matching a page against the catalogue and can
// still try the URL as it stands.
func (r PostingURLResolver) CanonicalPostingURL(ctx context.Context, raw string) string {
	if board, publication, ok := smartRecruitersPublication(raw); ok && r.http != nil {
		if posting, ok := r.smartRecruitersPostingURL(ctx, board, publication); ok {
			return posting
		}
		return raw
	}
	return CanonicalPostingURL(raw)
}

// smartRecruitersPublicationPath matches the one-click apply link SmartRecruiters' Apply
// button leads to: /oneclick-ui/company/<board>/publication/<uuid>.
var smartRecruitersPublicationPath = regexp.MustCompile(
	`^/oneclick-ui/company/([^/]+)/publication/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})/?$`,
)

// smartRecruitersPublication reads the board and publication uuid out of a one-click apply
// link, reporting false for every other URL.
func smartRecruitersPublication(raw string) (board, publication string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", "", false
	}
	switch strings.ToLower(u.Hostname()) {
	case "jobs.smartrecruiters.com", "careers.smartrecruiters.com":
	default:
		return "", "", false
	}
	m := smartRecruitersPublicationPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// smartRecruitersPostingURL asks SmartRecruiters which posting a publication uuid is. Its
// postings endpoint accepts either identifier, and answers with the posting's public detail
// URL — the same string the crawl stored in jobs.url.
func (r PostingURLResolver) smartRecruitersPostingURL(ctx context.Context, board, publication string) (string, bool) {
	var d struct {
		PostingURL string `json:"postingUrl"`
	}
	endpoint := fmt.Sprintf("%s/%s/postings/%s", smartRecruitersBaseURL, url.PathEscape(board), url.PathEscape(publication))
	if err := r.http.GetJSON(ctx, endpoint, &d); err != nil {
		return "", false
	}
	posting := strings.TrimSpace(d.PostingURL)
	return posting, posting != ""
}
