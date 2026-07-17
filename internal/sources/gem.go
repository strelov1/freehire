package sources

import (
	"context"
	"fmt"
)

// gemGraphQLURL is Gem's public (unauthenticated) GraphQL endpoint.
const (
	gemGraphQLURL = "https://jobs.gem.com/api/public/graphql"
)

// The list operation carries no description, so each posting's body comes from its own
// detail request. boardId is the board's vanity path (e.g. "go-cadre"), per the schema's
// jobBoardExternal(vanityUrlPath: $boardId).
const (
	gemListQuery = `query JobBoardList($boardId: String!) {
  oatsExternalJobPostings(boardId: $boardId) {
    jobPostings {
      extId
      title
      locations { city isoCountry isRemote }
      job { locationType }
    }
  }
}`

	gemDetailQuery = `query ExternalJobPostingQuery($boardId: String!, $extId: String!) {
  oatsExternalJobPosting(boardId: $boardId, extId: $extId) {
    descriptionHtml
    firstPublishedTsSec
  }
}`
)

// gem adapts Gem's public job-board GraphQL API. Its list endpoint carries no description,
// so it fetches each posting's detail (bounded-concurrency) to assemble the body, like the
// SmartRecruiters and Rippling adapters.
type gem struct {
	http JSONPoster
}

// NewGem builds the Gem adapter over the given HTTP client.
func NewGem(c JSONPoster) Source { return gem{http: c} }

func (gem) Provider() string { return "gem" }

// gemRequest is a GraphQL request body. Both operations share this shape; the variables
// carry boardId (and, for detail, extId).
type gemRequest struct {
	OperationName string         `json:"operationName"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
}

// gemError is one entry of a GraphQL response's top-level errors[]. The transport returns
// 200 even on a GraphQL-level failure (schema drift, server error), so the adapter checks
// errors[] explicitly rather than treating a null data payload as an empty board.
type gemError struct {
	Message string `json:"message"`
}

// gemPosting is one item from the JobBoardList response (no description here).
type gemPosting struct {
	ExtID     string `json:"extId"`
	Title     string `json:"title"`
	Locations []struct {
		City       string `json:"city"`
		IsoCountry string `json:"isoCountry"`
		IsRemote   bool   `json:"isRemote"`
	} `json:"locations"`
	Job struct {
		LocationType string `json:"locationType"`
	} `json:"job"`
}

func (g gem) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var resp struct {
		Errors []gemError `json:"errors"`
		Data   struct {
			Postings struct {
				JobPostings []gemPosting `json:"jobPostings"`
			} `json:"oatsExternalJobPostings"`
		} `json:"data"`
	}
	req := gemRequest{
		OperationName: "JobBoardList",
		Query:         gemListQuery,
		Variables:     map[string]any{"boardId": e.Board},
	}
	if err := g.http.PostJSON(ctx, gemGraphQLURL, req, &resp); err != nil {
		return nil, fmt.Errorf("gem: list board %s: %w", e.Board, err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("gem: list board %s: %s", e.Board, resp.Errors[0].Message)
	}

	// Each posting's description comes from its own detail request, fanned out under a
	// bounded worker pool.
	return fetchDetails(resp.Data.Postings.JobPostings, defaultDetailWorkers, func(p gemPosting) (Job, bool) {
		return g.detail(ctx, e, p)
	}), nil
}

// detail fetches one posting's description and maps it to a Job, returning ok=false when
// the detail request fails so the caller can skip just that posting.
func (g gem) detail(ctx context.Context, e CompanyEntry, p gemPosting) (Job, bool) {
	var resp struct {
		Errors []gemError `json:"errors"`
		Data   struct {
			Posting struct {
				DescriptionHTML string `json:"descriptionHtml"`
				// JSON numbers are untyped; firstPublishedTsSec is observed as integer
				// seconds but a float must still parse rather than drop the posting.
				FirstPublishedTsSec float64 `json:"firstPublishedTsSec"`
			} `json:"oatsExternalJobPosting"`
		} `json:"data"`
	}
	req := gemRequest{
		OperationName: "ExternalJobPostingQuery",
		Query:         gemDetailQuery,
		Variables:     map[string]any{"boardId": e.Board, "extId": p.ExtID},
	}
	if err := g.http.PostJSON(ctx, gemGraphQLURL, req, &resp); err != nil {
		return Job{}, false
	}
	if len(resp.Errors) > 0 {
		return Job{}, false
	}

	var city, country string
	var remote bool
	if len(p.Locations) > 0 {
		loc := p.Locations[0]
		city, country, remote = loc.City, loc.IsoCountry, loc.IsRemote
	}

	return Job{
		ExternalID:  p.ExtID,
		URL:         fmt.Sprintf("https://jobs.gem.com/%s/%s", e.Board, p.ExtID),
		Title:       p.Title,
		Company:     e.Company,
		Location:    joinNonEmpty(city, country),
		Description: sanitizeHTML(resp.Data.Posting.DescriptionHTML),
		Remote:      remote || p.Job.LocationType == "REMOTE",
		WorkMode:    workplaceTypeMode(p.Job.LocationType),
		PostedAt:    parseEpochSeconds(int64(resp.Data.Posting.FirstPublishedTsSec)),
	}, true
}
