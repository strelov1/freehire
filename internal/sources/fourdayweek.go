package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
)

// fourDayWeek adapts 4dayweek.io, a curated board of roles offering a shortened work week.
// Like the other aggregators it is boardless (one public API, no per-tenant board) yet lists
// many employers, so it stays in the source facet and takes each posting's company from the
// feed. The list API paginates and carries the platform's structured facets inline
// (work_arrangement, level, category, stack), but no description and no apply link, so the
// canonical URL is synthesized from the slug and the body is left for downstream enrichment.
type fourDayWeek struct {
	http JSONGetter
}

const (
	// fourDayWeekListURL pages the public listing; limit=100 is the largest page the API honours.
	fourDayWeekListURL = "https://4dayweek.io/api/jobs?page=%d&limit=100"
	// fourDayWeekJobURL is the public job page, keyed by slug — the outbound apply link.
	fourDayWeekJobURL = "https://4dayweek.io/job/%s"
	// fourDayWeekMaxPages bounds pagination so a feed that never reports has_more=false cannot
	// loop forever. The catalogue is ~21k postings at 100/page, so this leaves ample headroom.
	fourDayWeekMaxPages = 400
)

// NewFourDayWeek builds the 4dayweek adapter over the given HTTP client.
func NewFourDayWeek(c JSONGetter) Source { return fourDayWeek{http: c} }

func (fourDayWeek) Provider() string { return "4dayweek" }

// 4dayweek needs no board id (one API), so its config carries no board.
func (fourDayWeek) boardless() {}

// 4dayweek aggregates postings from many companies, so it stays in the source facet.
func (fourDayWeek) aggregator() {}

// fourDayWeekLocation is one location entry; the primary (or first) supplies the display string.
type fourDayWeekLocation struct {
	City      string `json:"city"`
	Country   string `json:"country"`
	IsPrimary bool   `json:"is_primary"`
}

// fourDayWeekPosting is one posting from the list API (facets inline, no body, no apply link).
type fourDayWeekPosting struct {
	ID              string                `json:"id"`
	Slug            string                `json:"slug"`
	Title           string                `json:"title"`
	CompanyName     string                `json:"company_name"`
	WorkArrangement string                `json:"work_arrangement"`
	Level           string                `json:"level"`
	Category        string                `json:"category"`
	Posted          int64                 `json:"posted"`
	Locations       []fourDayWeekLocation `json:"locations"`
	Stack           []struct {
		Name string `json:"name"`
	} `json:"stack"`
}

func (s fourDayWeek) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= fourDayWeekMaxPages; page++ {
		var resp struct {
			Jobs    []fourDayWeekPosting `json:"jobs"`
			HasMore bool                 `json:"has_more"`
		}
		if err := s.http.GetJSON(ctx, fmt.Sprintf(fourDayWeekListURL, page), &resp); err != nil {
			return nil, fmt.Errorf("4dayweek: page %d: %w", page, err)
		}
		for _, p := range resp.Jobs {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if len(resp.Jobs) == 0 || !resp.HasMore {
			break
		}
	}
	return jobs, nil
}

// toJob maps a posting to a Job, returning ok=false for an unusable posting (no native id,
// which would collide on the dedup key, no slug to build the URL, or no company, which would
// break the slug). The platform's structured facets map straight into freehire's vocabularies;
// values it does not state (or that have no clean equivalent) are left empty for the pipeline's
// dictionaries to decide.
func (p fourDayWeekPosting) toJob() (Job, bool) {
	if p.ID == "" || p.Slug == "" || p.CompanyName == "" {
		return Job{}, false
	}
	names := make([]string, 0, len(p.Stack))
	for _, s := range p.Stack {
		names = append(names, s.Name)
	}
	return Job{
		ExternalID: p.ID,
		URL:        fmt.Sprintf(fourDayWeekJobURL, p.Slug),
		Title:      p.Title,
		Company:    p.CompanyName,
		Location:   p.location(),
		Remote:     p.WorkArrangement == "remote",
		WorkMode:   fourDayWeekWorkMode(p.WorkArrangement),
		Seniority:  fourDayWeekSeniority(p.Level),
		Category:   fourDayWeekCategory(p.Category),
		Skills:     skilltag.Parse(strings.Join(names, " ")),
		PostedAt:   parseEpochSeconds(p.Posted),
	}, true
}

// location formats the primary (or first) location as "City, Country", degrading to whichever
// part is present, and empty when the posting carries no location (e.g. some remote roles).
func (p fourDayWeekPosting) location() string {
	if len(p.Locations) == 0 {
		return ""
	}
	loc := p.Locations[0]
	for _, l := range p.Locations {
		if l.IsPrimary {
			loc = l
			break
		}
	}
	switch {
	case loc.City != "" && loc.Country != "":
		return loc.City + ", " + loc.Country
	case loc.City != "":
		return loc.City
	default:
		return loc.Country
	}
}

// fourDayWeekWorkMode passes through the platform's structured work_arrangement when it is one
// of freehire's work modes, else empty so the location heuristic decides.
func fourDayWeekWorkMode(wa string) string {
	switch wa {
	case "remote", "hybrid", "onsite":
		return wa
	default:
		return ""
	}
}

// fourDayWeekSeniority maps the platform's level onto enrich.SeniorityValues; an unknown or
// absent level yields empty so the title dictionary decides.
func fourDayWeekSeniority(level string) string {
	switch level {
	case "entry":
		return "junior"
	case "mid":
		return "middle"
	case "senior":
		return "senior"
	case "lead":
		return "lead"
	case "executive":
		return "c_level"
	default:
		return ""
	}
}

// fourDayWeekCategory maps the platform's category onto enrich.CategoryValues for the ones with
// a clean equivalent; generic ("engineering"), ambiguous ("data"), or non-tech-with-no-vocab
// categories stay empty so the title dictionary decides rather than guessing.
func fourDayWeekCategory(category string) string {
	switch category {
	case "devops":
		return "devops"
	case "security":
		return "security"
	case "product":
		return "product"
	case "design":
		return "design"
	case "sales":
		return "sales"
	case "marketing":
		return "marketing"
	default:
		return ""
	}
}
