package sources

import (
	"context"
	"fmt"
	"regexp"
)

// workingnomads adapts workingnomads.com, a remote-jobs aggregator. Boardless (one public
// feed, no per-tenant board) and multi-company, so it stays in the source facet and takes
// each posting's company from the feed. The /api/exposed_jobs/ endpoint returns a single
// flat array with every posting's body inline (no detail call); the response is the recent
// window, so coverage is that window. The site is remote-only, so every job is remote.
type workingnomads struct {
	http JSONGetter
}

const workingNomadsListURL = "https://www.workingnomads.com/api/exposed_jobs/"

// workingNomadsIDRE pulls the posting id from the trailing path segment of the job URL
// (e.g. https://www.workingnomads.com/job/go/1663269/), because the feed carries no id
// field. A URL without a numeric tail yields no id and the posting is dropped.
var workingNomadsIDRE = regexp.MustCompile(`/(\d+)/?$`)

// NewWorkingNomads builds the Working Nomads adapter over the given HTTP client.
func NewWorkingNomads(c JSONGetter) Source { return workingnomads{http: c} }

func (workingnomads) Provider() string { return "workingnomads" }

func (workingnomads) boardless() {}

func (workingnomads) aggregator() {}

// workingNomadsPosting is one posting from the /api/exposed_jobs/ feed, body inline.
type workingNomadsPosting struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CompanyName string `json:"company_name"`
	Location    string `json:"location"`
	PubDate     string `json:"pub_date"`
}

func (s workingnomads) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var postings []workingNomadsPosting
	if err := s.http.GetJSON(ctx, workingNomadsListURL, &postings); err != nil {
		return nil, fmt.Errorf("workingnomads: list: %w", err)
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no id
// derivable from the URL, or no company which would break the slug). Working Nomads lists
// only remote jobs.
func (p workingNomadsPosting) toJob() (Job, bool) {
	m := workingNomadsIDRE.FindStringSubmatch(p.URL)
	if m == nil || p.CompanyName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  m[1],
		URL:         p.URL,
		Title:       p.Title,
		Company:     p.CompanyName,
		Location:    p.Location,
		Description: sanitizeHTML(p.Description),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseRFC3339(p.PubDate),
	}, true
}
