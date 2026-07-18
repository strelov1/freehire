package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// hh adapts hh.ru (HeadHunter), Russia's largest job board. It aggregates postings from many
// companies (company per posting), enumerated by professional_role id carried as the board file
// entry's board. Its public search API is IP-blocklisted from datacenter egress, so the crawl
// reads the server-rendered search page and decodes the vacancy list from its embedded
// client-hydration state (a <template id="HH-Lux-InitialState"> JSON blob). The list carries every
// field except the description, which — like justjoin/nofluffjobs — is hydrated per posting from
// the vacancy page's JobPosting ld+json, and only for postings the catalogue does not already have
// (HydratingSource.FetchNew). Multi-company, board-based.
//
// Caveat/seam: HH-Lux-InitialState is a frontend detail (the client-hydration state), not a
// documented API, so a hh.ru frontend change can move the vacancy list within it. The public
// render must carry the data somewhere; today it is this template.
type hh struct {
	http HTMLGetter
}

// NewHH builds the hh.ru adapter over the shared HTML-getter client.
func NewHH(c HTMLGetter) Source { return hh{http: c} }

func (hh) Provider() string { return "hh" }

// aggregator marks hh as a genuine multi-company aggregator: the same vacancy a company also
// posts on its own ATS appears here too, so the cross-source dedup pass prefers the first-party
// ATS copy over the hh.ru re-listing. Unlike the boardless aggregators hh still requires a board
// (professional_role id) to bound the crawl, so it is not boardless.
func (hh) aggregator() {}

const (
	hhSearchURL  = "https://hh.ru/search/vacancy"
	hhVacancyURL = "https://hh.ru/vacancy/"
	hhStateID    = "HH-Lux-InitialState"
	hhPageSize   = 100
	// hhWithinDays bounds each crawl to a recent-publish window; ordered newest-first this keeps
	// the walked set inside hh.ru's ~2000-result search depth cap and lets repeated runs converge.
	hhWithinDays = 7
	// hhMaxPages backstops the page loop at hh.ru's search depth cap (page*size ≤ ~2000).
	hhMaxPages = 2000 / hhPageSize
)

// hhState is the slice of the HH-Lux-InitialState blob the adapter reads: the vacancy list.
type hhState struct {
	VacancySearchResult struct {
		Vacancies []hhVacancy `json:"vacancies"`
	} `json:"vacancySearchResult"`
}

// hhVacancy is one search-result vacancy from the embedded state. It carries every field the
// adapter needs except the description, which is hydrated from the vacancy page.
type hhVacancy struct {
	VacancyID int64  `json:"vacancyId"`
	Name      string `json:"name"`
	IsAdv     bool   `json:"@isAdv"`
	Area      struct {
		Name string `json:"name"`
	} `json:"area"`
	Company struct {
		VisibleName string `json:"visibleName"`
		Name        string `json:"name"`
	} `json:"company"`
	Compensation hhCompensation `json:"compensation"`
	Employment   struct {
		Type string `json:"@type"`
	} `json:"employment"`
	WorkFormats []struct {
		Elements []string `json:"workFormatsElement"`
	} `json:"workFormats"`
	PublicationTime struct {
		Value string `json:"$"`
	} `json:"publicationTime"`
	Links struct {
		Desktop string `json:"desktop"`
	} `json:"links"`
}

// hhCompensation is the structured salary the list carries (the detail ld+json omits it). From/To
// are whole currency units; a bare {noCompensation:{}} leaves both nil (salary unstated).
type hhCompensation struct {
	From         *int64 `json:"from"`
	To           *int64 `json:"to"`
	CurrencyCode string `json:"currencyCode"`
	Gross        *bool  `json:"gross"`
}

