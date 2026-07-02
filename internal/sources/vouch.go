package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// vouch adapts vouch.careers, a referral-driven careers platform. The board is the URL
// company cuid (e.g. "cmghojtua00p5et0dvusuxx5o"), and the company page
// vouch.careers/companies/<board> is a Next.js App Router app that inlines every posting
// into its RSC flight stream as a "listings":[…] JSON array — each entry fully populated
// (title, pitch, must/nice HTML, employmentType, company, locations, state flags). One GET
// per board therefore assembles every Job with no per-posting detail request, the same
// embedded-payload shape as the deel/google adapters.
type vouch struct {
	http HTMLGetter
}

const vouchBaseURL = "https://vouch.careers"

// NewVouch builds the vouch.careers adapter over the given HTML client.
func NewVouch(c HTMLGetter) Source { return vouch{http: c} }

func (vouch) Provider() string { return "vouch" }

func (s vouch) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	pageURL := fmt.Sprintf("%s/companies/%s", vouchBaseURL, e.Board)
	root, err := s.http.GetHTML(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("vouch: board %q: %w", e.Board, err)
	}
	flight, err := decodeNextFlight(root)
	if err != nil {
		return nil, fmt.Errorf("vouch: board %q: %w", e.Board, err)
	}
	// The postings are the flight's "listings":[…] array. A missing array is an error (a
	// markup change must surface loudly rather than silently empty the catalogue); an empty
	// array is valid and yields no jobs.
	raw, ok := bracketSlice(flight, `"listings":`, '[', ']')
	if !ok {
		return nil, fmt.Errorf("vouch: board %q: listings payload not found", e.Board)
	}
	var listings []vouchListing
	if err := json.Unmarshal([]byte(raw), &listings); err != nil {
		return nil, fmt.Errorf("vouch: board %q: decode listings: %w", e.Board, err)
	}

	var jobs []Job
	for _, l := range listings {
		if j, ok := s.toJob(e, l); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// toJob maps one listing to a Job. ok is false for a non-live posting (deactivated, draft,
// or unlisted — not a real vacancy) or one with no id (which would collide on the
// (source, external_id) dedup key).
func (vouch) toJob(e CompanyEntry, l vouchListing) (Job, bool) {
	if !l.Activated || l.Draft || l.Unlisted || l.ID == "" {
		return Job{}, false
	}

	// The detail URL is the listing's own relative url; resolve it against the site base,
	// falling back to the canonical /companies/<board>/<id> shape if it is absent.
	detailURL := fmt.Sprintf("%s/companies/%s/%s", vouchBaseURL, e.Board, l.ID)
	if l.URL != "" {
		if ref, err := url.Parse(l.URL); err == nil {
			detailURL = resolveVouchURL(ref)
		}
	}

	location := vouchLocationText(l.Locations)
	workMode := vouchWorkMode(l.EmploymentType)

	return Job{
		ExternalID:  l.ID,
		URL:         detailURL,
		Title:       l.Title,
		Company:     firstNonEmpty(l.Company.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(vouchDescription(l)),
		Remote:      workMode == "remote" || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseRFC3339(l.PublishedAt),
	}, true
}

// vouchBase is the parsed site base, resolved once for every listing's relative url.
var vouchBase, _ = url.Parse(vouchBaseURL)

// resolveVouchURL resolves a listing's (relative) url against the vouch.careers base.
func resolveVouchURL(ref *url.URL) string {
	return vouchBase.ResolveReference(ref).String()
}

// vouchDescription composes the posting body from the listing's parts. `description` is
// usually empty; the real content is the pitch plus the `must`/`nice` HTML sections. The
// pitch is plain prose, so it is HTML-escaped before wrapping (an unescaped "<" would make
// the sanitizer treat the tail as a bogus tag and drop it).
func vouchDescription(l vouchListing) string {
	var b strings.Builder
	if l.Pitch != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(l.Pitch))
		b.WriteString("</p>")
	}
	b.WriteString(l.Description)
	b.WriteString(l.Must)
	b.WriteString(l.Nice)
	return b.String()
}

// vouchWorkMode maps the employmentType tokens to a work mode via the shared
// workplaceTypeMode. A posting can carry several tokens (e.g. ["Full-time","Remote","Hybrid"]);
// remote is the most permissive and wins, then hybrid, then on-site. Contract-type tokens
// (Full-time/Part-time/Contract) map to "" and are ignored, as is an unrecognized set (the
// location parser/LLM still resolve).
func vouchWorkMode(types []string) string {
	var hasHybrid, hasOnsite bool
	for _, t := range types {
		switch workplaceTypeMode(t) {
		case "remote":
			return "remote"
		case "hybrid":
			hasHybrid = true
		case "onsite":
			hasOnsite = true
		}
	}
	if hasHybrid {
		return "hybrid"
	}
	if hasOnsite {
		return "onsite"
	}
	return ""
}

// vouchLocationText builds a free-text location from the listing's first location, preferring
// the human-readable address, else the city/area/country parts. Remote roles often carry no
// location, yielding "".
func vouchLocationText(locs []vouchLocation) string {
	if len(locs) == 0 {
		return ""
	}
	l := locs[0]
	return firstNonEmpty(l.Address, joinNonEmpty(l.City, l.AdministrativeArea, l.Country))
}

// vouchListing is the subset of a flight `listings` entry the adapter maps.
type vouchListing struct {
	ID             string   `json:"id"`
	URL            string   `json:"url"`
	Title          string   `json:"title"`
	Pitch          string   `json:"pitch"`
	Must           string   `json:"must"`
	Nice           string   `json:"nice"`
	Description    string   `json:"description"`
	EmploymentType []string `json:"employmentType"`
	PublishedAt    string   `json:"publishedAt"`
	Activated      bool     `json:"activated"`
	Draft          bool     `json:"draft"`
	Unlisted       bool     `json:"unlisted"`
	Company        struct {
		Name string `json:"name"`
	} `json:"company"`
	Locations []vouchLocation `json:"locations"`
}

// vouchLocation is one entry of a listing's locations array (vouch emits ISO country codes
// and a human-readable address).
type vouchLocation struct {
	Address            string `json:"address"`
	City               string `json:"city"`
	AdministrativeArea string `json:"administrative_area"`
	Country            string `json:"country"`
}
