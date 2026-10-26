package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// mediatek adapts MediaTek's own careers site (careers.mediatek.com), the global catalogue —
// Hsinchu, San Jose, Singapore, Bangalore and the rest. Single-company and boardless.
//
// The site is a Next.js app over an undocumented tRPC search endpoint that answers a GET with
// a JSON-encoded `input`, up to 100 postings a page, every one with its responsibilities
// inline. The requirements live only on the server-rendered posting page, so a posting the
// catalogue does not hold yet is hydrated from that page (HydratingSource); a seen one costs
// no request.
//
// fullBoardListing: the listing walks every page the search reports and must read exactly its
// total_items, every id distinct.
type mediatek struct {
	http mediatekHTTP
}

// mediatekHTTP is the transport mediatek needs: the JSON search and the posting page, which is
// read as text because it must carry the locale cookie (mediatekLocaleCookie).
type mediatekHTTP interface {
	JSONGetter
	HeaderTextGetter
}

const (
	mediatekSearchURL = "https://careers.mediatek.com/api/trpc/job.getJobs?input="
	mediatekJobURL    = "https://careers.mediatek.com/en/jobs/"
	// mediatekPageSize is the most rows the search returns per page.
	mediatekPageSize = 100
	// mediatekMaxPages is a ceiling, not a stopping rule: a catalogue that outgrows it fails the
	// crawl (621 postings, 7 pages, on 2026-10-25).
	mediatekMaxPages = 30
	// mediatekRequirementsHeading is the posting page's h3 over the requirements list.
	mediatekRequirementsHeading = "Main Requirements and Qualifications"
)

// mediatekLocaleCookie is what the posting page needs to render. Without it the site's locale
// middleware answers 307 to the same path and sets the cookie, which a client without a jar
// follows into the same redirect until it gives up.
var mediatekLocaleCookie = map[string]string{"Cookie": "NEXT_LOCALE=en"}

// mediatekCategories maps the site's job category onto vocab.CategoryValues where it names one
// discipline. Titles are English here, so the dictionaries already read most of the rest;
// mediatekTechCategories marks the R&D categories that span several disciplines.
var mediatekCategories = map[string]string{
	"Chip Design":     "hardware",
	"Analog_RF":       "hardware",
	"Software":        "software_engineering",
	"SQA & Testing":   "qa",
	"Human Resources": "hr",
	"Finance":         "finance",
	"Legal & Patent":  "legal",
}

var mediatekTechCategories = map[string]bool{
	"Algorithm & Architecture": true,
	"System Application":       true,
	"Information Technology":   true,
}

// mediatekTaiwanSites are the Taiwanese site names the search states. HsinChu and Taipei read
// as places on their own; ChuPei (Zhubei) does not, so the country is stated for all three.
var mediatekTaiwanSites = map[string]bool{"HsinChu": true, "ChuPei": true, "Taipei": true}

var mediatekIDRE = regexp.MustCompile(`^[A-Z]{3,4}[0-9]+$`)

// NewMediaTek builds the MediaTek careers adapter over the given HTTP client.
func NewMediaTek(c mediatekHTTP) Source { return mediatek{http: c} }

func (mediatek) Provider() string { return "mediatek" }

// mediatek is single-company, so its config entry carries no board.
func (mediatek) boardless() {}

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (mediatek) fullBoardListing() {}

type mediatekSearchInput struct {
	Locale string `json:"locales"`
	Page   int    `json:"page"`
	Query  struct {
		Keywords []string `json:"keywords"`
		Relation string   `json:"relation"`
	} `json:"jobQueryInfo"`
	Filters struct {
		Categories      []string `json:"categorys"`
		WorkExperiences []string `json:"workExperiences"`
		Locations       []string `json:"locations"`
		Programs        []string `json:"programs"`
	} `json:"filters"`
	SortBy string `json:"sortBy"`
	Order  string `json:"order"`
	Limit  int    `json:"limit"`
}

type mediatekSearchResponse struct {
	Result struct {
		Data struct {
			JSON struct {
				Status     string        `json:"status"`
				Jobs       []mediatekJob `json:"jobs"`
				Pagination *struct {
					TotalPages int `json:"total_pages"`
					TotalItems int `json:"total_items"`
				} `json:"pagination"`
			} `json:"json"`
		} `json:"data"`
	} `json:"result"`
}

// mediatekLabel is one of the posting's tagged properties. The site is inconsistent about which
// half holds the name: the category carries it in `label`, the location in `code`.
type mediatekLabel struct {
	Label string `json:"label"`
	Code  string `json:"code"`
}

