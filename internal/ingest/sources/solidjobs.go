package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// solidjobs adapts solid.jobs, a Polish job board split into divisions (IT, finance, ...).
// Not boardless: like whatjobs, each board-file entry's board is a SEARCH SLICE rather than a
// tenant id — here a division slug (e.g. "it") rather than a keyword — so it stays an
// aggregator without the boardless marker. company is a display label only; each job's real
// employer comes from the posting.
//
// The public endpoint requires a campaign query param (any lowercase-alphanumeric-dash value;
// it is echoed back into every posting's own URL, so a stable identifier is used rather than a
// throwaway one) and ships each posting's full description inline (no detail call).
//
// KNOWN LIMITATION: the response reports totalPages/totalCount but the endpoint has no
// discoverable way to reach pages beyond the first: pageIndex, pageNumber, page, skip, offset and
// a POST body all silently return page 0 again. Only the first (fixed) 500 postings per division
// are reachable; the rest is a real gap, left for whoever finds the actual paging mechanism (or
// an authenticated one).
type solidjobs struct {
	http JSONGetter
}

// solidjobsCampaign is the stable identifying value sent as the required campaign param. It is
// echoed into every posting's returned url (".../o/<key>/<campaign>"), which is also why it is
// fixed rather than randomly generated per request.
const solidjobsCampaign = "freehire"

const solidjobsListURL = "https://solid.jobs/public-api/offers/%s?campaign=" + solidjobsCampaign

// NewSolidJobs builds the SolidJobs adapter over the given HTTP client.
func NewSolidJobs(c JSONGetter) Source { return solidjobs{http: c} }

func (solidjobs) Provider() string { return "solidjobs" }

func (solidjobs) Aggregator() {}

// solidjobsResponse is the one reachable page (see the KNOWN LIMITATION doc above).
type solidjobsResponse struct {
	Jobs []solidjobsPosting `json:"jobs"`
}

// solidjobsPosting is one posting, body inline (no detail call).
type solidjobsPosting struct {
	JobOfferKey     string   `json:"jobOfferKey"`
	Title           string   `json:"title"`
	Company         string   `json:"company"`
	Locations       []string `json:"locations"`
	Description     string   `json:"description"`
	URL             string   `json:"url"`
	IsRemote        bool     `json:"isRemote"`
	IsHybrid        bool     `json:"isHybrid"`
	ContractTime    string   `json:"contractTime"`
	ExperienceLevel string   `json:"experienceLevel"`
	ValidFrom       string   `json:"validFrom"`
	Skills          []struct {
		Name string `json:"name"`
	} `json:"skills"`
}

func (s solidjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	division := strings.TrimSpace(e.Board)
	if division == "" {
		return nil, fmt.Errorf("solidjobs: company %q has a blank division board", e.Company)
	}
	var resp solidjobsResponse
	url := fmt.Sprintf(solidjobsListURL, division)
	if err := s.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("solidjobs: division %q: %w", division, err)
	}
	jobs := make([]Job, 0, len(resp.Jobs))
	for _, p := range resp.Jobs {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// native id, or no company which would break the slug).
func (p solidjobsPosting) toJob() (Job, bool) {
	if p.JobOfferKey == "" || p.Company == "" {
		return Job{}, false
	}
	names := make([]string, len(p.Skills))
	for i, sk := range p.Skills {
		names[i] = sk.Name
	}
	return Job{
		ExternalID:     p.JobOfferKey,
		URL:            p.URL,
		Title:          p.Title,
		Company:        p.Company,
		Location:       joinNonEmpty(p.Locations...),
		Description:    sanitizeHTML(p.Description),
		Remote:         p.IsRemote,
		WorkMode:       workModeFromRemoteHybrid(p.IsRemote, p.IsHybrid),
		EmploymentType: solidjobsEmploymentType(p.ContractTime),
		Seniority:      solidjobsSeniority(p.ExperienceLevel),
		Skills:         skilltag.Canonicalize(names),
		PostedAt:       parseRFC3339(p.ValidFrom),
	}, true
}

// solidjobsEmploymentType maps the platform's contractTime field, which already uses freehire's
// own spelling for "full_time" and "part_time". Anything else maps to "" rather than a guess
// (vocab.EmploymentTypeValues is dict-only).
func solidjobsEmploymentType(contractTime string) string {
	switch contractTime {
	case "full_time", "part_time":
		return contractTime
	default:
		return ""
	}
}

// solidjobsSeniority maps the platform's own experienceLevel taxonomy ("Intern", "Junior",
// "Regular", "Senior") to freehire's controlled vocabulary. "Regular" is the Polish IT market's
// usual label for a mid-level developer. An unrecognized value maps to "" rather than a guess.
func solidjobsSeniority(level string) string {
	switch level {
	case "Intern":
		return "intern"
	case "Junior":
		return "junior"
	case "Regular":
		return "middle"
	case "Senior":
		return "senior"
	default:
		return ""
	}
}
