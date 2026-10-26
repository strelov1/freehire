package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// asus adapts ASUS's own recruitment site (recruit.asus.com), the Taiwan headquarters' catalogue
// with its offices abroad. Single-company and boardless. ASUS's US arm hires through its Jobvite
// tenant, which the jobvite adapter already crawls.
//
// The site is server-rendered ASP.NET with no JSON listing: /Jobs pages five postings at a time
// (688 postings, 138 pages, on 2026-10-25) and ignores every page-size parameter tried. The
// walk reads pages until one comes back empty. A posting page carries the duties, the
// requirements and the posting's facets, so a posting the catalogue does not hold yet is
// hydrated from it (HydratingSource); a seen one costs no request.
//
// The site is bilingual, but the language is a session cookie the server mints and the URL
// cannot select, so the crawl reads the default Traditional Chinese.
//
// fullBoardListing: the walk ends only on a page with no rows; any listing page failing, or the
// walk reaching asusMaxPages first, fails the crawl.
type asus struct {
	http HTMLGetter
}

const (
	asusListURL = "https://recruit.asus.com/Jobs?page="
	asusJobURL  = "https://recruit.asus.com/Jobs/Detail?sn="
	// asusMaxPages is a ceiling, not a stopping rule; a catalogue that outgrows it fails the
	// crawl.
	asusMaxPages = 400
)

// asusCategories maps the first two levels of the site's job category ("研究發展/ SW/ ...")
// onto vocab.CategoryValues where they name one discipline; asusTechCategories marks the R&D
// branches that span several. The rest (industrial design, engineering services, purchasing,
// quality, market development PM) are left to the dictionaries.
var asusCategories = map[string]string{
	"研究發展/SW":    "software_engineering",
	"研究發展/HW":    "hardware",
	"研究發展/RF":    "hardware",
	"研究發展/EMI":   "hardware",
	"研究發展/BIOS":  "embedded",
	"研究發展/韌體設計":  "embedded", // firmware design
	"研究發展/UI":    "design",
	"研究發展/ME":    "engineering_design", // mechanical engineering
	"管理支援/HR":    "hr",
	"管理支援/財會":    "finance",
	"管理支援/法務":    "legal",
	"管理支援/客服":    "support",
	"管理支援/RD PM": "project_management",
	"管理支援/SW PM": "project_management",
	"業務行銷/行銷企劃":  "marketing",
}

var asusTechCategories = map[string]bool{
	"研究發展/Data": true, // data engineering and science
	"研究發展/電子商務": true, // e-commerce platform R&D
	"管理支援/MIS":  true, // corporate IT
}

// asusEmploymentTypes maps the posting's 性質 (nature of employment).
var asusEmploymentTypes = map[string]string{
	"全職": "full_time",
	"兼職": "part_time",
	"工讀": "part_time",
	"約聘": "contract",
	"實習": "internship",
}

var (
	asusSNRE = regexp.MustCompile(`[?&]sn=([0-9a-fA-F-]{36})\b`)
	// asusJobCodeRE is the requisition code the site prefixes every title with ("RD11482 Brand
	// Design ..."), which the title dictionary would read as a word.
	asusJobCodeRE = regexp.MustCompile(`^[A-Z]{1,4}[0-9]{3,}\s+`)
)

// NewASUS builds the ASUS recruitment-site adapter over the given HTTP client.
func NewASUS(c HTMLGetter) Source { return asus{http: c} }

func (asus) Provider() string { return "asus" }

// asus is single-company, so its config entry carries no board.
func (asus) boardless() {}

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (asus) fullBoardListing() {}

// asusListed is one listing row: the posting's id and title, all a seen posting needs.
type asusListed struct {
	SN    string
	Title string
}

func (s asus) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s asus) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	asked := 0
	for _, p := range list {
		if !seen(p.SN) {
			asked++
		}
	}
	jobs := fetchDetails(list, defaultDetailWorkers, func(p asusListed) (Job, bool) {
		if seen(p.SN) {
			return Job{ExternalID: p.SN, URL: asusJobURL + p.SN, Title: p.Title, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, p)
	})
	if asked > 0 && len(jobs) == len(list)-asked {
		return nil, fmt.Errorf("asus: none of %d new postings could be read", asked)
	}
	return jobs, nil
}