// Fetch is the list-only crawl (no description) — the fallback for a non-hydrating pipeline.
func (s hh) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.crawl(ctx, e)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, v := range postings {
		if job, ok := v.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew hydrates a posting's description from its vacancy page only for a posting the catalogue
// does not already have (seen); a seen posting yields the list-only job marked SeenRefresh, so the
// pipeline refreshes liveness without re-downloading the ~1 MB detail page or wiping the hydrated
// body. A single detail failure is isolated (logged, list-only fallback) — a posting is never
// dropped over a missing detail.
func (s hh) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.crawl(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(v hhVacancy) (Job, bool) {
		base, ok := v.toJob()
		if !ok {
			return Job{}, false // unusable posting — dropped, as in Fetch
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		if body, ok := s.detail(ctx, base.URL); ok {
			base.Description += body // base.Description is the salary paragraph (or "")
		} else {
			log.Printf("hh: detail %q failed; ingesting list-only", base.URL)
		}
		return base, true
	}), nil
}

// crawl pages the search listing, decoding each page's embedded state, until a page yields no new
// vacancy or the depth cap is hit — the shared list walk behind Fetch and FetchNew. Promoted (ad)
// vacancies are skipped: they are injected across searches regardless of the role filter, so each
// role crawl keeps only genuine matches. A first-page failure is a board-level error; a later page
// failing ends the walk with the postings gathered so far, so a partial crawl survives a hiccup.
func (s hh) crawl(ctx context.Context, e CompanyEntry) ([]hhVacancy, error) {
	var out []hhVacancy
	seen := map[int64]bool{}
	for page := 0; page < hhMaxPages; page++ {
		root, err := s.http.GetHTML(ctx, s.searchURL(e.Board, page))
		if err != nil {
			if page == 0 {
				return nil, fmt.Errorf("hh: search role %q page %d: %w", e.Board, page, err)
			}
			break
		}
		st, ok := hhStateOf(root)
		if !ok {
			if page == 0 {
				return nil, fmt.Errorf("hh: search role %q page %d: no %s state", e.Board, page, hhStateID)
			}
			break
		}
		added := 0
		for _, v := range st.VacancySearchResult.Vacancies {
			if v.IsAdv || v.VacancyID == 0 || seen[v.VacancyID] {
				continue
			}
			seen[v.VacancyID] = true
			out = append(out, v)
			added++
		}
		if added == 0 { // empty page, or hh clamping ?page past its last page
			break
		}
	}
	return out, nil
}

// searchURL builds a professional_role search page, newest-first and bounded to the recent-publish
// window.
func (hh) searchURL(role string, page int) string {
	q := url.Values{}
	q.Set("professional_role", role)
	q.Set("items_on_page", strconv.Itoa(hhPageSize))
	q.Set("page", strconv.Itoa(page))
	q.Set("order_by", "publication_time")
	q.Set("period", strconv.Itoa(hhWithinDays))
	return hhSearchURL + "?" + q.Encode()
}

// toJob maps a listing vacancy to a Job, returning ok=false for an unusable posting (no id to key
// on, or no company which would break the company slug). The salary the list carries is folded into
// the description (Job has no salary field); the body is hydrated separately by FetchNew.
func (v hhVacancy) toJob() (Job, bool) {
	company := hhCompanyName(firstNonEmpty(v.Company.VisibleName, v.Company.Name))
	if v.VacancyID == 0 || company == "" {
		return Job{}, false
	}
	id := strconv.FormatInt(v.VacancyID, 10)
	jobURL := v.Links.Desktop
	if jobURL == "" {
		jobURL = hhVacancyURL + id
	}
	return Job{
		ExternalID:     id,
		URL:            jobURL,
		Title:          strings.TrimSpace(v.Name),
		Company:        company,
		Location:       strings.TrimSpace(v.Area.Name),
		Description:    v.salaryParagraph(),
		Remote:         v.workMode() == "remote",
		WorkMode:       v.workMode(),
		EmploymentType: hhEmploymentType(v.Employment.Type),
		PostedAt:       parseRFC3339(v.PublicationTime.Value),
	}, true
}

// hhCompanyName cleans an employer display name: hh.ru appends an employer's internal org-structure
// path to visibleName with a "::" separator for large employers (e.g. "HeadHunter::Analytics/Data
// Science"), which would otherwise become a bogus company name and slug. Only the top-level employer
// name is kept; clean names (no "::") pass through untouched.
func hhCompanyName(name string) string {
	name, _, _ = strings.Cut(name, "::")
	return strings.TrimSpace(name)
}

// salaryParagraph renders the structured compensation as a leading paragraph, folded into the
// description because Job has no salary field. Empty when the vacancy states no amount.
func (v hhVacancy) salaryParagraph() string {
	c := v.Compensation
	if c.From == nil && c.To == nil {
		return ""
	}
	var amount string
	switch {
	case c.From != nil && c.To != nil:
		amount = fmt.Sprintf("%d–%d %s", *c.From, *c.To, c.CurrencyCode)
	case c.From != nil:
		amount = fmt.Sprintf("от %d %s", *c.From, c.CurrencyCode)
	default:
		amount = fmt.Sprintf("до %d %s", *c.To, c.CurrencyCode)
	}
	if c.Gross != nil {
		if *c.Gross {
			amount += " (до вычета налогов)"
		} else {
			amount += " (на руки)"
		}
	}
	return sanitizeHTML("<p>Зарплата: " + strings.TrimSpace(amount) + "</p>")
}

// workMode maps hh's structured workFormats enum to our work-mode vocabulary, preferring the most
// remote arrangement a vacancy offers; an unstated/unknown format yields "".
func (v hhVacancy) workMode() string {
	set := map[string]bool{}
	for _, wf := range v.WorkFormats {
		for _, e := range wf.Elements {
			set[strings.ToUpper(strings.TrimSpace(e))] = true
		}
	}
	switch {
	case set["REMOTE"]:
		return "remote"
	case set["HYBRID"]:
		return "hybrid"
	case set["ON_SITE"]:
		return "onsite"
	default:
		return ""
	}
}

// hhEmploymentType maps hh's employment @type enum into enrich.EmploymentTypeValues, leaving
// unmapped types empty so the pipeline's dictionaries decide.
func hhEmploymentType(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "FULL":
		return "full_time"
	case "PART":
		return "part_time"
	case "PROJECT":
		return "contract"
	default:
		return ""
	}
}

// detail fetches the vacancy page and returns its sanitized JobPosting description, ok=false on a
// failed request or a page with no JobPosting ld+json so the caller falls back to the list-only job.
func (s hh) detail(ctx context.Context, vacancyURL string) (string, bool) {
	root, err := s.http.GetHTML(ctx, vacancyURL)
	if err != nil {
		return "", false
	}
	var p struct {
		Description string `json:"description"`
	}
	if !LDJobPosting(root, &p) || strings.TrimSpace(p.Description) == "" {
		return "", false
	}
	return sanitizeHTML(p.Description), true
}

// hhStateOf decodes the HH-Lux-InitialState template blob into the slice of state the adapter
// reads. ok=false when the page carries no such template or its JSON does not decode.
func hhStateOf(root *html.Node) (hhState, bool) {
	node := firstByID(root, hhStateID)
	if node == nil {
		return hhState{}, false
	}
	var st hhState
	if err := json.Unmarshal([]byte(textContent(node)), &st); err != nil {
		return hhState{}, false
	}
	return st, true
}
