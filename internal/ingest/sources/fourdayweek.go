package sources

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// fourDayWeek adapts 4dayweek.io, a curated board of roles offering a shortened work week.
// Like the other aggregators it is boardless (one public API, no per-tenant board) yet lists
// many employers, so it stays in the source facet and takes each posting's company from the
// feed.
//
// It reads /api/v2/jobs, which robots.txt explicitly ALLOWS ("Allow: /api/v2") — the /api/jobs
// this adapter used until 2026-09 is under that same file's "Disallow: /api/", and the crawl
// began failing when the site started enforcing it. The move is therefore a correction, not a
// workaround, and it is a better feed besides: v2 carries the DESCRIPTION and the canonical URL
// inline, so the per-posting HTML fetch this adapter used to make is gone — with it the ~24k
// extra requests per crawl and the whole "is this posting Pro-locked?" problem, since the API
// serves the body directly.
type fourDayWeek struct {
	http fourDayWeekHTTP
}

// fourDayWeekHTTP is the slice of the HTTP client the adapter needs. Only the JSON list now:
// v2 inlines the body, so there is no detail page to fetch.
type fourDayWeekHTTP interface {
	JSONGetter
}

const (
	// fourDayWeekListURL pages the public listing; limit=100 is the largest page the API honours.
	// /api/v2 is the path robots.txt allows — see the type doc.
	fourDayWeekListURL = "https://4dayweek.io/api/v2/jobs?page=%d&limit=100"
	// fourDayWeekJobURL is the public job page, keyed by slug. v2 also returns the URL inline;
	// this is the fallback for a posting that omits it.
	fourDayWeekJobURL = "https://4dayweek.io/job/%s"
	// fourDayWeekMaxPages bounds pagination so a feed that never reports has_more=false cannot
	// loop forever. The catalogue is ~21k postings at 100/page, so this leaves ample headroom.
	fourDayWeekMaxPages = 400
)

// NewFourDayWeek builds the 4dayweek adapter over the given HTTP client.
func NewFourDayWeek(c fourDayWeekHTTP) Source { return fourDayWeek{http: c} }

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

// fourDayWeekPosting is one posting from the v2 list API: the structured facets, the body and
// the canonical URL, all inline.
type fourDayWeekPosting struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	// Company is an object in v2 (it was a bare company_name in the retired /api/jobs).
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	WorkArrangement string                `json:"work_arrangement"`
	Level           string                `json:"level"`
	Category        string                `json:"category"`
	PostedAt        string                `json:"posted_at"`
	URL             string                `json:"url"`
	Description     string                `json:"description"`
	Locations       []fourDayWeekLocation `json:"locations"`
	Stack           []struct {
		Name string `json:"name"`
	} `json:"stack"`
}

// Fetch lists the board. v2 carries every field a Job needs, so there is no second pass: a
// posting is either usable as listed or dropped by toJob.
func (s fourDayWeek) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	postings, err := s.crawl(ctx)
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

// crawl pages the list feed and returns every raw posting — the list walk behind Fetch.
func (s fourDayWeek) crawl(ctx context.Context) ([]fourDayWeekPosting, error) {
	var postings []fourDayWeekPosting
	for page := 1; page <= fourDayWeekMaxPages; page++ {
		// v2 names the array "data"; the retired /api/jobs called it "jobs".
		var resp struct {
			Data    []fourDayWeekPosting `json:"data"`
			HasMore bool                 `json:"has_more"`
		}
		if err := s.http.GetJSON(ctx, fmt.Sprintf(fourDayWeekListURL, page), &resp); err != nil {
			return nil, fmt.Errorf("4dayweek: page %d: %w", page, err)
		}
		postings = append(postings, resp.Data...)
		if len(resp.Data) == 0 || !resp.HasMore {
			break
		}
	}
	return postings, nil
}

// toJob maps a posting to a Job, returning ok=false for an unusable posting (no native id,
// which would collide on the dedup key, no slug to build the URL, or no company, which would
// break the slug). The platform's structured facets map straight into freehire's vocabularies;
// values it does not state (or that have no clean equivalent) are left empty for the pipeline's
// dictionaries to decide.
func (p fourDayWeekPosting) toJob() (Job, bool) {
	if p.ID == "" || p.Slug == "" || p.Company.Name == "" {
		return Job{}, false
	}
	names := make([]string, 0, len(p.Stack))
	for _, s := range p.Stack {
		names = append(names, s.Name)
	}
	url := p.URL
	if url == "" {
		url = fmt.Sprintf(fourDayWeekJobURL, p.Slug)
	}
	return Job{
		ExternalID:  p.ID,
		URL:         url,
		Title:       p.Title,
		Company:     p.Company.Name,
		Description: sanitizeHTML(p.Description),
		Location:    p.location(),
		Remote:      p.WorkArrangement == "remote",
		WorkMode:    fourDayWeekWorkMode(p.WorkArrangement),
		Seniority:   fourDayWeekSeniority(p.Level),
		Category:    fourDayWeekCategory(p.Category),
		// Canonicalize, not Parse: names are already-discrete asserted tech names, not
		// prose — Parse's corroboration rule would drop an unambiguous single-word name
		// for lack of a second strong term.
		Skills:   skilltag.Canonicalize(names),
		PostedAt: parseRFC3339(p.PostedAt),
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

// fourDayWeekSeniority maps the platform's level onto vocab.SeniorityValues; an unknown or
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

// fourDayWeekCategory maps the platform's category onto vocab.CategoryValues for the ones with
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
