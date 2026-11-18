package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// careersgov adapts Careers@Gov (careers.gov.sg), the Singapore public service's job portal.
// Boardless and multi-agency: GovTech's own Open Government Products team publishes the whole
// live catalogue as one JSON file on GitHub, refreshed several times a day, so a crawl is a
// single request with every body inline. See providers/careersgov.md.
type careersgov struct {
	http JSONGetter
}

const (
	careersgovListURL = "https://raw.githubusercontent.com/opengovsg/careersgovsg-jobs-data/main/data/job-listings.json"
	careersgovJobURL  = "https://jobs.careers.gov.sg/jobs/hrp/%s/%s"
)

// NewCareersGov builds the Careers@Gov (Singapore) adapter over the given client.
func NewCareersGov(c JSONGetter) Source { return careersgov{http: c} }

func (careersgov) Provider() string { return "careersgov" }

func (careersgov) boardless() {}

func (careersgov) aggregator() {}

// careersgovPosting is one row of the file. Only `hrp` rows (the government HR platform) are
// decoded into it: the `greenhouse` and `workable` rows mirror ATS boards crawled under their
// own providers, and type the same keys differently ("1790844080000", "None").
type careersgovPosting struct {
	JobID               string `json:"jobId"`
	PostingNo           string `json:"postingNo"`
	JobTitle            string `json:"jobTitle"`
	Agency              string `json:"agency"`
	StartDate           int64  `json:"startDate"`
	ClosingDate         int64  `json:"closingDate"`
	EmploymentType      string `json:"employmentType"`
	WorkArrangement     string `json:"workArrangement"`
	ExperienceYearsMin  int    `json:"experienceYearsMin"`
	JobDescription      string `json:"jobDescription"`
	JobResponsibilities string `json:"jobResponsibilities"`
	JobRequirements     string `json:"jobRequirements"`
}

func (s careersgov) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var rows []json.RawMessage
	if err := s.http.GetJSON(ctx, careersgovListURL, &rows); err != nil {
		return nil, fmt.Errorf("careersgov: %w", err)
	}
	var jobs []Job
	seen := make(map[string]bool)
	for _, raw := range rows {
		var head struct {
			Platform string `json:"platform"`
		}
		if json.Unmarshal(raw, &head) != nil || head.Platform != "hrp" {
			continue
		}
		var p careersgovPosting
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("careersgov: hrp row: %w", err)
		}
		if job, ok := p.toJob(); ok && !seen[job.ExternalID] {
			seen[job.ExternalID] = true
			jobs = append(jobs, job)
		}
	}
	// A file that parses but holds no hrp row is a moved schema, not an empty public service.
	if len(jobs) == 0 {
		return nil, fmt.Errorf("careersgov: no hrp postings in %d rows", len(rows))
	}
	return jobs, nil
}

// toJob maps a row to a Job; ok=false when it lacks the ids the public page is keyed by.
func (p careersgovPosting) toJob() (Job, bool) {
	if p.JobID == "" || p.PostingNo == "" || p.JobTitle == "" || p.Agency == "" {
		return Job{}, false
	}
	body := careersgovSection(p.JobDescription)
	if r := careersgovSection(p.JobResponsibilities); r != "" {
		body += "<h3>Responsibilities</h3>" + r
	}
	if r := careersgovSection(p.JobRequirements); r != "" {
		body += "<h3>Requirements</h3>" + r
	}
	job := Job{
		ExternalID:     p.JobID,
		URL:            fmt.Sprintf(careersgovJobURL, url.PathEscape(p.JobID), url.PathEscape(p.PostingNo)),
		Title:          p.JobTitle,
		Company:        p.Agency,
		Location:       "Singapore",
		Description:    sanitizeHTML(body),
		PostedAt:       parseEpochMillis(p.StartDate),
		EmploymentType: careersgovEmploymentType(p.EmploymentType, p.WorkArrangement),
	}
	// Not parseEpochMillis: it clamps a future time to now, and a deadline is in the future.
	if p.ClosingDate > 0 {
		deadline := time.UnixMilli(p.ClosingDate).UTC()
		job.ApplicationDeadline = &deadline
	}
	if p.ExperienceYearsMin > 0 {
		years := p.ExperienceYearsMin
		job.ExperienceYearsMin = &years
	}
	return job, true
}

// careersgovDangling matches the cut-off heading markup each section starts and ends with:
// close tags at the start ("</strong></p>"), open tags at the end ("<p><strong>").
var careersgovDangling = regexp.MustCompile(`^(\s*</[a-z0-9]+>)+|(<[a-z0-9]+>\s*)+$`)

// careersgovSection trims one body field to its own balanced markup. The sanitizer keeps
// stray tags as they are, so they would otherwise open the section in bold.
func careersgovSection(s string) string {
	return strings.TrimSpace(careersgovDangling.ReplaceAllString(strings.TrimSpace(s), ""))
}

// careersgovEmploymentType reads the contract kind first, since "Fixed Terms" and "Contract"
// rows are also marked Full-time; "Permanent" and "Casual" say nothing beyond the hours.
func careersgovEmploymentType(employment, arrangement string) string {
	switch employment {
	case "Internship", "Traineeship":
		return "internship"
	case "Contract", "Fixed Terms":
		return "contract"
	}
	switch arrangement {
	case "Full-time":
		return "full_time"
	case "Part-time":
		return "part_time"
	}
	return ""
}
