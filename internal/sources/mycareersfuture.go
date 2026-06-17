package sources

import (
	"context"
	"fmt"
	"log"
)

// mycareersfuture adapts mycareersfuture.gov.sg, the Singapore government job portal.
// Boardless (one public API, no per-tenant board) and multi-company, so it stays in the
// source facet and takes each posting's company from the API. The v2 jobs endpoint carries
// every posting's body inline and paginates by page, so there is no per-posting detail
// request. The portal is large (~85k postings, all sectors), so a full crawl is sizeable —
// the cron schedule governs how often it runs.
type mycareersfuture struct {
	http JSONGetter
}

const (
	mcfListURL  = "https://api.mycareersfuture.gov.sg/v2/jobs?limit=%d&page=%d"
	mcfJobURL   = "https://www.mycareersfuture.gov.sg/job/%s"
	mcfPageSize = 100
	// mcfMaxPages is a runaway guard well above the portal's real size.
	mcfMaxPages = 1500
)

// NewMyCareersFuture builds the MyCareersFuture (Singapore) adapter over the given client.
func NewMyCareersFuture(c JSONGetter) Source { return mycareersfuture{http: c} }

func (mycareersfuture) Provider() string { return "mycareersfuture" }

func (mycareersfuture) boardless() {}

func (mycareersfuture) aggregator() {}

// mcfPosting is one job from the v2 list, body inline (no detail call).
type mcfPosting struct {
	UUID          string `json:"uuid"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	PostedCompany struct {
		Name string `json:"name"`
	} `json:"postedCompany"`
	Address struct {
		IsOverseas      bool   `json:"isOverseas"`
		OverseasCountry string `json:"overseasCountry"`
	} `json:"address"`
	Metadata struct {
		NewPostingDate string `json:"newPostingDate"`
	} `json:"metadata"`
}

func (s mycareersfuture) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 0; page < mcfMaxPages; page++ {
		var resp struct {
			Results []mcfPosting `json:"results"`
		}
		url := fmt.Sprintf(mcfListURL, mcfPageSize, page)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("mycareersfuture: page %d: %w", page, err)
			}
			// The portal rate-limits (429) a long paginated sweep partway through. Stop with
			// the jobs gathered so far rather than discarding the whole crawl — the next run
			// re-fetches idempotently. Erroring only on the very first page keeps a genuinely
			// dead board a failure.
			log.Printf("mycareersfuture: stopping at page %d (%v); keeping %d jobs gathered so far", page, err, len(jobs))
			break
		}
		if len(resp.Results) == 0 {
			break
		}
		for _, p := range resp.Results {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if len(resp.Results) < mcfPageSize {
			break
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// uuid, which would collide on the dedup key, or no company which would break the slug).
// The public job page is keyed by uuid (verified to resolve).
func (p mcfPosting) toJob() (Job, bool) {
	if p.UUID == "" || p.PostedCompany.Name == "" {
		return Job{}, false
	}
	location := "Singapore"
	if p.Address.IsOverseas && p.Address.OverseasCountry != "" {
		location = p.Address.OverseasCountry
	}
	return Job{
		ExternalID:  p.UUID,
		URL:         fmt.Sprintf(mcfJobURL, p.UUID),
		Title:       p.Title,
		Company:     p.PostedCompany.Name,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      isRemote(p.Title),
		PostedAt:    parseDate(p.Metadata.NewPostingDate),
	}, true
}
