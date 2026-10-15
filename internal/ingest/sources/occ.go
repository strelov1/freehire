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

	xhtml "golang.org/x/net/html"
)

// occ adapts OCC Mundial (occ.com.mx), Mexico's dominant general job board, whose postings each
// name their own employer. Board-based aggregator: a board is one of the site's own 23 job
// CATEGORIES ("tecnologias-de-la-informacion-sistemas" held 4,361 postings on 2026-10-15). The
// catalogue is broader than IT, so every category is a board; only the IT one hints its
// postings technical (occITBoard), the rest are left to the classifier.
//
// The listing is the server-rendered category search (/empleos/trabajo-en-<board>/?page=N, 20
// cards a page). Since 2026-10 it sits behind a Cloudflare challenge to datacenter IPs, so in
// production it is read through the Scrapling tier's browser (scraplingtier.go); the posting
// JSON is not challenged and keeps a plain client (occSplit). The posting itself is the JSON the site's own split view loads from
// oferta.occ.com.mx, so this is a HydratingSource that buys one small JSON document only for a
// posting the catalogue lacks.
//
// The walk follows the ordinary "which page failed decides" rule and claims no
// fullBoardListing: a full walk read 4,360 distinct postings against a stated 4,361, so the
// stated total cannot serve as the proof. See the OCC traps in AGENTS.md.
type occ struct {
	http occHTTP
	// maxNewPerBoard bounds how many new postings one board hydrates per run; non-positive
	// takes occMaxNewPerBoard.
	maxNewPerBoard int
}

// occSplit serves the listing and the posting JSON from different clients.
type occSplit struct {
	HTMLGetter
	JSONGetter
}

type occHTTP interface {
	HTMLGetter
	JSONGetter
}

const (
	occBase       = "https://www.occ.com.mx"
	occListURL    = occBase + "/empleos/trabajo-en-%s/"
	occDetailURL  = "https://oferta.occ.com.mx/offer/%s/d/j?ipo=41&iapo=1"
	occMaxPages   = 1000 // five times the largest measured board, never the expected end
	occBlindLabel = "Empresa confidencial"
	// occITBoard is the one category whose postings the site itself files as technical.
	occITBoard = "tecnologias-de-la-informacion-sistemas"
	// occMaxNewPerBoard bounds one board's posting-JSON requests per run. The first run over 23
	// categories meets every posting as new; a board writes nothing until its crawl returns, so
	// the rest are deferred — not yielded, so still new — to the next run.
	occMaxNewPerBoard = 2000
)

var (
	// occBoardRE is a category slug as the site spells it in its own category links.
	occBoardRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	occIDRE    = regexp.MustCompile(`^[0-9]+$`)
	// occPostingPathRE is the canonical posting path the detail states (its query is tracking).
	occPostingPathRE = regexp.MustCompile(`^/empleo/oferta/[0-9]+-[A-Za-z0-9-]+$`)
)

func NewOCC(c occHTTP) Source { return occ{http: c} }

func (occ) Provider() string { return "occ" }

// Every posting names its own employer, so occ stays in the source facet and in the
// cross-source duplicate suppression set.
func (occ) aggregator() {}

// occCard is what a listing card states about a posting.
type occCard struct {
	ID      string
	Title   string
	Company string
}

func (s occ) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s occ) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	if !occBoardRE.MatchString(e.Board) {
		return nil, fmt.Errorf("occ: board %q is not a category slug", e.Board)
	}
	cards, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, e.Board, cards, seen)
}