type mediatekJob struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"jobPostStatus"`
	Published   string `json:"publishedDate"`
	Properties  struct {
		Category mediatekLabel `json:"category"`
		Location mediatekLabel `json:"location"`
	} `json:"properties"`
}

func (s mediatek) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s mediatek) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var asked, read int
	jobs := fetchDetails(list, defaultDetailWorkers, func(p mediatekJob) (Job, bool) {
		if seen(p.ID) {
			return Job{ExternalID: p.ID, URL: mediatekJobURL + p.ID, Title: mediatekTitle(p.Title), Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, p)
	})
	for _, p := range list {
		if !seen(p.ID) {
			asked++
		}
	}
	for _, j := range jobs {
		if !j.SeenRefresh {
			read++
		}
	}
	if asked > 0 && read == 0 {
		return nil, fmt.Errorf("mediatek: none of %d new postings could be read", asked)
	}
	return jobs, nil
}

// list walks the search page by page until the reported page count, and fails unless it read
// exactly the reported total.
func (s mediatek) list(ctx context.Context) ([]mediatekJob, error) {
	var out []mediatekJob
	ids := map[string]bool{}
	total := -1
	for page := 1; ; page++ {
		if page > mediatekMaxPages {
			return nil, fmt.Errorf("mediatek: the search runs past %d pages", mediatekMaxPages)
		}
		var r mediatekSearchResponse
		if err := s.http.GetJSON(ctx, mediatekPageURL(page), &r); err != nil {
			return nil, fmt.Errorf("mediatek: search page %d: %w", page, err)
		}
		res := r.Result.Data.JSON
		if res.Status != "complete" || res.Pagination == nil {
			return nil, fmt.Errorf("mediatek: search page %d answered status %q", page, res.Status)
		}
		if total >= 0 && res.Pagination.TotalItems != total {
			return nil, fmt.Errorf("mediatek: the total moved from %d to %d mid-walk", total, res.Pagination.TotalItems)
		}
		total = res.Pagination.TotalItems
		for _, j := range res.Jobs {
			if !mediatekIDRE.MatchString(j.ID) || ids[j.ID] {
				return nil, fmt.Errorf("mediatek: posting id %q is malformed or listed twice", j.ID)
			}
			ids[j.ID] = true
			out = append(out, j)
		}
		if page >= res.Pagination.TotalPages {
			break
		}
	}
	switch {
	case total == 0:
		// The catalogue always holds postings; none is a changed search, and must not read as
		// every posting closed.
		return nil, fmt.Errorf("mediatek: the search reports no postings")
	case len(out) != total:
		return nil, fmt.Errorf("mediatek: read %d of %d postings", len(out), total)
	}
	return out, nil
}

// mediatekPageURL builds one unfiltered search page, newest first. The endpoint wants every
// filter present, as empty lists.
func mediatekPageURL(page int) string {
	in := mediatekSearchInput{Locale: "en_US", Page: page, SortBy: "publishedDate", Order: "DESC", Limit: mediatekPageSize}
	in.Query.Keywords, in.Query.Relation = []string{}, "AND"
	in.Filters.Categories, in.Filters.WorkExperiences = []string{}, []string{}
	in.Filters.Locations, in.Filters.Programs = []string{}, []string{}
	b, _ := json.Marshal(struct {
		JSON mediatekSearchInput `json:"json"`
	}{in})
	return mediatekSearchURL + url.QueryEscape(string(b))
}

// detail hydrates a new posting with the requirements from its page. A failed page defers the
// posting to the next crawl rather than storing it without them, since a stored posting is never
// hydrated again.
func (s mediatek) detail(ctx context.Context, e CompanyEntry, p mediatekJob) (Job, bool) {
	page, err := s.http.GetTextWithHeaders(ctx, mediatekJobURL+p.ID, mediatekLocaleCookie)
	if err != nil {
		log.Printf("mediatek: posting %s failed; deferring: %v", p.ID, err)
		return Job{}, false
	}
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		log.Printf("mediatek: posting %s page unparseable; deferring: %v", p.ID, err)
		return Job{}, false
	}
	if h1 := firstByTag(root, "h1"); h1 == nil || strings.TrimSpace(textContent(h1)) == "" {
		log.Printf("mediatek: posting %s page carries no posting; deferring", p.ID)
		return Job{}, false
	}
	title := mediatekTitle(p.Title)
	body := plainTextToHTML(p.Description) + plainTextToHTML(strings.Join(mediatekRequirements(root), "\n"))
	if title == "" || body == "" {
		return Job{}, false
	}
	category := strings.TrimSpace(p.Properties.Category.Label)
	site := strings.TrimSpace(p.Properties.Location.Code)
	job := Job{
		ExternalID:  p.ID,
		URL:         mediatekJobURL + p.ID,
		Title:       title,
		Company:     e.Company,
		Description: sanitizeHTML(body),
		Location:    site,
		PostedAt:    parseRFC3339OrDate(p.Published),
		Category:    mediatekCategories[category],
		IsTechHint:  mediatekTechCategories[category],
	}
	if mediatekTaiwanSites[site] {
		job.Countries = []string{"tw"}
	}
	return job, true
}

// mediatekTitle spaces out the underscore the site uses as a separator ("2027 Campus_IT
// Engineer", "Image processing algorithm engineer_ChuPei", "_5G Modem system validation
// engineer"), which the title dictionary would otherwise read as part of a word.
func mediatekTitle(raw string) string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '_' })
	for i, p := range parts {
		parts[i] = strings.Join(strings.Fields(p), " ")
	}
	return distinctJoin(parts, " - ", strings.TrimSpace)
}

// mediatekRequirements reads the requirement lines under the page's requirements heading: the
// items of the list that follows it. A posting without that section yields none.
func mediatekRequirements(root *html.Node) []string {
	var lines []string
	walk(root, func(n *html.Node) bool {
		if lines != nil || n.Type != html.ElementNode || n.Data != "h3" ||
			strings.TrimSpace(textContent(n)) != mediatekRequirementsHeading {
			return true
		}
		for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
			if sib.Type != html.ElementNode {
				continue
			}
			walk(sib, func(li *html.Node) bool {
				if li.Type == html.ElementNode && li.Data == "li" {
					if t := strings.TrimSpace(textContent(li)); t != "" {
						lines = append(lines, t)
					}
					return false
				}
				return true
			})
			break
		}
		return false
	})
	return lines
}
