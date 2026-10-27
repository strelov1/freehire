package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// kaspersky adapts Kaspersky's careers site (careers.kaspersky.ru), a single-company source with
// no per-tenant board id (boardless). There is no public JSON API (/api/ is disallowed in
// robots.txt); the site is a Next.js App Router app whose listing page server-renders its data
// into the RSC flight, so the structured fields come from there rather than from markup. The
// listing carries no description, which only the posting page holds, so the adapter hydrates
// (HydratingSource). Measured 2026-10-14: 85 postings, 13 a page over pages 0-6, worldwide —
// Moscow plus sales offices abroad.
type kaspersky struct {
	http kasperskyHTTP
}

// kasperskyHTTP is the transport kaspersky needs: listing pages, and posting pages whose final
// URL after redirects says whether the posting still exists.
type kasperskyHTTP interface {
	HTMLGetter
	HTMLResolvedGetter
}

const (
	// kasperskyListURL is the listing; page is 0-based (page=0 and no page are the same set).
	// The page size is fixed at 13 — size/limit/pageSize are ignored.
	kasperskyListURL = "https://careers.kaspersky.ru/vacancies?page=%d"
	kasperskyJobURL  = "https://careers.kaspersky.ru/vacancy/%s"
	// kasperskyListKey anchors the listing array in the flight. The bare field name is not
	// enough: the page's i18n messages carry a "vacancies" key too, with a string value, and it
	// comes first.
	kasperskyListKey = `"vacancies":[`
	// kasperskyMaxPages bounds the walk as a backstop; totalCount is the primary stop.
	kasperskyMaxPages = 50
)

// kasperskyTotalCount reads the catalogue size the listing states beside its array.
var kasperskyTotalCount = regexp.MustCompile(`"totalCount":(\d+)`)

// NewKaspersky builds the Kaspersky adapter over the given HTTP client.
func NewKaspersky(c kasperskyHTTP) Source { return kaspersky{http: c} }

func (kaspersky) Provider() string { return "kaspersky" }

// kaspersky is single-company, so its config entries carry no board.
func (kaspersky) boardless() {}

// kasperskyNamed is the {name, engName} shape of the listing's dictionary-backed fields. name
// is Russian for Russian places and categories and English for the rest; engName is English
// throughout, which is what the location and skill dictionaries read best.
type kasperskyNamed struct {
	Name    string `json:"name"`
	EngName string `json:"engName"`
}

func (n kasperskyNamed) label() string { return firstNonEmpty(strings.TrimSpace(n.EngName), n.Name) }

// kasperskyItem is one listed posting. jobReqId is either a numeric requisition id or a 24-hex
// one; both route the posting page.
type kasperskyItem struct {
	JobReqID string `json:"jobReqId"`
	Titles   []struct {
		Title string `json:"title"`
	} `json:"titles"`
	Skills     []kasperskyNamed `json:"skills"`
	Cities     []kasperskyNamed `json:"cities"`
	Categories []kasperskyNamed `json:"categories"`
}

// Fetch is the list-only crawl (no description) — the fallback for callers that do not drive
// hydration. FetchNew is the hydrating path used by ingest.
func (k kaspersky) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := k.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, it := range items {
		if job, ok := it.toJob(e); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// errKasperskyGone marks a posting page that redirected away from the posting — the site's
// answer for a requisition that no longer exists (it 307s to /vacancies).
var errKasperskyGone = errors.New("posting page redirected to the listing")

// FetchNew pages the same listing but fetches a posting page only when the catalogue does not
// already hold the posting. A seen posting yields a liveness-only refresh; a gone one (404/410,
// or a redirect to the listing) is dropped; any other failure keeps the posting list-only, since
// the catalogue re-offers a body-less row for hydration on later crawls.
func (k kaspersky) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := k.list(ctx)
	if err != nil {
		return nil, err
	}
	var attempted, failed, refreshed atomic.Int64
	jobs := fetchDetails(items, defaultDetailWorkers, func(it kasperskyItem) (Job, bool) {
		base, ok := it.toJob(e)
		if !ok {
			return Job{}, false
		}
		if seen(base.ExternalID) {
			refreshed.Add(1)
			base.SeenRefresh = true
			return base, true
		}
		attempted.Add(1)
		body, err := k.detail(ctx, base.URL)
		switch {
		case errors.Is(err, errKasperskyGone), err != nil && !detailUnreadable(err):
			return Job{}, false
		case err != nil:
			failed.Add(1)
			log.Printf("kaspersky: detail %s failed; ingesting list-only: %v", base.ExternalID, err)
			return base, true
		}
		base.Description = body
		return base, true
	})
	// A run that listed postings and read none of them is a board failure, not an empty crawl
	// (internal/ingest/sources/AGENTS.md, "A crawl that reads nothing of what it listed must
	// fail") — and for a flight-parsing adapter the likeliest cause is a markup change.
	if n := attempted.Load(); n > 0 && failed.Load() == n && refreshed.Load() == 0 {
		return nil, fmt.Errorf("kaspersky: listed %d postings and read none of them", len(items))
	}
	return jobs, nil
}

// list walks the 0-based listing pages until it holds totalCount postings or a page comes back
// empty. Per the house pagination rule, the first page failing is a board-level error, while a
// later page failing ends the walk with the postings gathered so far.
func (k kaspersky) list(ctx context.Context) ([]kasperskyItem, error) {
	var items []kasperskyItem
	for page := 0; page < kasperskyMaxPages; page++ {
		got, total, err := k.page(ctx, page)
		if err != nil {
			if page == 0 {
				return nil, fmt.Errorf("kaspersky: list page %d: %w", page, err)
			}
			log.Printf("kaspersky: list page %d failed, stopping with %d postings gathered: %v", page, len(items), err)
			return items, nil
		}
		items = append(items, got...)
		if len(got) == 0 || len(items) >= total {
			return items, nil
		}
	}
	return items, nil
}

