package sources

import (
	"context"
	"fmt"
	"strings"
)

// jobscore adapts JobScore career sites through the public per-company job feed. JobScore is a
// US ATS whose feed is keyless: the board is the company shortcode from its careers URL
// (careers.jobscore.com/careers/<slug>), and one feed.json call returns every published posting
// fully populated — structured city/state/country, a Yes/No remote string, job_type,
// experience_level, opened_date, a human careers-page detail_url, and the HTML description — so no
// per-posting detail fetch is needed.
//
// JobScore asks integrations to poll a feed at most once per hour (it may disable access on more
// frequent polling); the hourly ingest timer stays within that.
type jobscore struct {
	http JSONGetter
}

// jobscoreLimit caps a single feed read. JobScore feeds are slow-changing per-company lists that
// fit one response; a generous limit avoids truncating a large board without inviting abuse.
const jobscoreLimit = 1000

// NewJobscore builds the JobScore adapter over the given HTTP client.
func NewJobscore(c JSONGetter) Source { return jobscore{http: c} }

func (jobscore) Provider() string { return "jobscore" }

// jobscoreFeed is the feed.json envelope: company identity plus the posting list.
type jobscoreFeed struct {
	CompanyName string        `json:"company_name"`
	Jobs        []jobscoreJob `json:"jobs"`
}

type jobscoreJob struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	DetailURL       string `json:"detail_url"`
	Description     string `json:"description"`
	City            string `json:"city"`
	State           string `json:"state"`
	Country         string `json:"country"`
	Location        string `json:"location"`
	Remote          string `json:"remote"`
	JobType         string `json:"job_type"`
	ExperienceLevel string `json:"experience_level"`
	OpenedDate      string `json:"opened_date"`
}

func (s jobscore) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if e.Board == "" {
		return nil, fmt.Errorf("jobscore: empty board")
	}
	url := fmt.Sprintf("https://careers.jobscore.com/jobs/%s/feed.json?sort=date&limit=%d", e.Board, jobscoreLimit)

	var feed jobscoreFeed
	if err := s.http.GetJSON(ctx, url, &feed); err != nil {
		return nil, fmt.Errorf("jobscore: board %s: %w", e.Board, err)
	}

	company := e.Company
	if company == "" {
		company = feed.CompanyName
	}

	jobs := make([]Job, 0, len(feed.Jobs))
	for _, j := range feed.Jobs {
		if job, ok := toJobscoreJob(j, company); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJobscoreJob maps a feed posting to a Job, returning ok=false for a posting missing an id
// (which would collide on the dedup key).
func toJobscoreJob(j jobscoreJob, company string) (Job, bool) {
	if strings.TrimSpace(j.ID) == "" {
		return Job{}, false
	}
	mode := jobscoreWorkMode(j.Remote)
	return Job{
		ExternalID:     j.ID,
		URL:            j.DetailURL,
		Title:          j.Title,
		Company:        company,
		Location:       jobscoreLocation(j),
		Description:    sanitizeHTML(j.Description),
		Remote:         mode == "remote",
		WorkMode:       mode,
		PostedAt:       parseRFC3339OrDate(j.OpenedDate),
		Seniority:      jobscoreSeniority(j.ExperienceLevel),
		EmploymentType: jobscoreEmploymentType(j.JobType),
	}, true
}

// jobscoreLocation renders "City, State" from the structured fields, falling back to the country,
// then the free-text location string.
func jobscoreLocation(j jobscoreJob) string {
	var parts []string
	for _, p := range []string{j.City, j.State} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	if c := strings.TrimSpace(j.Country); c != "" {
		return c
	}
	return strings.TrimSpace(j.Location)
}

// jobscoreWorkMode maps JobScore's remote string onto the work-mode vocabulary. The field is
// free text prefixed Yes/No with an "all/some of the time" qualifier, e.g. "No | Must be able to
// work onsite all of the time" or "Yes | Can work remotely some of the time".
func jobscoreWorkMode(remote string) string {
	r := strings.ToLower(strings.TrimSpace(remote))
	switch {
	case strings.HasPrefix(r, "yes"):
		if strings.Contains(r, "some") {
			return "hybrid"
		}
		return "remote"
	case strings.HasPrefix(r, "no"):
		return "onsite"
	default:
		return ""
	}
}

// jobscoreSeniority maps JobScore's experience_level label onto the freehire seniority vocabulary
// via keyword containment. Only the individual-contributor levels map; management/executive labels
// (Manager, Executive) are left "" so the title dictionary decides rather than forcing a
// people-manager into an IC-seniority bucket. Structured signal only, never a guess.
func jobscoreSeniority(level string) string {
	l := strings.ToLower(level)
	switch {
	case strings.Contains(l, "student") || strings.Contains(l, "intern"):
		return "intern"
	case strings.Contains(l, "entry") || strings.Contains(l, "associate"):
		return "junior"
	case strings.Contains(l, "mid"):
		return "middle"
	case strings.Contains(l, "experienced") || strings.Contains(l, "senior"):
		return "senior"
	default:
		return ""
	}
}

// jobscoreEmploymentType maps JobScore's job_type label (Full Time, Part Time, Contractor,
// Temporary, Intern, Seasonal) onto the freehire vocabulary via keyword containment. An
// unrecognized value maps to "".
func jobscoreEmploymentType(t string) string {
	s := strings.ToLower(t)
	switch {
	case strings.Contains(s, "intern"):
		return "internship"
	case strings.Contains(s, "part"):
		return "part_time"
	case strings.Contains(s, "contract") || strings.Contains(s, "temp") || strings.Contains(s, "seasonal"):
		return "contract"
	case strings.Contains(s, "full"):
		return "full_time"
	default:
		return ""
	}
}
