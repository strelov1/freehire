package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
)

// functionalWorks adapts functional.works-hub.com, a niche board for functional-programming and
// adjacent engineering roles. Like the other aggregators it is boardless (one public GraphQL
// endpoint, no per-tenant board) yet lists many employers, so it stays in the source facet and
// takes each posting's company from the feed. One query returns the whole backlog (the API has
// no offset/page argument, so the page_size just needs to exceed the catalogue size) with the
// body inline — no per-posting detail request.
type functionalWorks struct {
	http JSONPoster
}

const (
	functionalWorksURL = "https://functional.works-hub.com/api/graphql"
	// functionalWorksQuery pulls the whole board in one request: page_size is set well above the
	// live catalogue (~900 postings) because the endpoint exposes no pagination argument.
	functionalWorksQuery = `{ jobs(page_size:2000, vertical:"functional", published:true) { ` +
		`title slug remote firstPublished descriptionHtml company { name } ` +
		`location { city country } tags { label } } }`
	// functionalWorksJobURL is the public job page, keyed by slug — the outbound apply link.
	functionalWorksJobURL = "https://functional.works-hub.com/jobs/%s"
)

// NewFunctionalWorks builds the functionalworks adapter over the given HTTP client.
func NewFunctionalWorks(c JSONPoster) Source { return functionalWorks{http: c} }

func (functionalWorks) Provider() string { return "functionalworks" }

// functionalworks needs no board id (one GraphQL endpoint), so its config carries no board.
func (functionalWorks) boardless() {}

// functionalworks aggregates postings from many companies, so it stays in the source facet.
func (functionalWorks) aggregator() {}

// functionalWorksPosting is one posting from the GraphQL feed, body inline (no detail call).
type functionalWorksPosting struct {
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	Remote         bool   `json:"remote"`
	FirstPublished string `json:"firstPublished"`
	DescriptionML  string `json:"descriptionHtml"`
	Company        struct {
		Name string `json:"name"`
	} `json:"company"`
	Location struct {
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"location"`
	Tags []struct {
		Label string `json:"label"`
	} `json:"tags"`
}

func (s functionalWorks) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	body := struct {
		Query string `json:"query"`
	}{Query: functionalWorksQuery}
	var resp struct {
		Data struct {
			Jobs []functionalWorksPosting `json:"jobs"`
		} `json:"data"`
	}
	if err := s.http.PostJSON(ctx, functionalWorksURL, body, &resp); err != nil {
		return nil, fmt.Errorf("functionalworks: query: %w", err)
	}
	jobs := make([]Job, 0, len(resp.Data.Jobs))
	for _, p := range resp.Data.Jobs {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps a posting to a Job, returning ok=false for an unusable posting (no slug to build
// the URL and key on, or no company, which would break the slug). The structured remote flag
// sets the work mode; the location text is a fallback. tags canonicalize into skills.
func (p functionalWorksPosting) toJob() (Job, bool) {
	if p.Slug == "" || p.Company.Name == "" {
		return Job{}, false
	}
	labels := make([]string, 0, len(p.Tags))
	for _, t := range p.Tags {
		labels = append(labels, t.Label)
	}
	return Job{
		ExternalID:  p.Slug,
		URL:         fmt.Sprintf(functionalWorksJobURL, p.Slug),
		Title:       p.Title,
		Company:     p.Company.Name,
		Location:    p.location(),
		Description: sanitizeHTML(p.DescriptionML),
		Remote:      p.Remote || isRemote(p.location()),
		WorkMode:    workModeFromRemote(p.Remote),
		Skills:      skilltag.Parse(strings.Join(labels, " ")),
		PostedAt:    parseRFC3339(p.FirstPublished),
	}, true
}

// location formats the posting's location as "City, Country", degrading to whichever part is
// present (the API leaves city null for country-only postings).
func (p functionalWorksPosting) location() string {
	switch {
	case p.Location.City != "" && p.Location.Country != "":
		return p.Location.City + ", " + p.Location.Country
	case p.Location.City != "":
		return p.Location.City
	default:
		return p.Location.Country
	}
}
