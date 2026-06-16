package sources

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// jobstash adapts JobStash (middleware.jobstash.xyz), a Web3 job aggregator. Unlike the
// per-company ATS adapters it is boardless (one API, no per-tenant board) yet aggregates
// many employers, so it is also an aggregator (stays in the source facet) and takes each
// posting's company from its own organization.name. The list endpoint carries every
// posting's body inline, so there is no per-posting detail request; it paginates by page.
type jobstash struct {
	http JSONGetter
}

const (
	jobstashListURL = "https://middleware.jobstash.xyz/jobs/list?page=%d&limit=%d"
	// jobstashDetailURL is the public posting page, used as the link for a protected
	// posting whose url is null (the /jobs/list feed omits the gated apply link).
	jobstashDetailURL = "https://jobstash.xyz/jobs/%s/details"
	jobstashPageSize  = 200
	// jobstashMaxPages bounds pagination so a server that mis-reports total (never
	// emptying its data) cannot loop forever; the empty-page check ends it sooner.
	jobstashMaxPages = 200
)

// NewJobStash builds the JobStash adapter over the given HTTP client.
func NewJobStash(c JSONGetter) Source { return jobstash{http: c} }

func (jobstash) Provider() string { return "jobstash" }

// jobstash needs no board id (one API), so its config carries no board.
func (jobstash) boardless() {}

// jobstash aggregates postings from many companies, so it stays in the source facet.
func (jobstash) aggregator() {}

// jobstashPosting is one posting with its body inline (no detail call). url is already the
// right target: the downstream ATS link for a public posting, the JobStash page for a
// protected one.
type jobstashPosting struct {
	ShortUUID        string   `json:"shortUUID"`
	Title            string   `json:"title"`
	URL              string   `json:"url"`
	Location         string   `json:"location"`
	LocationType     string   `json:"locationType"`
	Timestamp        int64    `json:"timestamp"`
	Description      string   `json:"description"`
	Responsibilities []string `json:"responsibilities"`
	Requirements     []string `json:"requirements"`
	Benefits         []string `json:"benefits"`
	Organization     struct {
		Name string `json:"name"`
	} `json:"organization"`
}

func (j jobstash) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	postings, err := j.list(ctx)
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// list walks the job-list endpoint page by page (page size jobstashPageSize) until the
// reported total is reached or a page returns no postings, whichever comes first. Each page
// carries postings with their body inline.
func (j jobstash) list(ctx context.Context) ([]jobstashPosting, error) {
	var all []jobstashPosting
	for page := 1; page <= jobstashMaxPages; page++ {
		var resp struct {
			Total int               `json:"total"`
			Data  []jobstashPosting `json:"data"`
		}
		url := fmt.Sprintf(jobstashListURL, page, jobstashPageSize)
		if err := j.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("jobstash: page %d: %w", page, err)
		}
		all = append(all, resp.Data...)
		if len(resp.Data) == 0 || len(all) >= resp.Total {
			break
		}
	}
	return all, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// native id, which would collide on the dedup key, or no company, which would break the
// slug) so the caller drops just that one. Company comes from the posting's own
// organization; the structured locationType sets the work mode, with the remote flag
// falling back to the location text.
func (p jobstashPosting) toJob() (Job, bool) {
	if p.ShortUUID == "" || p.Organization.Name == "" {
		return Job{}, false
	}
	workMode := workplaceTypeMode(p.LocationType)
	return Job{
		ExternalID:  p.ShortUUID,
		URL:         p.url(),
		Title:       p.Title,
		Company:     p.Organization.Name,
		Location:    p.Location,
		Description: p.description(),
		WorkMode:    workMode,
		Remote:      workMode == "remote" || isRemote(p.Location),
		PostedAt:    parseEpochMillis(p.Timestamp),
	}, true
}

// url is the posting's link: the inline url when present (the downstream ATS link for a
// public posting), else the JobStash detail page built from shortUUID (a protected
// posting's apply link is gated, so the feed leaves url null — we link to its JobStash
// page instead of dropping the vacancy).
func (p jobstashPosting) url() string {
	if p.URL != "" {
		return p.URL
	}
	return fmt.Sprintf(jobstashDetailURL, p.ShortUUID)
}

// description assembles the posting's prose and its responsibilities/requirements/benefits
// lists into sanitized HTML. Each text value is HTML-escaped before wrapping, so a stray
// angle bracket in source text cannot inject markup; a section with no items is omitted.
func (p jobstashPosting) description() string {
	var b strings.Builder
	if p.Description != "" {
		b.WriteString("<p>" + html.EscapeString(p.Description) + "</p>")
	}
	writeJobstashSection(&b, "Responsibilities", p.Responsibilities)
	writeJobstashSection(&b, "Requirements", p.Requirements)
	writeJobstashSection(&b, "Benefits", p.Benefits)
	return sanitizeHTML(b.String())
}

// writeJobstashSection appends an <h3> heading and a bulleted list for the items, or nothing
// when items is empty.
func writeJobstashSection(b *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString("<h3>" + heading + "</h3><ul>")
	for _, it := range items {
		b.WriteString("<li>" + html.EscapeString(it) + "</li>")
	}
	b.WriteString("</ul>")
}
