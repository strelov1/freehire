package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// alignerr adapts Alignerr's careers site (www.alignerr.com), a single-company source with no
// per-tenant board id (boardless). Each page is a Next.js app embedding its data in the
// __NEXT_DATA__ script: the /jobs listing carries every active posting's id and title (its
// description truncated to a preview), so the full body, dates, and structured fields come
// from each posting's own detail page, fanned out like the other detail-fetching adapters.
//
// Outbound links are attributable: Alignerr pays a bounty for a referred contributor who
// onboards and logs hours, and it is earned only when the contributor arrives carrying a
// referral code. See applyURL.
type alignerr struct {
	http TextGetter
	// referral is the Alignerr referral code outbound links are attributed to, from
	// ALIGNERR_REFERRAL_CODE (see registry.go). Empty means unattributed: the URL stays the
	// plain posting page, which is what an installation that is not an Alignerr referrer —
	// including every fork of this repository — gets.
	referral string
}

// NewAlignerr builds the Alignerr adapter over the given HTTP client, attributing outbound
// links to the given referral code (empty = unattributed, see alignerr.referral).
func NewAlignerr(c TextGetter, referral string) Source {
	if !alignerrReferralCodePattern.MatchString(referral) {
		referral = "" // absent or malformed: crawl unattributed rather than emit a broken link
	}
	return alignerr{http: c, referral: referral}
}

// alignerrReferralCodePattern is the shape of an Alignerr referral code: a UUID, as its own
// share links carry. The code is interpolated into an outbound URL, so anything else is
// rejected at construction rather than escaped — a code is copied from the "Refer & earn"
// panel by hand, and a malformed one is a misconfiguration to notice.
var alignerrReferralCodePattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// applyURL hangs the configured referral code off the posting page, which is all Alignerr needs:
// the page reads the parameter and builds its OWN apply links from it, pointing at
// app.alignerr.com/signin?job=<posting id>&referral-code=<code> (verified live — the id it uses
// is the same posting id we already store, so nothing has to be looked up). An unset code leaves
// the plain posting URL alone.
func (a alignerr) applyURL(jobURL string) string {
	if a.referral == "" {
		return jobURL
	}
	return jobURL + "?referral-code=" + a.referral
}

func (alignerr) Provider() string { return "alignerr" }

// alignerr is single-company, so its config entry carries no board.
func (alignerr) boardless() {}

const (
	alignerrListURL = "https://www.alignerr.com/jobs"
	alignerrJobURL  = "https://www.alignerr.com/jobs/"
)

// alignerrNextData is the slice of the __NEXT_DATA__ payload we read: the listing exposes
// initialJobs, a detail page exposes a single job.
type alignerrNextData struct {
	Props struct {
		PageProps struct {
			InitialJobs []alignerrListItem `json:"initialJobs"`
			Job         *alignerrJob       `json:"job"`
		} `json:"pageProps"`
	} `json:"props"`
}

// alignerrListItem is one posting from the listing's initialJobs array; it carries the id used
// to build the detail URL and a location/title fallback for when the detail fetch fails.
type alignerrListItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

// alignerrJob is the detail page's job record. htmlLongDescription is the full posting body;
// isActive gates closed postings; jobType is the structured employment enum; firstPostDate is
// the publish date (createdAt is the fallback).
type alignerrJob struct {
	Name                string `json:"name"`
	HTMLLongDescription string `json:"htmlLongDescription"`
	ShortDescription    string `json:"shortDescription"`
	IsActive            bool   `json:"isActive"`
	FirstPostDate       string `json:"firstPostDate"`
	CreatedAt           string `json:"createdAt"`
	JobType             string `json:"jobType"`
	Location            string `json:"location"`
}

func (a alignerr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	body, err := a.http.GetText(ctx, alignerrListURL)
	if err != nil {
		return nil, fmt.Errorf("alignerr: listing: %w", err)
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return nil, fmt.Errorf("alignerr: no __NEXT_DATA__ in listing")
	}
	var data alignerrNextData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("alignerr: decode listing: %w", err)
	}

	// Each posting's body and structured fields come from its own detail page, fanned out
	// under a bounded pool.
	return fetchDetails(data.Props.PageProps.InitialJobs, defaultDetailWorkers, func(it alignerrListItem) (Job, bool) {
		return a.detail(ctx, e, it)
	}), nil
}

// detail fetches one posting's detail page and maps its job record to a Job, returning ok=false
// when the id is missing, the fetch fails, the page carries no job, or the posting is inactive —
// so the caller skips just that posting.
func (a alignerr) detail(ctx context.Context, e CompanyEntry, it alignerrListItem) (Job, bool) {
	id := strings.TrimSpace(it.ID)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	jobURL := alignerrJobURL + id
	body, err := a.http.GetText(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return Job{}, false
	}
	var data alignerrNextData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return Job{}, false
	}
	j := data.Props.PageProps.Job
	if j == nil || !j.IsActive {
		return Job{}, false
	}

	description := sanitizeHTML(j.HTMLLongDescription)
	if description == "" {
		// The short field is feed-controlled too, and nothing guarantees it is free of
		// markup — it goes through the same sanitizer as the long one.
		description = strings.TrimSpace(sanitizeHTML(j.ShortDescription))
	}
	// The listing presents every posting's location as "Remote"; the detail's own location is
	// the fallback when the listing item is absent.
	location := firstNonEmpty(it.Location, j.Location)
	return Job{
		ExternalID:     id,
		URL:            a.applyURL(jobURL),
		Title:          firstNonEmpty(strings.TrimSpace(j.Name), strings.TrimSpace(it.Title)),
		Company:        e.Company,
		Location:       location,
		Description:    description,
		Remote:         isRemote(location),
		EmploymentType: alignerrEmploymentType(j.JobType),
		PostedAt:       parseRFC3339(firstNonEmpty(j.FirstPostDate, j.CreatedAt)),
	}, true
}

// alignerrEmploymentType maps Alignerr's jobType enum onto the freehire vocabulary, returning
// "" for an unknown/absent value so the description parser decides.
func alignerrEmploymentType(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "FULL_TIME", "FULLTIME":
		return "full_time"
	case "PART_TIME", "PARTTIME":
		return "part_time"
	case "CONTRACT", "CONTRACTOR", "TEMPORARY":
		return "contract"
	case "INTERN", "INTERNSHIP":
		return "internship"
	default:
		return ""
	}
}
