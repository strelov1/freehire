package boardresolve

import (
	"context"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/atsdetect"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// ResolveCompany follows actual careers links from the employer's published
// website. The same board recognizer handles every fetched page; no ATS hostname
// is guessed from an employer name. Eight pages and two hops bound discovery.
func (r *Resolver) ResolveCompany(ctx context.Context, website string) (source, board, canonical string, ok bool) {
	type page struct {
		url   string
		depth int
	}
	queue := []page{{url: website}}
	seen := map[string]bool{website: true}
	for i := 0; i < len(queue) && i < 8; i++ {
		p := queue[i]
		base, err := url.Parse(p.url)
		if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Hostname() == "" || base.User != nil {
			continue
		}
		body, err := r.http.GetText(ctx, p.url)
		if err != nil {
			continue
		}
		// Keep the observed vendor URL, including any data-residency host.
		for _, raw := range atsdetect.AbsURLRe.FindAllString(body, -1) {
			if s, b, c, found := atsboard.Recognize(raw); found {
				return s, b, c, true
			}
		}
		if s, b, c, found := resolveContent(body, p.url); found {
			return s, b, c, true
		}
		if p.depth >= 2 {
			continue
		}
		root, err := html.Parse(strings.NewReader(body))
		if err != nil {
			continue
		}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "a" {
				href := sources.Attr(n, "href")
				lower := strings.ToLower(href)
				career := false
				for _, word := range []string{"career", "job", "vacan", "work-for", "join-us", "karriere", "stellen", "werken"} {
					career = career || strings.Contains(lower, word)
				}
				if u, err := base.Parse(href); career && err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil {
					u.Fragment = ""
					if raw := u.String(); !seen[raw] && len(queue) < 8 {
						seen[raw] = true
						queue = append(queue, page{url: raw, depth: p.depth + 1})
					}
				}
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(root)
	}
	return "", "", "", false
}
