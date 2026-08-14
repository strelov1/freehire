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

// jsonGetter fetches and decodes a URL's JSON body. Matches sources.JSONGetter
// so the production sources.Client satisfies it.
type jsonGetter interface {
	GetJSON(ctx context.Context, url string, v any) error
}

// httpGetter is everything NewRegistry's resolvers need from the shared HTTP
// client — one param type so a single sources.Client satisfies both the
// title-scrape resolvers (textGetter) and join's API resolver (jsonGetter).
type httpGetter interface {
	textGetter
	jsonGetter
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

// NewRegistry wires the per-ATS resolvers over the shared HTTP getter. Only ATSes whose
// board is derivable from a job URL are here (see BoardFromURL): the board is the host
// label (Pinpoint) or first path segment (Lever/Ashby).
//
// Each ATS titles its careers page differently, and the extractor is picked per source —
// this stopped being "the same parser, different host template" the moment it was checked
// against real boards: Pinpoint alone uses the lead-in-or-"Careers"-suffix shape
// ExtractTitleName was built and validated for (PR #825); Lever's storefront titles the
// page with the bare company name (jobs.lever.co/binance -> "Binance"); Ashby's titles
// itself "{Name} Jobs" (jobs.ashbyhq.com/airgarage -> "AirGarage Jobs"). These two shapes
// match cmd/harvest-boards/prober.go's storefrontEmployer, which independently discovered
// them for the same platforms.
//
// Greenhouse is intentionally absent — its job URLs are the company's own vanity careers
// domain (e.g. a16z.com/about/jobs), so no board can be recovered from the URL; resolving
// it needs a board-from-source-file lookup, a separate seam.
//
// BambooHR is intentionally absent too: its careers page is a client-rendered SPA whose
// static, unrendered <title> is just the platform's own boilerplate ("BambooHR"), never
// the tenant's name — cmd/harvest-boards' bamboohrProber reached the same conclusion and
// falls back to the slug rather than trying. A resolver wired here would always return ""
// while looking like it was attempting something; leaving it out is the honest answer.
//
// join is the odd one out: its board (used by Board, not BoardFromURL — see board.go) is
// the company's numeric join.com id, and join's own public API resolves that id straight
// to a display name (no title-scrape needed): GET
// https://join.com/api/public/companies/{id} -> {"name": "..."}. Confirmed live against
// id 175014 -> "Goodweek".
func NewRegistry(http httpGetter) Registry {
	return Registry{
		"pinpoint": newTitleResolver(http, "https://%s.pinpointhq.com", ExtractTitleName),
		"lever":    newTitleResolver(http, "https://jobs.lever.co/%s", ExtractBareTitle),
		"join":     newJoinResolver(http),
		"ashby": newTitleResolver(http, "https://jobs.ashbyhq.com/%s", func(title string) string {
			return ExtractSuffixName(title, " Jobs")
		}),
	}
}

var titleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

type titleResolver struct {
	http    textGetter
	tmpl    string                    // host/careers URL template with a single %s for the board
	extract func(title string) string // this ATS's careers-page <title> shape
}

func newTitleResolver(http textGetter, tmpl string, extract func(string) string) *titleResolver {
	return &titleResolver{http: http, tmpl: tmpl, extract: extract}
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
	return r.extract(m[1]), nil
}

const joinCompanyURL = "https://join.com/api/public/companies/%s"

// joinCompanyResp is the subset of join.com's public company-profile response
// this package needs. board is the numeric join company id.
type joinCompanyResp struct {
	Name string `json:"name"`
}

type joinResolver struct{ http jsonGetter }

func newJoinResolver(http jsonGetter) *joinResolver { return &joinResolver{http: http} }

func (r *joinResolver) Name(ctx context.Context, board string) (string, error) {
	var resp joinCompanyResp
	if err := r.http.GetJSON(ctx, fmt.Sprintf(joinCompanyURL, board), &resp); err != nil {
		return "", err
	}
	return resp.Name, nil
}
