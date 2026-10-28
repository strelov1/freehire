package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"
)

// hiringroom adapts HiringRoom, a Latin American ATS (Argentina, Peru, Chile, Ecuador, Panama):
// one <tenant>.hiringroom.com job portal per employer. A board is that tenant label.
//
// The portal page renders only its first 20 vacancies. Its own pager calls
// POST /jobs/getVacanciesForPortal/<page>, which answers JSON carrying the page's vacancy cards
// as an HTML fragment and the portal's total. Both portal layouts (the legacy /jobs page and
// the newer /portal) list through it. The cards carry no body, so this is a HydratingSource over
// the vacancy page.
type hiringroom struct {
	http hiringroomHTTP
}

type hiringroomHTTP interface {
	FormJSONPoster
	HTMLGetter
}

const (
	// hiringroomMaxPages bounds the walk; reaching it before the stated total fails the crawl.
	// The largest portal measured holds 241 vacancies (13 pages).
	hiringroomMaxPages = 200
)

// NewHiringRoom builds the HiringRoom adapter over the shared client.
func NewHiringRoom(c hiringroomHTTP) Source { return hiringroom{http: c} }

func (hiringroom) Provider() string { return "hiringroom" }

// fullBoardListing: every page states the portal's total, and the walk fails unless the
// distinct vacancies read equal it. Any page failing, a page that adds nothing before the total
// is reached, and the page cap all fail the crawl.
func (hiringroom) fullBoardListing() {}

// hiringroomPage is the pager's answer.
type hiringroomPage struct {
	Result string `json:"result"`
	Data   struct {
		HTMLContent string `json:"htmlContent"`
		Total       *int   `json:"total_vacancies"`
	} `json:"data"`
}

// hiringroomCard is one vacancy card of the listing.
type hiringroomCard struct {
	ID, Title, Location, Area string
	Schedule, Modality, Level string
}

var hiringroomBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (s hiringroom) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s hiringroom) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !hiringroomBoard.MatchString(board) {
		return nil, fmt.Errorf("hiringroom: board %q is not a tenant label", e.Board)
	}
	cards, err := s.list(ctx, board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, board, e, cards, seen)
}

func (s hiringroom) list(ctx context.Context, board string) ([]hiringroomCard, error) {
	var cards []hiringroomCard
	ids := map[string]bool{}
	for page := 1; ; page++ {
		if page > hiringroomMaxPages {
			return nil, fmt.Errorf("hiringroom: no end of the listing after %d pages", hiringroomMaxPages)
		}
		var resp hiringroomPage
		u := fmt.Sprintf("https://%s.hiringroom.com/jobs/getVacanciesForPortal/%d", board, page)
		form := url.Values{"selectedPage": {strconv.Itoa(page)}, "typePortal": {"external"}}
		if err := s.http.PostFormJSON(ctx, u, form, &resp); err != nil {
			return nil, fmt.Errorf("hiringroom: page %d: %w", page, err)
		}
		if resp.Result != "success" || resp.Data.Total == nil {
			return nil, fmt.Errorf("hiringroom: page %d answered %q with no total", page, resp.Result)
		}
		got, err := parseHiringRoomCards(resp.Data.HTMLContent)
		if err != nil {
			return nil, fmt.Errorf("hiringroom: page %d: %w", page, err)
		}
		added := 0
		for _, c := range got {
			if !ids[c.ID] {
				ids[c.ID] = true
				cards = append(cards, c)
				added++
			}
		}
		total := *resp.Data.Total
		if len(cards) >= total {
			if len(cards) != total {
				return nil, fmt.Errorf("hiringroom: read %d vacancies, the portal states %d", len(cards), total)
			}
			return cards, nil
		}
		if added == 0 {
			return nil, fmt.Errorf("hiringroom: page %d added no vacancy (%d of %d)", page, len(cards), total)
		}
	}
}

var hiringroomVacancyPath = regexp.MustCompile(`^/jobs/get_vacancy/([0-9a-f]{24})$`)

// parseHiringRoomCards reads the vacancy cards of one listing page: an <a> linking a vacancy,
// wrapping the title (h4.name__vacancy) and icon-tagged facts. Each card renders its tags twice
// (desktop and mobile copies); the first copy is read.
func parseHiringRoomCards(fragment string) ([]hiringroomCard, error) {
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil, err
	}
	var out []hiringroomCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		m := hiringroomVacancyPath.FindStringSubmatch(Attr(n, "href"))
		if m == nil {
			return true
		}
		c := hiringroomCard{ID: m[1]}
		if t := firstByClass(n, "name__vacancy"); t != nil {
			c.Title = strings.Join(strings.Fields(textContent(t)), " ")
		}
		walk(n, func(i *html.Node) bool {
			if i.Type != html.ElementNode || i.Data != "i" || i.Parent == nil {
				return true
			}
			text := strings.Join(strings.Fields(textContent(i.Parent)), " ")
			for _, set := range []struct {
				icon string
				dst  *string
			}{
				{"hr-Location-pin", &c.Location}, {"hr-Work-area", &c.Area}, {"hr-Clock", &c.Schedule},
				{"hr-Company", &c.Modality}, {"hr-Remote", &c.Modality}, {"hr-Campus", &c.Level},
			} {
				if HasClass(i, set.icon) && *set.dst == "" {
					*set.dst = text
				}
			}
			return true
		})
		if c.Title != "" {
			out = append(out, c)
		}
		return false
	})
	return out, nil
}

