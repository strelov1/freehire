package companyname

import (
	"context"
	"fmt"
	"regexp"
)

// textGetter fetches a URL's raw body (careers-page HTML). Matches
// sources.TextGetter so the production sources.Client satisfies it.
type textGetter interface {
	GetText(ctx context.Context, url string) (string, error)
}

// Resolver resolves a raw display-name candidate for a board from an ATS's own
// source. It returns "" (not an error) when the source yields no usable name;
// an error is reserved for transport failures. The candidate is unvalidated —
// the caller gates it with Accept against the company's slug.
type Resolver interface {
	Name(ctx context.Context, board string) (string, error)
}

// Registry maps a source name to its resolver. Sources with no entry are left
// alone rather than guessed.
type Registry map[string]Resolver

// NewRegistry wires the per-ATS resolvers over the shared HTTP getter. Only ATSes
// whose board is derivable from a job URL are here (see BoardFromURL): the board
// is the host label (Pinpoint/BambooHR) or first path segment (Lever/Ashby).
// Greenhouse is intentionally absent — its job URLs are the company's own vanity
// careers domain (e.g. a16z.com/about/jobs), so no board can be recovered from
// the URL; resolving it needs a board-from-source-file lookup, a separate seam.
func NewRegistry(text textGetter) Registry {
	return Registry{
		// Careers-page <title> ATSes: same parser, different host template.
		"pinpoint": newTitleResolver(text, "https://%s.pinpointhq.com"),
		"bamboohr": newTitleResolver(text, "https://%s.bamboohr.com/careers"),
		"lever":    newTitleResolver(text, "https://jobs.lever.co/%s"),
		"ashby":    newTitleResolver(text, "https://jobs.ashbyhq.com/%s"),
	}
}

var titleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

type titleResolver struct {
	http textGetter
	tmpl string // host/careers URL template with a single %s for the board
}

func newTitleResolver(http textGetter, tmpl string) *titleResolver {
	return &titleResolver{http: http, tmpl: tmpl}
}

func (r *titleResolver) Name(ctx context.Context, board string) (string, error) {
	body, err := r.http.GetText(ctx, fmt.Sprintf(r.tmpl, board))
	if err != nil {
		return "", err
	}
	m := titleTag.FindStringSubmatch(body)
	if m == nil {
		return "", nil
	}
	return ExtractTitleName(m[1]), nil
}
