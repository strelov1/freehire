package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/skilltag"
)

// teamex adapts teamex.io, a global talent marketplace. Its keyless API exposes one paged
// POST list endpoint (/api/jts/global/filter) whose items already carry the full posting —
// title, HTML descriptions, countries, required skills — so one paged crawl assembles every
// Job with no per-posting detail request. The marketplace lists many employers but usually
// anonymizes them (isAnonymous), so a posting's company falls back to the marketplace label;
// its boardless config entry's company is only a validation placeholder.
type teamex struct {
	http JSONPoster
}

const (
	teamexListURL = "https://api.teamex.io/api/jts/global/filter?pageSize=%d&pageIndex=%d"
	// teamexJobURL is the public posting page, addressed by the job's sequenceId.
	teamexJobURL   = "https://teamex.io/job/%d"
	teamexPageSize = 50
	// teamexMaxPages bounds pagination so a wrong or missing paging.totalCount cannot loop.
	teamexMaxPages = 100
	// teamexAnonymousCompany labels a posting whose employer the marketplace hides.
	teamexAnonymousCompany = "TeamEx"
)

// NewTeamex builds the teamex adapter over the given HTTP client.
func NewTeamex(c JSONPoster) Source { return teamex{http: c} }

func (teamex) Provider() string { return "teamex" }

// teamex is a marketplace with one global feed, so its config entries carry no board.
func (teamex) boardless() {}

// teamex aggregates postings from many employers, so it stays in the source facet.
func (teamex) aggregator() {}

// teamexListResponse is one page of the filter endpoint: Data.Paging.TotalCount is the
// catalogue size used to stop pagination; Data.Data is the page of postings.
type teamexListResponse struct {
	Data struct {
		Paging struct {
			TotalCount int `json:"totalCount"`
		} `json:"paging"`
		Data []teamexJob `json:"data"`
	} `json:"data"`
}

// teamexJob is one posting. The list item is fully populated (descriptions, countries,
// skills), so the detail endpoint is never called. Company nests the employer's own name
// when the posting is not anonymous; IsActive gates closed postings out of the crawl.
type teamexJob struct {
	SequenceID           int                   `json:"sequenceId"`
	Title                string                `json:"title"`
	IsActive             bool                  `json:"isActive"`
	IsAnonymous          bool                  `json:"isAnonymous"`
	PublishedDate        string                `json:"publishedDate"`
	YearsOfExperienceMin *int                  `json:"yearsOfExperienceMin"`
	Descriptions         []teamexDescription   `json:"descriptions"`
	Countries            []teamexCountry       `json:"countries"`
	RequiredSkills       []teamexRequiredSkill `json:"requiredSkills"`
	Company              struct {
		Name string `json:"name"`
	} `json:"company"`
}

// teamexDescription is one titled HTML section of a posting's body.
type teamexDescription struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// teamexCountry is one of a posting's countries; only the display Name is used for the
// location string (the pipeline's location dictionary derives codes/regions from it).
type teamexCountry struct {
	Name string `json:"name"`
}

// teamexRequiredSkill wraps a required skill; only the display name is used, canonicalized
// through the skilltag dictionary.
type teamexRequiredSkill struct {
	Skill struct {
		DisplayName string `json:"displayName"`
	} `json:"skill"`
}

func (s teamex) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= teamexMaxPages; page++ {
		var resp teamexListResponse
		url := fmt.Sprintf(teamexListURL, teamexPageSize, page)
		if err := s.http.PostJSON(ctx, url, struct{}{}, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("teamex: list page %d: %w", page, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		if len(resp.Data.Data) == 0 {
			break
		}
		for _, it := range resp.Data.Data {
			if !it.IsActive {
				continue // closed postings are not crawled
			}
			jobs = append(jobs, teamexToJob(it))
		}
		if resp.Data.Paging.TotalCount > 0 && page*teamexPageSize >= resp.Data.Paging.TotalCount {
			break
		}
	}
	return jobs, nil
}

// teamexToJob maps a posting to a Job. The employer is the posting's own company, but an
// anonymized posting (isAnonymous) hides it behind the marketplace label even when the payload
// still carries the name — the name must not leak; an unnamed posting also falls back. The
// location is the joined country names; skills are canonicalized through skilltag.
func teamexToJob(it teamexJob) Job {
	company := teamexAnonymousCompany
	if !it.IsAnonymous {
		company = firstNonEmpty(it.Company.Name, teamexAnonymousCompany)
	}
	return Job{
		ExternalID:         strconv.Itoa(it.SequenceID),
		URL:                fmt.Sprintf(teamexJobURL, it.SequenceID),
		Title:              it.Title,
		Company:            company,
		Location:           teamexLocation(it.Countries),
		Description:        sanitizeHTML(teamexDescriptionHTML(it.Descriptions)),
		PostedAt:           parseRFC3339(it.PublishedDate),
		Skills:             teamexSkills(it.RequiredSkills),
		ExperienceYearsMin: it.YearsOfExperienceMin,
	}
}

// teamexDescriptionHTML composes the posting body from its titled sections, wrapping each
// plain-text title as a heading before its HTML content (the title is prose, so a bare "<"
// would otherwise make the sanitizer drop the tail).
func teamexDescriptionHTML(sections []teamexDescription) string {
	var b strings.Builder
	for _, s := range sections {
		if strings.TrimSpace(s.Title) != "" {
			b.WriteString("<h3>")
			b.WriteString(html.EscapeString(s.Title))
			b.WriteString("</h3>")
		}
		b.WriteString(s.Content)
	}
	return b.String()
}

// teamexLocation joins the posting's country names in order, skipping blanks via the shared
// joinNonEmpty helper.
func teamexLocation(countries []teamexCountry) string {
	names := make([]string, 0, len(countries))
	for _, c := range countries {
		names = append(names, c.Name)
	}
	return joinNonEmpty(names...)
}

// teamexSkills canonicalizes the posting's required skills through the skilltag dictionary,
// keeping only resolved technologies. The names are joined into one blob so skilltag.Parse
// applies the same matching it uses on a description.
func teamexSkills(skills []teamexRequiredSkill) []string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Skill.DisplayName)
	}
	return skilltag.Parse(strings.Join(names, " "))
}
