package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// werecruit adapts careers.werecruit.io career sites. A board is "<locale>/<tenant>", both
// segments required, exactly as the platform's own public URLs carry them
// (careers.werecruit.io/<locale>/<tenant>/…). The locale is load-bearing, not cosmetic: a
// tenant configured for only one locale answers every OTHER locale with an empty listing rather
// than an error — verified live (see internal/ingest/sources/AGENTS.md's "werecruit traps"
// section) — so it is kept as part of the board id rather than folded off the way Dayforce's
// optional culture segment is.
//
// The tenant's listing page embeds its WHOLE open-postings list server-side as
// `window.allOffers = [...]` — no pagination exists. The listing carries no description, so each
// posting's body comes from its own page (already linked by the listing's own Url field), fanned
// out under the shared bounded-concurrency fetchDetails helper — the Factorial shape (fetch
// every detail every crawl; boards found are small enough that this is cheap), not a
// HydratingSource.
type werecruitHTTP interface {
	TextGetter
	HTMLGetter
}

type werecruit struct {
	http werecruitHTTP
}

// NewWerecruit builds the werecruit adapter over the given HTTP client.
func NewWerecruit(c werecruitHTTP) Source { return werecruit{http: c} }

func (werecruit) Provider() string { return "werecruit" }

const werecruitBaseURL = "https://careers.werecruit.io"

// werecruitOffersMarker locates the `window.allOffers = ` assignment. A json.Decoder positioned
// right after it reads exactly the one JSON array that follows and stops where it ends, so
// extraction needs no brittle end-of-array regex boundary.
var werecruitOffersMarker = regexp.MustCompile(`window\.allOffers\s*=\s*`)

// werecruitOffer is one listed posting. Address_State is a two-letter ISO COUNTRY code despite
// the name (confirmed live: "FR", "CH" — never a US state on a non-US address).
type werecruitOffer struct {
	ID                   string `json:"Id"`
	TitleTranslated      string `json:"TitleTranslated"`
	URL                  string `json:"Url"`
	AddressCity          string `json:"Address_City"`
	AddressRegion        string `json:"Address_Region"`
	AddressState         string `json:"Address_State"`
	TimeTranslated       string `json:"TimeTranslated"`
	PublicationStartDate string `json:"PublicationStartDate"`
}

func (s werecruit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	locale, tenant, err := parseWerecruitBoard(e.Board)
	if err != nil {
		return nil, err
	}
	listingURL := fmt.Sprintf("%s/%s/%s", werecruitBaseURL, locale, tenant)
	page, err := s.http.GetText(ctx, listingURL)
	if err != nil {
		return nil, fmt.Errorf("werecruit: listing %s: %w", e.Board, err)
	}
	offers, err := werecruitDecodeOffers(page)
	if err != nil {
		return nil, fmt.Errorf("werecruit: %s: %w", e.Board, err)
	}
	return fetchDetails(offers, defaultDetailWorkers, func(o werecruitOffer) (Job, bool) {
		return s.detail(ctx, e, o)
	}), nil
}

// parseWerecruitBoard splits "<locale>/<tenant>", requiring both segments non-empty.
func parseWerecruitBoard(board string) (locale, tenant string, err error) {
	parts := strings.Split(board, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("werecruit: board %q must be \"locale/tenant\"", board)
	}
	return parts[0], parts[1], nil
}

// werecruitDecodeOffers locates the window.allOffers assignment in a listing page and decodes
// the JSON array that follows. A page with no such assignment (an unconfigured locale, which the
// platform answers as a page with nothing to list) decodes to an empty slice, not an error.
func werecruitDecodeOffers(page string) ([]werecruitOffer, error) {
	loc := werecruitOffersMarker.FindStringIndex(page)
	if loc == nil {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(page[loc[1]:]))
	var offers []werecruitOffer
	if err := dec.Decode(&offers); err != nil {
		return nil, fmt.Errorf("decode allOffers: %w", err)
	}
	return offers, nil
}

// detail fetches one posting's own page for its description, returning ok=false when the fetch
// fails or the posting has no id (which would collide on the dedup key) — the same posture
// factorial/cornerstone take on a per-posting detail failure.
func (s werecruit) detail(ctx context.Context, e CompanyEntry, o werecruitOffer) (Job, bool) {
	if o.ID == "" {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, o.URL)
	if err != nil {
		return Job{}, false
	}
	description := elementInnerHTMLByClass(root, "div", "description")

	location := joinNonEmpty(o.AddressCity, o.AddressRegion)
	return Job{
		ExternalID:     o.ID,
		URL:            o.URL,
		Title:          strings.TrimSpace(o.TitleTranslated),
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(description),
		Remote:         isRemote(location),
		EmploymentType: werecruitEmploymentType(o.TimeTranslated),
		Countries:      countryFromCode(o.AddressState),
		PostedAt:       parseRFC3339(o.PublicationStartDate),
	}, true
}

// werecruitEmploymentType maps the platform's own schedule label to our vocabulary; every other
// value (an employer could type anything) is left to the pipeline's own dictionaries.
func werecruitEmploymentType(timeTranslated string) string {
	switch strings.ToLower(strings.TrimSpace(timeTranslated)) {
	case "full time", "full-time":
		return "full_time"
	case "part time", "part-time":
		return "part_time"
	default:
		return ""
	}
}
