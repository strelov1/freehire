package sources

import (
	"context"
	"fmt"
)

// powertofly adapts powertofly.com, a remote/diversity-focused job board. Like the other
// aggregators it is boardless (one public API, no per-tenant board) yet lists many
// employers, so it stays in the source facet and takes each posting's company from the
// feed. The /api/v1/jobs endpoint carries every posting's body inline and paginates via a
// numeric meta.next_page, so there is no per-posting detail request. Applications are hosted
// on powertofly.com itself (no outbound ATS URL), so a posting's identity is its own numeric
// id — the dedup key namespaces cleanly by board without link resolution.
type powertofly struct {
	http JSONGetter
}

const (
	powertoflyListURL = "https://powertofly.com/api/v1/jobs/"
	// powertoflyMaxPages bounds pagination so a feed that never stops handing out a
	// next_page cannot loop forever (~192 pages at 50/page as of onboarding).
	powertoflyMaxPages = 400
)

// NewPowerToFly builds the PowerToFly adapter over the given HTTP client.
func NewPowerToFly(c JSONGetter) Source { return powertofly{http: c} }

func (powertofly) Provider() string { return "powertofly" }

// powertofly needs no board id (one API), so its config carries no board.
func (powertofly) boardless() {}

// powertofly aggregates postings from many companies, so it stays in the source facet.
func (powertofly) aggregator() {}

// powertoflyPlace is a structured country/state/city object; only its title is used.
type powertoflyPlace struct {
	Title string `json:"title"`
}

// powertoflyPosting is one posting from /api/v1/jobs, body inline (no detail call). The
// free-text "location" field is the work arrangement (Onsite/Hybrid/Remote), NOT geography —
// the place is carried structurally in city/state/country, with location_regions as a fallback.
type powertoflyPosting struct {
	ID             int64    `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Location       string   `json:"location"`
	LocationRegion []string `json:"location_regions"`
	EmploymentType string   `json:"employment_type"`
	Company        struct {
		Name string `json:"name"`
	} `json:"company"`
	Country powertoflyPlace `json:"country"`
	State   powertoflyPlace `json:"state"`
	City    powertoflyPlace `json:"city"`
}

func (s powertofly) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= powertoflyMaxPages; page++ {
		var resp struct {
			Data []powertoflyPosting `json:"data"`
			Meta struct {
				NextPage *int `json:"next_page"`
			} `json:"meta"`
		}
		url := fmt.Sprintf("%s?page=%d", powertoflyListURL, page)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("powertofly: page %d: %w", page, err)
		}
		for _, p := range resp.Data {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		// next_page is null on the last page; an empty page also terminates defensively.
		if resp.Meta.NextPage == nil || len(resp.Data) == 0 {
			break
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// native id, which would collide on the dedup key, or no company, which would break the
// slug). The structured "location" work-arrangement field sets the work mode; geography is
// built from the city/state/country titles, falling back to the region tags. required_skills
// is deliberately left off Job.Skills — its titles are free text, not freehire's canonical
// skill vocabulary, so the pipeline's skilltag dictionary derives skills from the description.
func (p powertoflyPosting) toJob() (Job, bool) {
	if p.ID == 0 || p.Company.Name == "" {
		return Job{}, false
	}
	location := joinNonEmpty(p.City.Title, p.State.Title, p.Country.Title)
	if location == "" {
		location = joinNonEmpty(p.LocationRegion...)
	}
	mode := workplaceTypeMode(p.Location)
	return Job{
		ExternalID:     fmt.Sprintf("%d", p.ID),
		URL:            fmt.Sprintf("https://powertofly.com/jobs/detail/%d", p.ID),
		Title:          p.Title,
		Company:        p.Company.Name,
		Location:       location,
		Description:    sanitizeHTML(p.Description),
		Remote:         mode == "remote" || isRemote(p.Location),
		WorkMode:       mode,
		EmploymentType: powertoflyEmploymentType(p.EmploymentType),
	}, true
}

// powertoflyEmploymentType maps PowerToFly's employment_type text onto the freehire
// vocabulary, returning "" for an unknown/absent value so the description parser decides.
func powertoflyEmploymentType(t string) string {
	switch t {
	case "Full Time":
		return "full_time"
	case "Part Time":
		return "part_time"
	case "Contract", "Per Project":
		return "contract"
	case "Internship":
		return "internship"
	default:
		return ""
	}
}
