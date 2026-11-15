package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// garena adapts Garena's careers site (careers.garena.com), a single-company source with no
// per-tenant board id (boardless). The site is a Nuxt app over a small keyless JSON API: one
// POST lists the whole catalogue, and a GET per posting returns its body. The listing's own
// `description` is only the summary paragraph, often empty, while the responsibilities and requirements live in the detail alone, so every posting is
// fetched.
type garena struct {
	http garenaHTTP
}

// garenaHTTP is the transport garena needs: a JSON POST for the listing and a JSON GET for
// each posting.
type garenaHTTP interface {
	JSONPoster
	JSONGetter
}

const (
	// garenaOffice is the site's office path segment. The listing ignores it (every office
	// code, a made-up one included, returns the same list), but the posting pages and the
	// site's own links use "global", so the URLs do too.
	garenaOffice    = "global"
	garenaListURL   = "https://careers.garena.com/api/job/list?office=" + garenaOffice
	garenaDetailURL = "https://careers.garena.com/api/job/detail/%s?office=" + garenaOffice
	garenaJobURL    = "https://careers.garena.com/" + garenaOffice + "/careers/%s"
)

// NewGarena builds the Garena careers adapter over the given HTTP client.
func NewGarena(c garenaHTTP) Source { return garena{http: c} }

func (garena) Provider() string { return "garena" }

// garena is single-company, so its config entry carries no board.
func (garena) boardless() {}

// fullBoardListing: the listing is ONE unpaginated request returning the whole catalogue, the
// ashby shape — there is no later page that could fail and leave a truncated walk, and a
// response without its `jobs` array fails the crawl rather than reading as an empty board.
func (garena) fullBoardListing() {}

type garenaTags struct {
	Location []string `json:"location"`
	Category []string `json:"job_category"`
	Type     []string `json:"job_type"`
}

type garenaListItem struct {
	ID    string     `json:"id"`
	Title string     `json:"title"`
	Tags  garenaTags `json:"tags"`
}

// garenaDetail is the detail payload's `job` object. A posting that no longer exists answers
// 200 with `{"job": {}}`, so an empty id is the platform's own "gone".
type garenaDetail struct {
	ID              string `json:"id"`
	Description     string `json:"description"`
	JobDescription  string `json:"job_description"`
	JobRequirements string `json:"job_requirements"`
}

func (s garena) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var list struct {
		Jobs *[]garenaListItem `json:"jobs"`
	}
	// The site posts the remaining filter fields as the body; an empty object lists everything.
	if err := s.http.PostJSON(ctx, garenaListURL, map[string]any{}, &list); err != nil {
		return nil, fmt.Errorf("garena: list: %w", err)
	}
	if list.Jobs == nil {
		return nil, fmt.Errorf("garena: list: response carries no jobs array")
	}
	items := *list.Jobs
	jobs := fetchDetails(items, defaultDetailWorkers, func(it garenaListItem) (Job, bool) {
		return s.detail(ctx, e, it)
	})
	unreadable := 0
	for _, j := range jobs {
		if j.Unreadable {
			unreadable++
		}
	}
	// A crawl that listed postings and read none of them is a failure, not an empty catalogue
	// (AGENTS.md, "A crawl that reads nothing of what it listed must fail").
	if len(items) > 0 && unreadable == len(items) {
		return nil, fmt.Errorf("garena: listed %d postings and read none of them", len(items))
	}
	return jobs, nil
}

// detail fetches one posting's body. A 404/410 or an empty `job` object is the platform saying
// the posting is gone (dropped); any other failure is an Unreadable marker, since the detail is
// the only place the body lives.
func (s garena) detail(ctx context.Context, e CompanyEntry, it garenaListItem) (Job, bool) {
	id, title := strings.TrimSpace(it.ID), strings.Join(strings.Fields(it.Title), " ")
	if id == "" || title == "" {
		return Job{}, false
	}
	jobURL := fmt.Sprintf(garenaJobURL, url.PathEscape(id))
	var resp struct {
		Job garenaDetail `json:"job"`
	}
	if err := s.http.GetJSON(ctx, fmt.Sprintf(garenaDetailURL, url.PathEscape(id)), &resp); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, jobURL, e.Company), true
		}
		return Job{}, false
	}
	if resp.Job.ID == "" {
		return Job{}, false
	}
	desc := garenaDescription(resp.Job)
	if desc == "" {
		return unreadableDetail(id, jobURL, e.Company), true
	}
	job := Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       title,
		Company:     e.Company,
		Description: desc,
	}
	var places []string
	for _, l := range it.Tags.Location {
		if l = strings.TrimSpace(l); strings.EqualFold(l, "Remote") {
			job.WorkMode, job.Remote = "remote", true
		} else if l != "" {
			places = append(places, l)
		}
	}
	job.Location = strings.Join(places, "; ")
	if len(it.Tags.Type) == 1 {
		switch it.Tags.Type[0] {
		case "Internship":
			job.EmploymentType, job.Seniority = "internship", "intern"
		case "Entry Level":
			job.Seniority = "junior"
		case "Experienced (Team Lead)":
			job.Seniority = "lead"
		}
	}
	return job, true
}

// garenaDescription assembles the body the posting page shows: the summary paragraph, then the
// "Job Description" and "Job Requirements" sections (both HTML lists), each under the page's own
// heading. An empty section is left out.
func garenaDescription(d garenaDetail) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(d.Description))
	for _, sec := range []struct{ heading, body string }{
		{"Job Description", d.JobDescription},
		{"Job Requirements", d.JobRequirements},
	} {
		if body := strings.TrimSpace(sec.body); body != "" {
			b.WriteString("<h3>" + sec.heading + "</h3>" + body)
		}
	}
	return strings.TrimSpace(sanitizeHTML(b.String()))
}
