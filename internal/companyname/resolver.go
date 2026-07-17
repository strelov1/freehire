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

// jsonGetter decodes a URL's JSON body. Matches sources.JSONGetter.
type jsonGetter interface {
	GetJSON(ctx context.Context, url string, v any) error
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

// NewRegistry wires the per-ATS resolvers over the shared HTTP getters.
func NewRegistry(text textGetter, jsonG jsonGetter) Registry {
	return Registry{
		// Careers-page <title> ATSes: same parser, different host template.
		"pinpoint": newTitleResolver(text, "https://%s.pinpointhq.com"),
		"bamboohr": newTitleResolver(text, "https://%s.bamboohr.com/careers"),
		"lever":    newTitleResolver(text, "https://jobs.lever.co/%s"),
		"ashby":    newTitleResolver(text, "https://jobs.ashbyhq.com/%s"),
		// API-field ATSes.
		"greenhouse": newGreenhouseResolver(jsonG),
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

type greenhouseResolver struct{ http jsonGetter }

func newGreenhouseResolver(http jsonGetter) *greenhouseResolver {
	return &greenhouseResolver{http: http}
}

func (r *greenhouseResolver) Name(ctx context.Context, board string) (string, error) {
	var resp struct {
		Name string `json:"name"`
	}
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/", board)
	if err := r.http.GetJSON(ctx, url, &resp); err != nil {
		return "", err
	}
	return resp.Name, nil
}