// hydrate reads the vacancy page of each vacancy the catalogue lacks. A page that could not be
// read is deferred to a later run rather than stored body-less; a crawl that listed new vacancies
// and read none of them fails.
//
// A page the platform answers 404/410 for is not unreadable but unpublished: a tenant can switch
// its public portal off (its /jobs then 404s, /portal still redirects there) while the pager
// keeps listing every vacancy, and each vacancy page 404s. Measured 2026-10-28 on 71 of 317
// Argentine and Latin American tenants. Those vacancies are skipped and do not count towards the
// "read none" failure — the platform has answered that there is nothing public to read.
func (s hiringroom) hydrate(ctx context.Context, board string, e CompanyEntry, cards []hiringroomCard, seen func(string) bool) ([]Job, error) {
	var wanted, read atomic.Int64
	jobs := fetchDetails(cards, defaultDetailWorkers, func(c hiringroomCard) (Job, bool) {
		link := "https://" + board + ".hiringroom.com/jobs/get_vacancy/" + c.ID
		if seen(c.ID) {
			return Job{ExternalID: c.ID, URL: link, Title: c.Title, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		root, err := s.http.GetHTML(ctx, link)
		if err != nil {
			if !detailUnreadable(err) {
				wanted.Add(-1) // unpublished, not unreadable (see above)
				return Job{}, false
			}
			log.Printf("hiringroom: %s: %v; deferring", link, err)
			return Job{}, false
		}
		desc, ok := hiringroomDescription(root)
		if !ok {
			log.Printf("hiringroom: %s carries no vacancy; deferring", link)
			return Job{}, false
		}
		read.Add(1)
		return hiringroomJob(c, link, desc, e), true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("hiringroom: listed %d new vacancies and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// hiringroomDescription joins the vacancy page's sections — each an h6 heading followed by a
// div.job-description-content ("Descripción del puesto", "Requisitos", "Beneficios"). ok is false
// when the page has no such section at all, i.e. it is not a vacancy page; a page whose sections
// are all empty (a tenant's catch-all "Postulación en base" vacancy) is read, with no body.
func hiringroomDescription(root *html.Node) (string, bool) {
	var b strings.Builder
	found := false
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "div" || !HasClass(n, "job-description-content") {
			return true
		}
		found = true
		body := innerHTML(n)
		if strings.TrimSpace(body) == "" {
			return false
		}
		for p := n.PrevSibling; p != nil; p = p.PrevSibling {
			if p.Type == html.ElementNode {
				if p.Data == "h6" {
					b.WriteString("<h3>" + html.EscapeString(strings.Join(strings.Fields(textContent(p)), " ")) + "</h3>")
				}
				break
			}
		}
		b.WriteString(body)
		return false
	})
	if !found {
		return "", false
	}
	return sanitizeHTML(b.String()), true
}

// hiringroomJob maps a card plus its body. The tags are the platform's own picklists.
func hiringroomJob(c hiringroomCard, link, desc string, e CompanyEntry) Job {
	job := Job{
		ExternalID:  c.ID,
		URL:         link,
		Title:       c.Title,
		Company:     e.Company,
		Location:    c.Location,
		Description: desc,
	}
	switch c.Schedule {
	case "Full-time":
		job.EmploymentType = "full_time"
	case "Part time":
		job.EmploymentType = "part_time"
	case "Freelance", "Eventual":
		job.EmploymentType = "contract"
	case "Pasantía":
		job.EmploymentType = "internship"
	}
	switch c.Modality {
	case "Remoto":
		job.WorkMode, job.Remote = "remote", true
	case "Híbrido":
		job.WorkMode = "hybrid"
	case "Presencial":
		job.WorkMode = "onsite"
	}
	// The management levels (Jefe, Gerencia, Dirección, ...) name a role, not a seniority.
	switch c.Level {
	case "Junior", "Primer empleo":
		job.Seniority = "junior"
	case "Semi Senior":
		job.Seniority = "middle"
	case "Senior":
		job.Seniority = "senior"
	}
	// The area is the platform's own taxonomy, "<area> / <sub-area>"; its technology area is a
	// structured statement that the vacancy is technical.
	job.IsTechHint = strings.HasPrefix(c.Area, "Tecnología, Sistemas y Telecomunicaciones")
	return job
}