// list walks the listing until a page with no rows. A posting that shifts across a page
// boundary mid-walk can appear twice and is kept once.
func (s asus) list(ctx context.Context) ([]asusListed, error) {
	var out []asusListed
	ids := map[string]bool{}
	for page := 1; ; page++ {
		if page > asusMaxPages {
			return nil, fmt.Errorf("asus: the listing runs past %d pages", asusMaxPages)
		}
		root, err := s.http.GetHTML(ctx, asusListURL+strconv.Itoa(page))
		if err != nil {
			return nil, fmt.Errorf("asus: listing page %d: %w", page, err)
		}
		rows := asusRows(root)
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if !ids[r.SN] {
				ids[r.SN] = true
				out = append(out, r)
			}
		}
	}
	if len(out) == 0 {
		// The board always carries postings; none is a changed page, and must not read as every
		// posting closed.
		return nil, fmt.Errorf("asus: the listing carries no postings")
	}
	return out, nil
}

// asusRows reads a listing page's rows: each <tr> holding a posting link and a title cell.
func asusRows(root *html.Node) []asusListed {
	var out []asusListed
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "tr" {
			return true
		}
		var r asusListed
		walk(n, func(c *html.Node) bool {
			if c.Type != html.ElementNode {
				return true
			}
			switch {
			case c.Data == "a" && r.SN == "":
				r.SN = firstSubmatch(asusSNRE, Attr(c, "href"))
			case c.Data == "td" && Attr(c, "data-label") == "職缺名稱":
				r.Title = asusTitle(textContent(c))
			}
			return true
		})
		if r.SN != "" && r.Title != "" {
			out = append(out, r)
		}
		return false
	})
	return out
}

// detail hydrates a new posting from its page. A failed page defers the posting to the next
// crawl; the site answers an unknown id with a 500, so a withdrawn posting lands here too and is
// retried until the listing drops it.
func (s asus) detail(ctx context.Context, e CompanyEntry, p asusListed) (Job, bool) {
	root, err := s.http.GetHTML(ctx, asusJobURL+p.SN)
	if err != nil {
		log.Printf("asus: posting %s failed; deferring: %v", p.SN, err)
		return Job{}, false
	}
	head, text := firstByClass(root, "Detail-Title"), firstByClass(root, "Detail-InfoText")
	if head == nil || text == nil {
		log.Printf("asus: posting %s page carries no posting; deferring", p.SN)
		return Job{}, false
	}
	title := asusTitle(textContent(head))
	body := sanitizeHTML(innerHTML(text))
	if title == "" || body == "" {
		return Job{}, false
	}
	cells := asusCells(root)
	location, countries := twLocation(strings.Split(cells["工作地區"], "/")...)
	category := asusCategoryKey(cells["職務類別"])
	return Job{
		ExternalID:     p.SN,
		URL:            asusJobURL + p.SN,
		Title:          title,
		Company:        e.Company,
		Description:    body,
		Location:       location,
		Countries:      countries,
		Category:       asusCategories[category],
		IsTechHint:     asusTechCategories[category],
		EmploymentType: asusEmploymentTypes[cells["性質"]],
	}, true
}

// asusCells reads the posting page's labelled facet cells (data-label → text).
func asusCells(root *html.Node) map[string]string {
	out := map[string]string{}
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "td" {
			if label := Attr(n, "data-label"); label != "" {
				out[label] = strings.Join(strings.Fields(textContent(n)), " ")
			}
			return false
		}
		return true
	})
	return out
}

// asusCategoryKey reduces a category path ("研究發展/ SW/ 系統軟體") to its first two levels
// with the padding removed ("研究發展/SW").
func asusCategoryKey(path string) string {
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}

func asusTitle(s string) string {
	return asusJobCodeRE.ReplaceAllString(strings.Join(strings.Fields(s), " "), "")
}
