package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// recrutei adapts Recrutei (jobs.recrutei.com.br), a Brazilian multi-tenant ATS. The board is
// the tenant's path segment. The listing is the tenant's own internal frontend API — found by
// capturing real browser network traffic, not from any published documentation, since the
// listing page's HTML/JS carries no discoverable endpoint by itself: a single
// POST /api/v2/vacancies/per-departments/<board> with body {"search":""} returns every open
// posting grouped by department, with the declared total equal to the summed item count
// (verified live). Each item already carries its own title, company name, location, and
// employment regime — only the description and post date come from a per-posting detail
// fetch, decoded from the page's schema.org application/ld+json JobPosting block via the
// shared ldJobPosting decoder (the same one geekhunter already uses).
type recrutei struct {
	http recruteiHTTP
}

// recruteiHTTP is the transport recrutei needs: the POST listing plus the HTML detail page.
type recruteiHTTP interface {
	JSONPoster
	HTMLGetter
}

// NewRecrutei builds the Recrutei adapter over the given POST+HTML client.
func NewRecrutei(c recruteiHTTP) Source { return recrutei{http: c} }

func (recrutei) Provider() string { return "recrutei" }

// fullBoardListing: the listing proves the whole set of open postings by construction (one
// request, no pagination), verified against its own declared total on every fetch (see
// listRecrutei). A detail-fetch failure for one posting becomes an Unreadable marker rather
// than a silent drop, the same contract a listing-then-detail adapter like geekhunter or
// humanbit already gives this marker. A listing fetch, decode, or completeness-check failure
// fails the whole Fetch outright.
func (recrutei) fullBoardListing() {}

func (s recrutei) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(items, defaultDetailWorkers, func(it recruteiItem) (Job, bool) {
		return s.detail(ctx, e, it)
	}), nil
}

// recruteiListResponse is the per-departments endpoint's envelope.
type recruteiListResponse struct {
	Data struct {
		Total     int                  `json:"total"`
		Vacancies []recruteiDepartment `json:"vacancies"`
	} `json:"data"`
}

type recruteiDepartment struct {
	Department string         `json:"department"`
	Items      []recruteiItem `json:"items"`
}

// recruteiItem is one listing posting. Everything except the description and post date is
// read straight from here — the detail page's own copies of location/employmentType are
// unreliable (see design.md: a literal "undefined" locality and a constant "FULL_TIME").
type recruteiItem struct {
	ID          int              `json:"id"`
	Title       string           `json:"title"`
	Regime      string           `json:"regime"`
	CompanyName string           `json:"company_name"`
	Location    recruteiLocation `json:"location"`
	PublicLink  string           `json:"public_link"`
}

// recruteiLocation decodes the listing item's "location" field, which the platform emits as
// EITHER a JSON array of strings (the shape every sample carried during design) or, for a
// posting with no stated address, the bare Portuguese placeholder string "Não informado"
// ("not stated") — found live on ~20% of a real tenant's postings, which broke the very
// first production crawl with "cannot unmarshal string into []string" the day this adapter
// shipped. "Não informado" decodes to an empty slice (no location, never a literal
// placeholder string in the job — the same posture the detail page's "undefined" leak
// already earned in design.md); any OTHER bare string is kept as a single-element location,
// since nothing observed live rules out a real value ever being sent that way.
type recruteiLocation []string

func (l *recruteiLocation) UnmarshalJSON(b []byte) error {
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*l = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s != "" && s != "Não informado" {
		*l = []string{s}
	} else {
		*l = nil
	}
	return nil
}

// list fetches the tenant's whole listing in one POST and verifies the declared total against
// the summed item count across departments — the only completeness signal this endpoint
// offers, since it carries no pagination parameters at all.
func (s recrutei) list(ctx context.Context, board string) ([]recruteiItem, error) {
	url := "https://api.recrutei.com.br/api/v2/vacancies/per-departments/" + board
	var resp recruteiListResponse
	if err := s.http.PostJSON(ctx, url, map[string]string{"search": ""}, &resp); err != nil {
		return nil, fmt.Errorf("recrutei: board %q: %w", board, err)
	}

	var items []recruteiItem
	for _, dept := range resp.Data.Vacancies {
		items = append(items, dept.Items...)
	}
	if len(items) != resp.Data.Total {
		return nil, fmt.Errorf("recrutei: board %q: declared total %d disagrees with %d summed items",
			board, resp.Data.Total, len(items))
	}
	return items, nil
}

// detail fetches one posting's page and maps it (with its listing fields) to a Job. A
// transport failure that states nothing about the posting yields an Unreadable marker, since
// the detail page is this adapter's only source for the description and post date; a page
// that answers but carries no JobPosting ld+json block drops the posting instead.
func (s recrutei) detail(ctx context.Context, e CompanyEntry, it recruteiItem) (Job, bool) {
	id := strconv.Itoa(it.ID)
	location := joinNonEmpty([]string(it.Location)...)
	company := firstNonEmpty(it.CompanyName, e.Company)

	root, err := s.http.GetHTML(ctx, it.PublicLink)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, it.PublicLink, company), true
		}
		return Job{}, false
	}
	var ld struct {
		Description string `json:"description"`
		DatePosted  string `json:"datePosted"`
	}
	if !ldJobPosting(root, &ld) {
		return unreadableDetail(id, it.PublicLink, company), true
	}

	return Job{
		ExternalID:     id,
		URL:            it.PublicLink,
		Title:          strings.TrimSpace(it.Title),
		Company:        company,
		Location:       location,
		Description:    sanitizeHTML(ld.Description),
		Remote:         isRemote(it.Title + " " + location),
		EmploymentType: recruteiEmploymentType(it.Regime),
		PostedAt:       parseLayout("02/01/2006 15:04:05", ld.DatePosted),
	}, true
}

// recruteiEmploymentType maps Recrutei's Brazilian labor-regime vocabulary onto
// vocab.EmploymentTypeValues. CLT is the standard full-time employment contract; Pessoa
// Jurídica ("PJ") is a legal-entity/freelance contractor arrangement. "CLT ou PJ" (either) and
// "Não informado" (not stated) are genuinely ambiguous or unstated, so they map to "" rather
// than a guess — the detail page's own ld+json employmentType is NOT used here: verified live
// that it reports "FULL_TIME" for postings of every regime, a platform-side constant rather
// than a real per-posting signal.
func recruteiEmploymentType(regime string) string {
	switch strings.TrimSpace(regime) {
	case "CLT":
		return "full_time"
	case "Pessoa Jurídica":
		return "contract"
	default:
		return ""
	}
}
