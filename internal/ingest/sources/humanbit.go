package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// humanbit adapts HumanBit, a multi-tenant ATS (jobs.humanbit.ai/<board>); the board is the
// tenant's path segment. A Next.js App Router site decoded via the shared
// fetchFlight/bracketSlice/nextFlightTextRows primitives.
//
// The two page shapes carry complementary fields: the listing has every posting's id and the
// company display name (org_name) but no employment type/skills/remote flag; a detail page has
// those but never org_name. The listing is ~5.6 MB (framework chunks) against a ~59 KB detail
// page, so Fetch reads the listing once, then hydrates each posting from its detail page, like
// hiringthing/topco.
type humanbit struct {
	http HTMLGetter
}

// NewHumanBit builds the HumanBit adapter over the given HTML client.
func NewHumanBit(c HTMLGetter) Source { return humanbit{http: c} }

func (humanbit) Provider() string { return "humanbit" }

// fullBoardListing: the listing proves the whole set of open postings, and a detail-fetch failure
// for one posting becomes an Unreadable marker rather than a silent drop (see detail). A listing
// fetch/decode failure fails the whole Fetch.
func (humanbit) fullBoardListing() {}

func (h humanbit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	listingURL := fmt.Sprintf("https://jobs.humanbit.ai/%s", e.Board)
	listingFlight, err := fetchFlight(ctx, h.http, listingURL)
	if err != nil {
		return nil, fmt.Errorf("humanbit: board %q: %w", e.Board, err)
	}
	listing, err := flightArray[humanbitListJob](listingFlight, `"jobs":`)
	if err != nil {
		return nil, fmt.Errorf("humanbit: board %q: %w", e.Board, err)
	}
	company := e.Company
	if len(listing) > 0 {
		company = firstNonEmpty(listing[0].OrgName, e.Company)
	}

	var ids []string
	for _, j := range listing {
		if j.ID != "" {
			ids = append(ids, j.ID)
		}
	}

	return fetchDetails(ids, defaultDetailWorkers, func(id string) (Job, bool) {
		return h.detail(ctx, e, company, id)
	}), nil
}

// humanbitListJob is one listing entry: the posting id and the platform's company display name
// (org_name, the same for every posting on a board). `"jobs":` is the only occurrence of that key
// on the page, so flightArray's first-match anchor cannot pick up an unrelated array.
type humanbitListJob struct {
	ID      string `json:"id"`
	OrgName string `json:"org_name"`
}

// humanbitJob is the object a posting's detail page carries. description is a "$<id>" reference
// into THIS page's flight text rows, not the listing's, so resolve it against the detail flight.
type humanbitJob struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Location       string   `json:"location"`
	EmploymentType []string `json:"employment_type"`
	Remote         bool     `json:"remote"`
	Skills         []string `json:"skills"`
	CreatedAt      string   `json:"created_at"`
}

// extractHumanBitJob decodes the "job":{"id"...} object out of a detail page's flight. ok is false
// when the page has none: a stale/removed posting answers HTTP 200 with a client "not found" tree.
func extractHumanBitJob(flight string) (humanbitJob, bool) {
	raw, ok := bracketSlice(flight, `"job":{"id"`, '{', '}')
	if !ok {
		return humanbitJob{}, false
	}
	var j humanbitJob
	if json.Unmarshal([]byte(raw), &j) != nil {
		return humanbitJob{}, false
	}
	return j, true
}

// detail fetches one posting's page and maps it to a Job. A transport failure that says nothing
// about the posting yields an Unreadable marker (the page is the only source); a page with no job
// object (the soft-404 shape) or a platform-stated gone status drops the posting.
func (h humanbit) detail(ctx context.Context, e CompanyEntry, company, id string) (Job, bool) {
	url := fmt.Sprintf("https://jobs.humanbit.ai/%s/jobs/%s", e.Board, id)
	flight, err := fetchFlight(ctx, h.http, url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, url, company), true
		}
		return Job{}, false
	}
	j, ok := extractHumanBitJob(flight)
	if !ok {
		return Job{}, false
	}

	desc := j.Description
	if ref, isRef := strings.CutPrefix(desc, "$"); isRef {
		desc = nextFlightTextRows(flight)[ref]
	}

	workMode := ""
	if j.Remote {
		workMode = "remote"
	}

	return Job{
		ExternalID:     id,
		URL:            url,
		Title:          strings.TrimSpace(j.Title),
		Company:        company,
		Location:       strings.TrimSpace(j.Location),
		Description:    sanitizeHTML(desc),
		Remote:         j.Remote || isRemote(j.Title+" "+j.Location),
		WorkMode:       workMode,
		EmploymentType: humanbitEmploymentType(j.EmploymentType),
		Skills:         humanbitSkills(j.Skills),
		PostedAt:       parseRFC3339(j.CreatedAt),
	}, true
}

// humanbitEmploymentType maps HumanBit's employment_type array onto vocab.EmploymentTypeValues,
// returning the first recognized element or "". Only "full-time" is confirmed live; the other
// spellings are the usual defensive mapping for self-evident words.
func humanbitEmploymentType(types []string) string {
	for _, t := range types {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "full-time", "full_time", "fulltime":
			return "full_time"
		case "part-time", "part_time", "parttime":
			return "part_time"
		case "contract", "contractor", "temporary", "freelance":
			return "contract"
		case "internship", "intern":
			return "internship"
		}
	}
	return ""
}

// humanbitSkills canonicalizes HumanBit's skill strings through the skilltag dictionary. Entries
// are compound phrases ("Cost Accounting"), like micro1Skills, so they are joined into one blob
// and mined via Parse rather than matched whole.
func humanbitSkills(skills []string) []string {
	return skilltag.Parse(strings.Join(skills, " "), skilltag.WithSkillList())
}
