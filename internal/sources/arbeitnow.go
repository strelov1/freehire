package sources

import (
	"context"
	"fmt"
)

// arbeitnow adapts arbeitnow.com, a (mostly German-market) job board. Like the other
// aggregators it is boardless (one public API, no per-tenant board) yet lists many
// employers, so it stays in the source facet and takes each posting's company from the
// feed. The job-board API carries every posting's body inline and paginates via a "next"
// link, so there is no per-posting detail request.
type arbeitnow struct {
	http JSONGetter
}

const (
	arbeitnowListURL = "https://www.arbeitnow.com/api/job-board-api"
	// arbeitnowMaxPages bounds pagination so a feed that never stops handing out a "next"
	// link cannot loop forever.
	arbeitnowMaxPages = 100
)

// NewArbeitnow builds the arbeitnow adapter over the given HTTP client.
func NewArbeitnow(c JSONGetter) Source { return arbeitnow{http: c} }

func (arbeitnow) Provider() string { return "arbeitnow" }

// arbeitnow needs no board id (one API), so its config carries no board.
func (arbeitnow) boardless() {}

// arbeitnow aggregates postings from many companies, so it stays in the source facet.
func (arbeitnow) aggregator() {}

// arbeitnowPosting is one posting from the job-board API, body inline (no detail call).
type arbeitnowPosting struct {
	Slug        string `json:"slug"`
	CompanyName string `json:"company_name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Remote      bool   `json:"remote"`
	URL         string `json:"url"`
	Location    string `json:"location"`
	CreatedAt   int64  `json:"created_at"`
}

func (a arbeitnow) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	url := arbeitnowListURL
	for page := 0; page < arbeitnowMaxPages && url != ""; page++ {
		var resp struct {
			Data  []arbeitnowPosting `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := a.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("arbeitnow: page %d: %w", page, err)
		}
		for _, p := range resp.Data {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if len(resp.Data) == 0 {
			break
		}
		url = resp.Links.Next
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// native id, which would collide on the dedup key, or no company, which would break the
// slug). The structured remote flag sets the work mode; the location text is a fallback.
func (p arbeitnowPosting) toJob() (Job, bool) {
	if p.Slug == "" || p.CompanyName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  p.Slug,
		URL:         p.URL,
		Title:       p.Title,
		Company:     p.CompanyName,
		Location:    p.Location,
		Description: sanitizeHTML(p.Description),
		Remote:      p.Remote || isRemote(p.Location),
		WorkMode:    workModeFromRemote(p.Remote),
		PostedAt:    parseEpochSeconds(p.CreatedAt),
	}, true
}
