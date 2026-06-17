package sources

import (
	"context"
	"fmt"
)

// ashbyGraphQL adapts Ashby boards whose public Posting API (see ashby.go) is disabled, so
// the only machine-readable source is the same embed GraphQL endpoint Ashby's own hosted
// careers widget calls. It is a separate provider from ashby on purpose: the 2400+ boards on
// the stable Posting API stay untouched, and only the handful that 404 there (e.g. Chainlink
// Labs, Toggl) opt into this path. The list query returns brief postings without a
// description, so each posting takes a detail query — the same list+detail shape as
// SmartRecruiters.
type ashbyGraphQL struct {
	http JSONPoster
}

const (
	ashbyGQLURL      = "https://jobs.ashbyhq.com/api/non-user-graphql"
	ashbyGQLJobURL   = "https://jobs.ashbyhq.com/" // + <board>/<id>
	ashbyGQLListOp   = "ApiJobBoardWithTeams"
	ashbyGQLDetailOp = "ApiJobPosting"

	ashbyGQLListQuery = `query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) { ` +
		`jobBoard: jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) ` +
		`{ jobPostings { id title locationName employmentType } } }`

	ashbyGQLDetailQuery = `query ApiJobPosting($organizationHostedJobsPageName: String!, $jobPostingId: String!) { ` +
		`jobPosting(organizationHostedJobsPageName: $organizationHostedJobsPageName, jobPostingId: $jobPostingId) ` +
		`{ id title descriptionHtml locationName } }`
)

// NewAshbyGraphQL builds the Ashby embed-GraphQL adapter over the given HTTP client.
func NewAshbyGraphQL(c JSONPoster) Source { return ashbyGraphQL{http: c} }

func (ashbyGraphQL) Provider() string { return "ashbygraphql" }

// ashbyGQLRequest is the GraphQL POST body the embed endpoint expects.
type ashbyGQLRequest struct {
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
	Query         string         `json:"query"`
}

// ashbyGQLBrief is one posting from the list query (no description).
type ashbyGQLBrief struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	LocationName string `json:"locationName"`
}

func (a ashbyGraphQL) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	body := ashbyGQLRequest{
		OperationName: ashbyGQLListOp,
		Variables:     map[string]any{"organizationHostedJobsPageName": e.Board},
		Query:         ashbyGQLListQuery,
	}
	var resp struct {
		Data struct {
			JobBoard struct {
				JobPostings []ashbyGQLBrief `json:"jobPostings"`
			} `json:"jobBoard"`
		} `json:"data"`
	}
	if err := a.http.PostJSON(ctx, ashbyGQLURL+"?op="+ashbyGQLListOp, body, &resp); err != nil {
		return nil, fmt.Errorf("ashbygraphql: list board %s: %w", e.Board, err)
	}

	postings := resp.Data.JobBoard.JobPostings
	return fetchDetails(postings, defaultDetailWorkers, func(p ashbyGQLBrief) (Job, bool) {
		return a.detail(ctx, e, p)
	}), nil
}

// detail fetches one posting's descriptionHtml and maps it to a Job, returning ok=false when
// the detail request fails so the caller skips just that posting.
func (a ashbyGraphQL) detail(ctx context.Context, e CompanyEntry, p ashbyGQLBrief) (Job, bool) {
	body := ashbyGQLRequest{
		OperationName: ashbyGQLDetailOp,
		Variables: map[string]any{
			"organizationHostedJobsPageName": e.Board,
			"jobPostingId":                   p.ID,
		},
		Query: ashbyGQLDetailQuery,
	}
	var resp struct {
		Data struct {
			JobPosting struct {
				DescriptionHTML string `json:"descriptionHtml"`
			} `json:"jobPosting"`
		} `json:"data"`
	}
	if err := a.http.PostJSON(ctx, ashbyGQLURL+"?op="+ashbyGQLDetailOp, body, &resp); err != nil {
		return Job{}, false
	}

	return Job{
		ExternalID:  p.ID,
		URL:         ashbyGQLJobURL + e.Board + "/" + p.ID,
		Title:       p.Title,
		Company:     e.Company,
		Location:    p.LocationName,
		Description: sanitizeHTML(resp.Data.JobPosting.DescriptionHTML),
		Remote:      isRemote(p.LocationName),
	}, true
}
