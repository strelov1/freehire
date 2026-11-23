package boardresolve

import (
	"context"
	"net/url"
	"slices"
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
	source, board, canonical, _, ok = r.ResolveCompanyWithCareerHosts(ctx, website)
	return source, board, canonical, ok
}

// ResolveCompanyWithCareerHosts also reports up to four observed external career
// hosts for follow-up when discovery is unresolved.
func (r *Resolver) ResolveCompanyWithCareerHosts(ctx context.Context, website string) (source, board, canonical string, careerHosts []string, ok bool) {
	noteHost := func(host string) {
		host = strings.TrimSuffix(strings.ToLower(host), ".")
		if host != "" && len(careerHosts) < 4 && !slices.Contains(careerHosts, host) {
			careerHosts = append(careerHosts, host)
		}
	}
	type page struct {
		url   string
		depth int
	}
	queue := []page{{url: website}}
	seen := map[string]bool{website: true}
	for i := 0; i < len(queue) && i < 8; i++ {
		p := queue[i]
		if multiEmployerIndex(p.url) {
			continue
		}
		base, err := url.Parse(p.url)
		if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Hostname() == "" || base.User != nil {
			continue
		}
		body, final, err := r.fetchPage(ctx, p.url)
		if err != nil {
			continue
		}
		if final != p.url && multiEmployerIndex(final) {
			continue
		}
		requestHost := base.Hostname()
		base, err = url.Parse(final)
		if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Hostname() == "" || base.User != nil {
			continue
		}
		if p.depth > 0 && !strings.EqualFold(base.Hostname(), requestHost) {
			noteHost(base.Hostname())
		}
		// Keep the observed vendor URL, including any data-residency host.
		if s, b, c, found := atsboard.Recognize(final); found {
			return s, b, c, nil, true
		}
		for _, raw := range atsdetect.AbsURLRe.FindAllString(body, -1) {
			if s, b, c, found := atsboard.Recognize(raw); found {
				return s, b, c, nil, true
			}
		}
		if s, b, c, found := resolveContent(body, final); found {
			return s, b, c, nil, true
		}
		root, err := html.Parse(strings.NewReader(body))
		if err != nil {
			continue
		}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if ok {
				return
			}
			if n.Type == html.ElementNode && n.Data == "a" {
				href := sources.Attr(n, "href")
				lower := strings.ToLower(href)
				career := false
				for _, word := range []string{"career", "job", "vacan", "work-for", "join-us", "karriere", "stellen", "werken"} {
					career = career || strings.Contains(lower, word)
				}
				if u, err := base.Parse(href); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil {
					u.Fragment = ""
					if s, b, c, found := atsboard.Recognize(u.String()); found {
						source, board, canonical, ok = s, b, c, true
						return
					}
					if !strings.EqualFold(u.Hostname(), base.Hostname()) &&
						(career || strings.Contains(strings.ToLower(u.Hostname()+u.Path), "apply")) {
						noteHost(u.Hostname())
					}
					if raw := u.String(); career && p.depth < 2 && !seen[raw] && len(queue) < 8 {
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
		if ok {
			return source, board, canonical, nil, true
		}
	}
	return "", "", "", careerHosts, false
}
