package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// humanbit adapts HumanBit, a multi-tenant ATS (jobs.humanbit.ai/<board>). The board is the
// tenant's path segment. Like Scalis, it is a Next.js App Router site decoded via the
// shared fetchFlight/bracketSlice/nextFlightTextRows primitives, but its two page shapes
// carry complementary rather than identical fields: the listing embeds every posting's id
// and the platform's own company display name (org_name) but no employment
// type/skills/remote flag, while a posting's own detail page carries those structured
// fields but never org_name. Confirmed live: the listing is ~5.6 MB (dominated by
// framework chunks) against a ~59 KB detail page, so Fetch reads the listing once for
// enumeration and the employer name, then hydrates each posting from its own detail page —
// the same "cheap list, per-item hydrate" shape hiringthing/topco already use.
type humanbit struct {
	http HTMLGetter
}

// NewHumanBit builds the HumanBit adapter over the given HTML client.
func NewHumanBit(c HTMLGetter) Source { return humanbit{http: c} }

func (humanbit) Provider() string { return "humanbit" }

// fullBoardListing: the listing proves the whole set of open postings, and a detail-fetch
// failure for one posting becomes an Unreadable marker rather than a silent drop (see
// detail) — the same contract a listing-then-detail adapter like successfactors already
// gives this marker. A listing fetch/decode failure fails the whole Fetch outright.
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

// humanbitListJob is one listing-page entry: the posting id and the platform's own company
// display name (org_name is carried per-posting in the real payload, not once at the top
// level, but is the same value for every posting on a board). Confirmed live that
// `"jobs":` is the only occurrence of that key on the whole page, so flightArray's
// first-match anchor cannot pick up an unrelated array.
type humanbitListJob struct {
	ID      string `json:"id"`
	OrgName string `json:"org_name"`
}

// humanbitJob is the structured object a posting's own detail page carries.
// description is a "$<id>" reference into THIS SAME page's flight text rows — confirmed
// live that the listing page's own text rows do NOT carry the same id, so it must be
// resolved against the detail flight, not the listing's.
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

// extractHumanBitJob decodes the "job":{"id"...} object out of a detail page's flight. ok
// is false when the page carries no such object — the shape a stale/removed posting
// answers with (a real, decodable flight whose tree renders a client "not found" state
// rather than a job, confirmed live, HTTP 200 throughout).
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

// detail fetches one posting's detail page and maps it to a Job. A transport failure that
// states nothing about the posting (detailUnreadable) yields an Unreadable marker, since
// the detail page is this adapter's only source for the posting; a page that answers but
// carries no job object (the soft-404 shape) or a genuine platform-stated gone status drops
// the posting instead, matching the "page read successfully and said it's not there"
// distinction every other detail-fetching adapter in this package already makes.
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

// humanbitEmploymentType maps HumanBit's employment_type array onto
// vocab.EmploymentTypeValues, returning the first recognized element or "" when none match.
// "full-time" is the only value confirmed live (both sampled postings carried it); the
// other plain-English spellings below follow this codebase's usual defensive mapping for a
// self-evident word (e.g. talenthr's employment_status_name), not a guess at an opaque code.
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

// humanbitSkills canonicalizes HumanBit's raw skill strings through the shared skilltag
// dictionary. Live entries are compound phrases ("Cost Accounting", "Zero-Based
// Budgeting"), not atomic canonical tokens, the same shape micro1Skills already documents —
// so, like there, the entries are mined via Parse rather than matched whole, and joined
// into one blob first to keep that mining ability across entry boundaries.
func humanbitSkills(skills []string) []string {
	return skilltag.Parse(strings.Join(skills, " "))
}