// list walks the category until a page adds no new card. The site's "premium" cards repeat at
// the top of every page, so a page past the end is not empty of cards but empty of NEW ones —
// which is exactly what ends the walk.
func (s occ) list(ctx context.Context, board string) ([]occCard, error) {
	byID := map[string]occCard{}
	pageURL := func(page int) string {
		u := fmt.Sprintf(occListURL, board)
		if page == 1 {
			return u
		}
		return u + "?page=" + strconv.Itoa(page)
	}
	ids, err := crawlPagedLinks(ctx, s.http, occMaxPages, pageURL, func(root *xhtml.Node) []string {
		var out []string
		for _, c := range parseOCCCards(root) {
			byID[c.ID] = c
			out = append(out, c.ID)
		}
		return out
	})
	if err != nil {
		return nil, fmt.Errorf("occ: board %q: listing: %w", board, err)
	}
	// A category always lists postings; a first page without a card is a markup change or a
	// refusal page, never an empty market.
	if len(ids) == 0 {
		return nil, fmt.Errorf("occ: board %q: listing page 1 carries no job cards", board)
	}
	out := make([]occCard, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out, nil
}

// parseOCCCards reads every li.vacancy-card of a listing page: its data-id, its title and the
// employer the card names (empty on the card layout that draws the employer elsewhere).
func parseOCCCards(root *xhtml.Node) []occCard {
	var out []occCard
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "li" || !HasClass(n, "vacancy-card") {
			return true
		}
		id := strings.TrimSpace(Attr(n, "data-id"))
		if !occIDRE.MatchString(id) {
			return false
		}
		c := occCard{ID: id}
		if p := firstByClass(n, "vacancy-card__company-text"); p != nil {
			c.Company = strings.Join(strings.Fields(textContent(p)), " ")
		}
		if p := firstByClass(n, "vacancy-card__title"); p != nil {
			c.Title = strings.Join(strings.Fields(textContent(p)), " ")
		}
		out = append(out, c)
		return false
	})
	return out
}

