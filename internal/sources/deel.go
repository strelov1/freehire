package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// deel adapts Deel's multi-tenant ATS (jobs.deel.com/<orgSlug>), a board-based source
// whose board id is the org's URL slug. The board page is a server-rendered Next.js app:
// it inlines the org's whole catalogue in the React flight stream (a sequence of
// self.__next_f.push([1,"…"]) chunks), including every posting's full HTML description as
// a "$N" reference to a length-delimited text row in the same stream. One GET per board
// therefore assembles every Job with no per-posting detail request — the same
// embedded-payload shape as the google adapter.
type deel struct {
	http HTMLGetter
}

const (
	// deelBoardURL is the org's career board page; %s is the org URL slug (the board id).
	deelBoardURL = "https://jobs.deel.com/%s"
	// deelJobURL is the public detail page for one posting; the args are the org slug and
	// the posting id (the same id the org sitemap addresses).
	deelJobURL = "https://jobs.deel.com/%s/job-details/%s/overview"
)

// NewDeel builds the Deel ATS adapter over the given HTML client.
func NewDeel(c HTMLGetter) Source { return deel{http: c} }

func (deel) Provider() string { return "deel" }

func (d deel) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	root, err := d.http.GetHTML(ctx, fmt.Sprintf(deelBoardURL, e.Board))
	if err != nil {
		return nil, fmt.Errorf("deel: board %q: %w", e.Board, err)
	}
	flight, err := decodeNextFlight(root)
	if err != nil {
		return nil, fmt.Errorf("deel: board %q: %w", e.Board, err)
	}
	postings, err := extractDeelPostings(flight)
	if err != nil {
		return nil, fmt.Errorf("deel: board %q: %w", e.Board, err)
	}
	org := extractDeelOrgName(flight)
	rows := deelTextRows(flight)

	var jobs []Job
	refs, resolved := 0, 0
	for _, p := range postings {
		if ref, ok := strings.CutPrefix(p.RichtextDescription, "$"); ok {
			refs++
			if strings.TrimSpace(rows[ref]) != "" {
				resolved++
			}
		}
		if j, ok := d.toJob(e, org, rows, p); ok {
			jobs = append(jobs, j)
		}
	}
	// Postings reference their descriptions by id into the flight's text rows. If every
	// reference fails to resolve, the row parse broke (e.g. the marker format changed) —
	// fail loudly rather than ship a whole board of empty-bodied jobs. A single unresolved
	// reference still yields its posting with an empty description (tolerated degradation).
	if refs > 0 && resolved == 0 {
		return nil, fmt.Errorf("deel: board %q: %d description references but none resolved", e.Board, refs)
	}
	return jobs, nil
}

// deelTextRowMarker matches the start of an RSC text row, "<id>:T<hexlen>,". RSC numbers
// both the row id and the byte length in lowercase HEX (refs run …$29, $2a, $2b, $30…),
// so the id is hex, not decimal. The leading non-hex byte anchors the id's left edge (so
// "a2a:T" is not read as "2a:T") without a lookbehind; a text row is never at offset 0, so
// requiring one preceding byte is safe.
var deelTextRowMarker = regexp.MustCompile(`[^0-9a-f]([0-9a-f]+):T([0-9a-f]+),`)

// deelTextRows returns the flight stream's text rows keyed by id. A text row is
// "<id>:T<hexlen>,<bytes>" whose content is exactly hexlen BYTES (it may itself contain
// newlines), so each marker's content is sliced by its declared byte length. A posting's
// "$N" richtextDescription reference resolves to row N's content.
func deelTextRows(flight string) map[string]string {
	b := []byte(flight)
	rows := make(map[string]string)
	for _, m := range deelTextRowMarker.FindAllSubmatchIndex(b, -1) {
		id := string(b[m[2]:m[3]])
		n, err := strconv.ParseInt(string(b[m[4]:m[5]]), 16, 64)
		if err != nil {
			continue
		}
		start := m[1] // end of the full match = first content byte after the comma
		end := start + int(n)
		if end > len(b) {
			end = len(b)
		}
		rows[id] = string(b[start:end])
	}
	return rows
}

// deelPosting is the subset of a jobPostings entry the adapter maps. id is the posting id
// used in the public URL and the org sitemap; richtextDescription is either a "$N"
// reference into the flight stream or, defensively, an inline HTML string.
type deelPosting struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	RichtextDescription string `json:"richtextDescription"`
	CreatedAt           string `json:"createdAt"`
	Job                 struct {
		JobLocations []struct {
			Location struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobLocations"`
	} `json:"job"`
}

// extractDeelPostings decodes the jobPostings array out of the flight stream. A missing
// array is an error (a markup change must surface loudly rather than silently empty the
// catalogue); an empty array is valid and yields no postings.
func extractDeelPostings(flight string) ([]deelPosting, error) {
	raw, ok := bracketSlice(flight, `"jobPostings":`, '[', ']')
	if !ok {
		return nil, fmt.Errorf("jobPostings payload not found")
	}
	var postings []deelPosting
	if err := json.Unmarshal([]byte(raw), &postings); err != nil {
		return nil, fmt.Errorf("decode jobPostings: %w", err)
	}
	return postings, nil
}

// extractDeelOrgName reads careerPageSettings.preferredOrganizationName from the flight
// stream, or "" when absent (the caller falls back to the configured company name).
func extractDeelOrgName(flight string) string {
	raw, ok := bracketSlice(flight, `"careerPageSettings":`, '{', '}')
	if !ok {
		return ""
	}
	var settings struct {
		PreferredOrganizationName string `json:"preferredOrganizationName"`
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return ""
	}
	return settings.PreferredOrganizationName
}

// toJob maps one posting to a Job. ok is false when the posting carries no id, which would
// collide on the (source, external_id) dedup key. Deel exposes no structured workplace
// field, so WorkMode is left empty (the location parser resolves it) and the remote flag
// comes from the shared heuristic over the title and location.
func (deel) toJob(e CompanyEntry, org string, rows map[string]string, p deelPosting) (Job, bool) {
	if p.ID == "" {
		return Job{}, false
	}
	desc := p.RichtextDescription
	if ref, ok := strings.CutPrefix(desc, "$"); ok {
		desc = rows[ref]
	}
	var locs []string
	for _, jl := range p.Job.JobLocations {
		locs = append(locs, jl.Location.Name)
	}
	location := joinNonEmpty(locs...)
	return Job{
		ExternalID:  p.ID,
		URL:         fmt.Sprintf(deelJobURL, e.Board, p.ID),
		Title:       p.Title,
		Company:     firstNonEmpty(org, e.Company),
		Location:    location,
		Description: sanitizeHTML(desc),
		Remote:      isRemote(p.Title + " " + location),
		PostedAt:    parseRFC3339(p.CreatedAt),
	}, true
}
