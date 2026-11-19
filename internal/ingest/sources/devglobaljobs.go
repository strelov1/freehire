package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
)

// devglobaljobs adapts devglobaljobs.com, a global jobs aggregator. Boardless (one public
// REST API, no per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed.
//
// The /api/v1/jobs endpoint returns recent postings across all categories. Full descriptions are
// fetched via /api/v1/jobs/<id> only for unseen postings that are not ATS-covered (CoverageGated).
// Failed detail requests yield unreadableDetail markers rather than falling back to excerpts.
type devglobaljobs struct {
	http JSONGetter
}

const (
	devglobaljobsListURL   = "https://devglobaljobs.com/api/v1/jobs"
	devglobaljobsPageSize  = 100
	devglobaljobsMaxPages  = 100 // bounds pagination against runaway loops; offset is clamped at 10,000
	devglobaljobsDetailURL = "https://devglobaljobs.com/api/v1/jobs/%s"
	// devglobaljobsSweepGrace is wide because the API clamps offset pagination at 10,000, which
	// reaches only the newest day or two of the feed; older postings cannot be re-seen, and a
	// short grace would let the sweep close them while still active.
	devglobaljobsSweepGrace = 30 * 24 * time.Hour
)

// NewDevGlobalJobs builds the Dev Global Jobs adapter over the given HTTP client.
func NewDevGlobalJobs(c JSONGetter) Source { return devglobaljobs{http: c} }

func (devglobaljobs) Provider() string { return "devglobaljobs" }

func (devglobaljobs) Boardless() {}

func (devglobaljobs) Aggregator() {}

func (devglobaljobs) SweepGrace() time.Duration { return devglobaljobsSweepGrace }

type devglobaljobsPosting struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	Organization       string `json:"organization"`
	Location           string `json:"location"`
	Country            string `json:"country"`
	Category           string `json:"category"`
	SkillCategory      string `json:"skillCategory"`
	JobType            string `json:"jobType"`
	DescriptionExcerpt string `json:"descriptionExcerpt"`
	PostedAt           string `json:"postedAt"`
	URL                string `json:"url"`
}

type devglobaljobsDetail struct {
	Description string `json:"description"`
}

func (s devglobaljobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s devglobaljobs) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

func (s devglobaljobs) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	var postings []devglobaljobsPosting
	for page := 0; page < devglobaljobsMaxPages; page++ {
		offset := page * devglobaljobsPageSize
		listURL := fmt.Sprintf("%s?limit=%d&offset=%d",
			devglobaljobsListURL, devglobaljobsPageSize, offset)
		var resp struct {
			Meta struct {
				HasMore bool `json:"hasMore"`
			} `json:"meta"`
			Jobs []devglobaljobsPosting `json:"jobs"`
		}
		if err := s.http.GetJSON(ctx, listURL, &resp); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("devglobaljobs: list: %w", err)
			}
			log.Printf("devglobaljobs: page %d (offset %d): %v; keeping %d postings", page, offset, err, len(postings))
			break
		}
		postings = append(postings, resp.Jobs...)
		if len(resp.Jobs) < devglobaljobsPageSize || !resp.Meta.HasMore {
			break
		}
	}

	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(postings))
		for _, p := range postings {
			if p.Organization != "" {
				companies = append(companies, p.Organization)
			}
		}
		covered = gate.Covered(companies)
	}

	var toHydrate []devglobaljobsPosting
	var out []Job

	for _, p := range postings {
		if p.ID == 0 || p.Organization == "" || p.Title == "" {
			continue
		}
		idStr := strconv.FormatInt(p.ID, 10)
		url := p.URL
		if url == "" {
			url = fmt.Sprintf("https://devglobaljobs.com/jobs/detail/%s", idStr)
		}
		if seen(idStr) {
			out = append(out, Job{
				ExternalID:  idStr,
				URL:         url,
				Title:       p.Title,
				Company:     p.Organization,
				SeenRefresh: true,
			})
			continue
		}

		base := p.toJob(p.DescriptionExcerpt)
		if covered[p.Organization] {
			// Covered by first-party ATS or rejected by filter: yield without detail call.
			out = append(out, base)
			continue
		}

		toHydrate = append(toHydrate, p)
	}

	hydrated := fetchDetails(toHydrate, defaultDetailWorkers, func(p devglobaljobsPosting) (Job, bool) {
		return s.detail(ctx, p)
	})

	return append(out, hydrated...), nil
}

func (s devglobaljobs) detail(ctx context.Context, p devglobaljobsPosting) (Job, bool) {
	idStr := strconv.FormatInt(p.ID, 10)
	url := p.URL
	if url == "" {
		url = fmt.Sprintf("https://devglobaljobs.com/jobs/detail/%s", idStr)
	}
	var d devglobaljobsDetail
	if err := s.http.GetJSON(ctx, fmt.Sprintf(devglobaljobsDetailURL, idStr), &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(idStr, url, p.Organization), true
		}
		return Job{}, false
	}
	if strings.TrimSpace(d.Description) == "" {
		return unreadableDetail(idStr, url, p.Organization), true
	}
	return p.toJob(d.Description), true
}

func (p devglobaljobsPosting) toJob(desc string) Job {
	idStr := strconv.FormatInt(p.ID, 10)
	url := p.URL
	if url == "" {
		url = fmt.Sprintf("https://devglobaljobs.com/jobs/detail/%s", idStr)
	}
	job := Job{
		ExternalID:  idStr,
		URL:         url,
		Title:       p.Title,
		Company:     p.Organization,
		Location:    strings.TrimSpace(p.Location),
		Description: sanitizeHTML(html.UnescapeString(desc)),
		PostedAt:    parseRFC3339(p.PostedAt),
	}

	if p.Country != "" {
		if c := location.NormalizeCountry(p.Country); c != "" {
			job.Countries = []string{c}
		}
	}

	locLower := strings.ToLower(p.Location)
	if strings.Contains(locLower, "remote") {
		job.Remote = true
		job.WorkMode = "remote"
	} else if strings.Contains(locLower, "hybrid") {
		job.WorkMode = "hybrid"
	}

	switch strings.ToLower(p.JobType) {
	case "full-time", "full_time":
		job.EmploymentType = "full_time"
	case "part-time", "part_time":
		job.EmploymentType = "part_time"
	case "contract":
		job.EmploymentType = "contract"
	case "internship":
		job.EmploymentType = "internship"
	}

	return job
}
