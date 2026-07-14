package sources

import (
	"context"
	"fmt"
	"strings"
)

// smartRecruitersBaseURL is the SmartRecruiters public postings API root.
const smartRecruitersBaseURL = "https://api.smartrecruiters.com/v1/companies"

// smartRecruitersPageLimit is the postings page size.
const (
	smartRecruitersPageLimit = 100
)

// smartRecruiters adapts the SmartRecruiters public API. Unlike the other adapters its
// list endpoint carries no description, so it paginates the postings and fetches each
// posting's detail (bounded-concurrency) to assemble the description.
type smartRecruiters struct {
	http JSONGetter
}

// NewSmartRecruiters builds the SmartRecruiters adapter over the given HTTP client.
func NewSmartRecruiters(c JSONGetter) Source { return smartRecruiters{http: c} }

func (smartRecruiters) Provider() string { return "smartrecruiters" }

// smartRecruitersSeniorityMap maps the SmartRecruiters experienceLevel enum onto
// freehire's seniority vocabulary. Values not in the map (notably "not_applicable",
// the plurality, plus any unknown/empty id) yield "" so the Job carries no structured
// seniority and the pipeline's title dictionary decides — structured signal only,
// never a guess (the sources.Job.Seniority contract).
var smartRecruitersSeniorityMap = map[string]string{
	"internship":       "intern",
	"entry_level":      "junior",
	"associate":        "middle",
	"mid_senior_level": "senior",
	"director":         "lead",
	"executive":        "c_level",
}

// smartRecruitersSeniority maps an experienceLevel id to a freehire seniority, or ""
// when the level is unset/ambiguous/unknown.
func smartRecruitersSeniority(experienceLevelID string) string {
	return smartRecruitersSeniorityMap[experienceLevelID]
}

// srSection is one HTML section of a posting's job ad (the description lives here).
type srSection struct {
	Text string `json:"text"`
}

// smartRecruitersPosting is one item from the postings list (no description here).
type smartRecruitersPosting struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ReleasedDate string `json:"releasedDate"`
	Location     struct {
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
		Remote  bool   `json:"remote"`
	} `json:"location"`
}

func (s smartRecruiters) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.listPostings(ctx, e.Board)
	if err != nil {
		return nil, err
	}

	// Each posting's description comes from its own detail request, fanned out under a
	// bounded worker pool.
	return fetchDetails(postings, defaultDetailWorkers, func(p smartRecruitersPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// listPostings pages through the board's postings, stopping when a page is empty or all
// postings reported by totalFound have been collected.
func (s smartRecruiters) listPostings(ctx context.Context, board string) ([]smartRecruitersPosting, error) {
	var postings []smartRecruitersPosting
	for offset := 0; ; {
		url := fmt.Sprintf("%s/%s/postings?limit=%d&offset=%d", smartRecruitersBaseURL, board, smartRecruitersPageLimit, offset)
		var page struct {
			TotalFound int                      `json:"totalFound"`
			Content    []smartRecruitersPosting `json:"content"`
		}
		if err := s.http.GetJSON(ctx, url, &page); err != nil {
			return nil, fmt.Errorf("smartrecruiters: list board %s: %w", board, err)
		}
		if len(page.Content) == 0 {
			break
		}
		postings = append(postings, page.Content...)
		offset += len(page.Content)
		if offset >= page.TotalFound {
			break
		}
	}
	return postings, nil
}

// detail fetches one posting's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that posting.
func (s smartRecruiters) detail(ctx context.Context, e CompanyEntry, p smartRecruitersPosting) (Job, bool) {
	url := fmt.Sprintf("%s/%s/postings/%s", smartRecruitersBaseURL, e.Board, p.ID)

	var d struct {
		PostingURL      string `json:"postingUrl"`
		ExperienceLevel struct {
			ID string `json:"id"`
		} `json:"experienceLevel"`
		TypeOfEmployment struct {
			ID string `json:"id"`
		} `json:"typeOfEmployment"`
		JobAd struct {
			Sections struct {
				JobDescription        srSection `json:"jobDescription"`
				Qualifications        srSection `json:"qualifications"`
				AdditionalInformation srSection `json:"additionalInformation"`
			} `json:"sections"`
		} `json:"jobAd"`
	}
	if err := s.http.GetJSON(ctx, url, &d); err != nil {
		return Job{}, false
	}

	// companyDescription is intentionally excluded — it is boilerplate, not the role.
	sec := d.JobAd.Sections
	body := sec.JobDescription.Text + sec.Qualifications.Text + sec.AdditionalInformation.Text

	return Job{
		ExternalID:  p.ID,
		URL:         d.PostingURL,
		Title:       strings.TrimSpace(p.Name),
		Company:     e.Company,
		Location:    joinNonEmpty(p.Location.City, p.Location.Region, p.Location.Country),
		Description: sanitizeHTML(body),
		Remote:      p.Location.Remote,
		WorkMode:    workModeFromRemote(p.Location.Remote),
		PostedAt:    parseRFC3339(p.ReleasedDate),
		// experienceLevel is the employer's own structured seniority; the pipeline gives it
		// precedence over the title dictionary, so it fills the grade for title-silent roles.
		Seniority: smartRecruitersSeniority(d.ExperienceLevel.ID),
		// typeOfEmployment is the employer's structured employment type; preferred over the
		// free-text description parse.
		EmploymentType: smartRecruitersEmploymentType(d.TypeOfEmployment.ID),
	}, true
}

// smartRecruitersEmploymentType maps the SmartRecruiters typeOfEmployment enum onto the
// freehire employment-type vocabulary, returning "" for an unset/unknown id so the
// description parser decides — structured signal only, never a guess.
func smartRecruitersEmploymentType(typeOfEmploymentID string) string {
	switch typeOfEmploymentID {
	case "permanent", "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "contract", "temporary":
		return "contract"
	case "internship", "traineeship":
		return "internship"
	default:
		return ""
	}
}
