package sources

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// hibob adapts the careers module of HiBob ("Bob"), the HR platform. A board is the tenant
// slug, which is also its careers subdomain (<board>.careers.hibob.com). One keyless call
// returns the tenant's whole job board with descriptions inline, so there is no per-posting
// detail fetch.
//
// The call needs a Referer naming that tenant's own careers page — no cookie, no token, no
// key, just that header; without it the API answers 401. It is the only thing standing between
// this adapter and an empty board, which is why a test asserts it.
type hibob struct {
	http HeaderJSONGetter
}

const (
	hibobJobAdURL   = "https://%s.careers.hibob.com/api/job-ad"
	hibobJobsURL    = "https://%s.careers.hibob.com/jobs"
	hibobPostingURL = "https://%s.careers.hibob.com/jobs/%s"
)

// NewHiBob builds the HiBob adapter over the given HTTP client.
func NewHiBob(c HeaderJSONGetter) Source { return hibob{http: c} }

func (hibob) Provider() string { return "hibob" }

// hibobJobAd is one posting from the job-ad response. HiBob splits the ad's body across four
// HTML fields rather than carrying one description.
type hibobJobAd struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Site             string `json:"site"`
	Country          string `json:"country"`
	Description      string `json:"description"`
	Responsibilities string `json:"responsibilities"`
	Requirements     string `json:"requirements"`
	Benefits         string `json:"benefits"`
	PublishedAt      string `json:"publishedAt"`
	WorkspaceType    string `json:"workspaceType"`
}

func (h hibob) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var resp struct {
		JobAdDetails []hibobJobAd `json:"jobAdDetails"`
	}
	url := fmt.Sprintf(hibobJobAdURL, e.Board)
	headers := map[string]string{"Referer": fmt.Sprintf(hibobJobsURL, e.Board)}
	if err := h.http.GetJSONWithHeaders(ctx, url, headers, &resp); err != nil {
		return nil, fmt.Errorf("hibob: list board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.JobAdDetails))
	for _, ad := range resp.JobAdDetails {
		if ad.ID == "" {
			continue // no native id → no dedup key; every id-less posting would collide
		}
		jobs = append(jobs, h.job(e, ad))
	}
	return jobs, nil
}

// job maps one job ad to a Job.
func (hibob) job(e CompanyEntry, ad hibobJobAd) Job {
	mode := workplaceTypeMode(ad.WorkspaceType)

	return Job{
		ExternalID:  ad.ID,
		URL:         fmt.Sprintf(hibobPostingURL, e.Board, ad.ID),
		Title:       strings.TrimSpace(ad.Title),
		Company:     e.Company,
		Location:    joinNonEmpty(ad.Site, ad.Country),
		Description: hibobDescription(ad),
		Remote:      mode == "remote",
		PostedAt:    parseRFC3339(ad.PublishedAt),
		WorkMode:    mode,
	}
}

// hibobDescription assembles the posting's four body fields into one description. All four are
// the job ad — the requirements section in particular is where the skills live, so dropping it
// would starve skill tagging and enrichment.
//
// The sections are kept as separate blocks rather than concatenated: sanitizeHTML strips tags,
// and adjacent blocks whose text ran together would glue the last word of one section to the
// first of the next (see role_fingerprint, which hashes the visible text).
//
// HiBob also carries a sectionLabels object for tenants that rename the section headings. Every
// tenant seen so far leaves it empty, so the headings are the platform's own defaults; the seam
// to honour custom labels is this function.
func hibobDescription(ad hibobJobAd) string {
	sections := []struct{ heading, body string }{
		{"", ad.Description},
		{"Responsibilities", ad.Responsibilities},
		{"Requirements", ad.Requirements},
		{"Benefits", ad.Benefits},
	}

	var b strings.Builder
	for _, s := range sections {
		if strings.TrimSpace(s.body) == "" {
			continue
		}
		if s.heading != "" {
			b.WriteString("<h3>" + s.heading + "</h3>")
		}
		b.WriteString(s.body)
	}
	return sanitizeHTML(html.UnescapeString(b.String()))
}
