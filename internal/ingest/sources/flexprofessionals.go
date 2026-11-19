package sources

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// flexprofessionals adapts FlexProfessionals (flexprofessionalsllc.com), a Washington DC / Boston
// staffing firm placing part-time and flexible professionals. Its site runs the Matador Jobs
// WordPress plugin, whose public REST type lists every live posting with the body inline, so a
// crawl is one request. Single-company and boardless; the clients are anonymous, so every
// posting carries the agency as its company. See providers/flexprofessionals.md.
type flexprofessionals struct {
	http JSONGetter
}

const flexprofessionalsListURL = "https://www.flexprofessionalsllc.com/wp-json/wp/v2/matador-job-listings?per_page=100"

// NewFlexProfessionals builds the FlexProfessionals adapter over the given client.
func NewFlexProfessionals(c JSONGetter) Source { return flexprofessionals{http: c} }

func (flexprofessionals) Provider() string { return "flexprofessionals" }

func (flexprofessionals) Boardless() {}

type flexprofessionalsPost struct {
	ID    int    `json:"id"`
	Date  string `json:"date_gmt"`
	Link  string `json:"link"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
}

func (s flexprofessionals) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	// shortcut: one page of 100, the firm lists ~35 postings; page via X-WP-TotalPages if it outgrows that.
	var posts []flexprofessionalsPost
	if err := s.http.GetJSON(ctx, flexprofessionalsListURL, &posts); err != nil {
		return nil, fmt.Errorf("flexprofessionals: %w", err)
	}
	jobs := make([]Job, 0, len(posts))
	for _, p := range posts {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

var (
	// flexprofessionalsMetaList is the plugin's "Category / Location / Work Arrangement / Type"
	// list that opens every body; it is read for facets and dropped from the description.
	flexprofessionalsMetaList = regexp.MustCompile(`(?s)^\s*<ul class="matador-job-meta.*?</ul>`)
	flexprofessionalsField    = regexp.MustCompile(`(?s)matador-job-meta-label">([^<]*):</span>\s*<span class="matador-job-meta-value">(.*?)</span>\s*</li>`)
	flexprofessionalsTag      = regexp.MustCompile(`<[^>]*>`)
)

func (p flexprofessionalsPost) toJob() (Job, bool) {
	title := strings.TrimSpace(html.UnescapeString(p.Title.Rendered))
	if p.ID == 0 || p.Link == "" || title == "" {
		return Job{}, false
	}
	body := p.Content.Rendered
	fields := map[string]string{}
	if meta := flexprofessionalsMetaList.FindString(body); meta != "" {
		for _, m := range flexprofessionalsField.FindAllStringSubmatch(meta, -1) {
			v := html.UnescapeString(flexprofessionalsTag.ReplaceAllString(m[2], ""))
			fields[m[1]] = strings.Join(strings.Fields(v), " ")
		}
		body = strings.TrimPrefix(body, meta)
	}
	// The site writes "Washington , District Of Columbia"; fields already collapsed the spaces.
	location := strings.ReplaceAll(fields["Location"], " ,", ",")
	job := Job{
		ExternalID:     strconv.Itoa(p.ID),
		URL:            p.Link,
		Title:          title,
		Company:        "FlexProfessionals",
		Location:       location,
		Description:    sanitizeHTML(body),
		PostedAt:       parseRFC3339(p.Date + "Z"),
		EmploymentType: flexprofessionalsEmploymentType(fields["Type"]),
	}
	switch strings.ToLower(fields["Work Arrangement"]) {
	case "remote":
		job.Remote, job.WorkMode = true, "remote"
	case "hybrid":
		job.WorkMode = "hybrid"
	case "on-site":
		job.WorkMode = "onsite"
	}
	return job, true
}

// flexprofessionalsEmploymentType maps the plugin's Type term; "Full-time/Flex" and "Project-FT"
// are full-time hours, "Part-time" is the firm's core offer.
func flexprofessionalsEmploymentType(t string) string {
	switch t = strings.ToLower(t); {
	case strings.HasPrefix(t, "part-time"):
		return "part_time"
	case strings.HasPrefix(t, "full-time"), strings.HasPrefix(t, "project-ft"):
		return "full_time"
	}
	return ""
}