// page fetches one listing page and decodes its postings and the stated total from the flight.
// A page whose flight lacks the listing array is an error rather than an empty page: a markup
// change must surface loudly instead of reading as a catalogue that emptied overnight.
func (k kaspersky) page(ctx context.Context, page int) ([]kasperskyItem, int, error) {
	flight, err := fetchFlight(ctx, k.http, fmt.Sprintf(kasperskyListURL, page))
	if err != nil {
		return nil, 0, err
	}
	raw, ok := bracketSlice(flight, kasperskyListKey, '[', ']')
	if !ok {
		return nil, 0, fmt.Errorf("listing array not found in the flight")
	}
	var items []kasperskyItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, 0, fmt.Errorf("decode listing: %w", err)
	}
	total := 0
	if m := kasperskyTotalCount.FindStringSubmatch(flight[strings.Index(flight, kasperskyListKey):]); m != nil {
		total, _ = strconv.Atoi(m[1])
	}
	return items, total, nil
}

// detail fetches a posting page and returns its sanitized body. The body is the server-rendered
// element marked data-testid="vacancy-body"; the "Мы предлагаем" block beside it is site-wide
// benefits boilerplate, identical on every Russian posting and absent abroad, and stays out.
func (k kaspersky) detail(ctx context.Context, url string) (string, error) {
	root, final, err := k.http.GetHTMLResolved(ctx, url)
	if err != nil {
		return "", err
	}
	if final != "" && !strings.Contains(final, "/vacancy/") {
		return "", errKasperskyGone
	}
	var body *html.Node
	walk(root, func(n *html.Node) bool {
		if body == nil && n.Type == html.ElementNode && Attr(n, "data-testid") == "vacancy-body" {
			body = n
		}
		return body == nil
	})
	if body == nil {
		return "", fmt.Errorf("no vacancy body on %s", url)
	}
	desc := sanitizeHTML(innerHTML(body))
	if strings.TrimSpace(desc) == "" {
		return "", fmt.Errorf("empty vacancy body on %s", url)
	}
	return desc, nil
}

// toJob maps a listed posting to a Job, returning ok=false for one with no id or no title.
func (it kasperskyItem) toJob(e CompanyEntry) (Job, bool) {
	id := strings.TrimSpace(it.JobReqID)
	title := ""
	for _, t := range it.Titles {
		if title = strings.TrimSpace(t.Title); title != "" {
			break
		}
	}
	if id == "" || title == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID: id,
		URL:        fmt.Sprintf(kasperskyJobURL, id),
		Title:      title,
		Company:    e.Company,
		Location:   distinctJoin(it.Cities, ", ", kasperskyNamed.label),
		Category:   kasperskyCategory(it.Categories),
	}
	job.Skills = skilltag.Canonicalize(kasperskySkillNames(it.Skills))
	if kasperskyIsInternship(it.Categories) {
		job.EmploymentType = "internship"
	}
	return job, true
}

// kasperskySkillNames lists a posting's skill tags for the skill dictionary. The site spells
// some as "Name (Alias)" — "Golang (Go)" — which matches no dictionary entry whole, so such a
// tag contributes both halves; the dictionary keeps whichever it knows.
func kasperskySkillNames(skills []kasperskyNamed) []string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		l := s.label()
		open, closing := strings.IndexByte(l, '('), strings.LastIndexByte(l, ')')
		if open > 0 && closing > open {
			names = append(names, strings.TrimSpace(l[:open]), strings.TrimSpace(l[open+1:closing]))
			continue
		}
		names = append(names, l)
	}
	return names
}

// kasperskyCategories maps the site's categories (by English name) that pin exactly ONE freehire
// category. The rest are deliberately absent: the "Development <language>" categories name a
// language, not a discipline (JS is frontend or Node); "Sales & Presales", "System and Business
// Analysis" and "Business Development" each span two roles; "System Administration" and "Product
// Development" pin none; "Internship" is an employment type, read by kasperskyIsInternship.
var kasperskyCategories = map[string]string{
	"information security":      "security",
	"testing automation | sdet": "qa",
	"testing manual":            "qa",
	"devops":                    "devops",
	"data analysis":             "data_analytics",
	"software architect":        "architecture",
	"design":                    "design",
	"technical support":         "it_support",
	"technical writer":          "technical_writing",
	"project management":        "project_management",
	"finance":                   "finance",
	"human resources":           "hr",
	"legal":                     "legal",
	"marketing & pr":            "marketing",
	"administrative staff":      "administration",
}

// kasperskyCategory resolves a posting's categories to one freehire category, or "" when they
// resolve to none or to more than one — a posting filed under Finance and Legal names no single
// discipline, and choosing one would state what the source did not.
func kasperskyCategory(cats []kasperskyNamed) string {
	found := ""
	for _, c := range cats {
		v := kasperskyCategories[strings.ToLower(strings.TrimSpace(c.EngName))]
		switch {
		case v == "" || v == found:
		case found == "":
			found = v
		default:
			return ""
		}
	}
	return found
}

// kasperskyIsInternship reports whether the posting is filed under the Internship category.
func kasperskyIsInternship(cats []kasperskyNamed) bool {
	for _, c := range cats {
		if strings.EqualFold(strings.TrimSpace(c.EngName), "internship") {
			return true
		}
	}
	return false
}