// hydrate buys the posting JSON only for a posting the catalogue lacks. A posting whose card
// hides the employer ("Empresa confidencial", ~8% of cards) is skipped before any request: it
// names no employer to file it under. A detail that could not be read defers the posting to a
// later run rather than storing it body-less.
func (s occ) hydrate(ctx context.Context, board string, cards []occCard, seen func(string) bool) ([]Job, error) {
	budget := s.maxNewPerBoard
	if budget <= 0 {
		budget = occMaxNewPerBoard
	}
	// The budget is taken in listing order before the fan-out, so which postings wait is
	// decided by the listing and not by which worker ran first.
	var picked []occCard
	fresh, deferred := 0, 0
	for _, c := range cards {
		if strings.EqualFold(c.Company, occBlindLabel) {
			continue
		}
		if !seen(c.ID) {
			if fresh == budget {
				deferred++
				continue
			}
			fresh++
		}
		picked = append(picked, c)
	}
	if deferred > 0 {
		log.Printf("occ: board %q: hydration budget %d reached; %d new postings deferred to a later run", board, budget, deferred)
	}
	tech := board == occITBoard
	var wanted, read atomic.Int64
	jobs := fetchDetails(picked, defaultDetailWorkers, func(c occCard) (Job, bool) {
		if seen(c.ID) {
			return Job{ExternalID: c.ID, Title: c.Title, Company: c.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		var d occDetail
		if err := s.http.GetJSON(ctx, fmt.Sprintf(occDetailURL, c.ID), &d); err != nil {
			if detailUnreadable(err) {
				log.Printf("occ: detail %s failed; deferring: %v", c.ID, err)
			}
			return Job{}, false
		}
		job, ok := d.toJob(c.ID, tech)
		if !ok {
			log.Printf("occ: detail %s carries no usable posting; deferring", c.ID)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("occ: listed %d new postings and read none of them", wanted.Load())
	}
	return jobs, nil
}

// occDetail is the posting JSON (oferta.occ.com.mx/offer/<id>/d/j). Its keys are the site's own
// abbreviations; only the mapped ones are named.
type occDetail struct {
	O struct {
		ID          string `json:"eoi"`
		Title       string `json:"t"`
		Company     string `json:"cn"`
		Description string `json:"ld"`
		Posted      string `json:"pt"`
		City        string `json:"c"`
		State       string `json:"l"`
		SalaryMin   int    `json:"smin"`
		SalaryMax   int    `json:"smax"`
		SalaryText  string `json:"lss"`
		WorkTypeURL string `json:"uwt"`
		EmployURL   string `json:"uet"`
		PostingPath string `json:"ur"`
		ExternalURL string `json:"uaext"`
	} `json:"o"`
	C struct {
		Company string `json:"cn"`
	} `json:"c"`
}

func (d occDetail) toJob(id string, tech bool) (Job, bool) {
	o := d.O
	title := strings.Join(strings.Fields(o.Title), " ")
	company := strings.Join(strings.Fields(d.C.Company), " ")
	if company == "" {
		company = strings.Join(strings.Fields(o.Company), " ")
	}
	description := sanitizeHTML(o.Description)
	if o.ID != id || title == "" || company == "" || strings.EqualFold(company, occBlindLabel) ||
		strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	workMode := occWorkModes[o.WorkTypeURL]
	job := Job{
		ExternalID:     id,
		URL:            occPostingURL(id, o.PostingPath, o.ExternalURL),
		Title:          title,
		Company:        company,
		Location:       distinctJoin([]string{o.City, o.State}, ", ", strings.TrimSpace),
		Description:    description,
		PostedAt:       NotFuture(parseDate(occDay(o.Posted))),
		WorkMode:       workMode,
		Remote:         workMode == "remote",
		EmploymentType: occEmploymentTypes[o.EmployURL],
		IsTechHint:     tech,
	}
	occSalary(&job, o.SalaryMin, o.SalaryMax, o.SalaryText)
	return job, true
}

// occPostingURL is the employer's own apply link when the posting states one, else the canonical
// OCC posting page (the detail's path without its tracking query, or the bare id route).
func occPostingURL(id, path, external string) string {
	if u, err := url.Parse(strings.TrimSpace(external)); err == nil && validHTTPURL(external) &&
		!strings.HasSuffix(strings.ToLower(u.Hostname()), "occ.com.mx") {
		return strings.TrimSpace(external)
	}
	if p, _, _ := strings.Cut(path, "?"); occPostingPathRE.MatchString(p) {
		return occBase + p
	}
	return occBase + "/empleo/oferta/" + id + "/"
}

func occDay(s string) string {
	if len(s) < len("2006-01-02") {
		return ""
	}
	return s[:len("2006-01-02")]
}

// occWorkModes keys the site's work-arrangement filter link, which is what the posting states.
var occWorkModes = map[string]string{
	"/empleos/tipo-en-oficina":         "onsite",
	"/empleos/tipo-home-office-mixto":  "hybrid",
	"/empleos/tipo-home-office-remoto": "remote",
}

// occEmploymentTypes keys the site's schedule filter link, the two values seen on 162 sampled
// postings. "Honorarios" (a fee-based contract) is filed under either of them by the site, so
// the label beside the link is not read.
var occEmploymentTypes = map[string]string{
	"/empleos/tiempo-completo": "full_time",
	"/empleos/medio-tiempo":    "part_time",
}

// occSalary takes the posting's range when the site displays one. The display ("$45,000 -
// $50,000 Mensual") is the only place the period is stated and names no currency; every figure
// is in pesos on this Mexican board, and a hidden salary reads "Sueldo no mostrado por la
// empresa" with zero bounds.
func occSalary(job *Job, lo, hi int, display string) {
	display = strings.TrimSpace(display)
	if !strings.HasPrefix(display, "$") || !strings.HasSuffix(display, "Mensual") || (lo <= 0 && hi <= 0) {
		return
	}
	if lo > 0 {
		job.SalaryMin = &lo
	}
	if hi > 0 {
		job.SalaryMax = &hi
	}
	job.SalaryCurrency, job.SalaryPeriod = "MXN", "month"
}
