package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// posaoHR adapts posao.hr, a Croatian general job board: ~570 postings employers and staffing
// agencies publish on the site itself, every trade, many of them abroad (Austria, Germany). The
// site also republishes the public employment service's vacancies, which the hzz adapter reads
// at their source, so those are skipped here. It is boardless and an aggregator — the employer is
// read per posting. See the "posao.hr traps" section of AGENTS.md for the measurements behind each
// rule.
//
// The listing is the site's county pages, server-rendered HTML, fifteen cards a page, the
// site's own postings ahead of the republished ones; a card carries the title, the employer and
// the place, never the body. The body is bought from the posting page for postings the catalogue
// lacks, within a per-run budget. A posting filed under a Croatian county and not under "abroad"
// is in Croatia, which is the structured country.
type posaoHR struct {
	http           posaoHRGetter
	limiter        waiter
	counties       []string
	maxNewPerRun   int
	maxPages       int
	deadlineMargin time.Duration
}

// posaoHRGetter is the transport the adapter needs: pages as HTML trees, a posting page with the
// address it was finally answered from (a gone posting redirects to the home page). The site
// keeps the display language in the session, so the client must keep cookies (see the registry).
type posaoHRGetter interface {
	HTMLGetter
	HTMLResolvedGetter
}

const (
	posaoHRBaseURL = "https://www.posao.hr"
	// posaoHRLanguageURL switches the session's display language. Without it the site picks one
	// by the visitor's address — English from a Brazilian one, so German is likely from the
	// crawl hosts — and the labels the body is built from would follow.
	posaoHRLanguageURL = posaoHRBaseURL + "/trebam/promijeniti-jezik/hr"
	// posaoHRAbroad is the county slice of postings outside Croatia.
	posaoHRAbroad = "inozemstvo"
	// posaoHRMaxPages is a safety ceiling on one county's walk: the largest (Grad Zagreb) took 16
	// pages to reach its republished tail on 2026-10-29. Reaching it fails the crawl.
	posaoHRMaxPages = 60
	// posaoHRRequestInterval paces every request of a run at one a second — a courtesy ceiling:
	// two clients reading back to back drew no refusal.
	posaoHRRequestInterval = time.Second
	posaoHRDetailWorkers   = 2
	// posaoHRMaxNewPerRun bounds the posting pages one run reads: the listing is ~95 pages, and
	// 600 posting pages at one a second is ten minutes, well inside the scheduler's crawl budget.
	// The whole catalogue (~570) fits one run.
	posaoHRMaxNewPerRun   = 600
	posaoHRDeadlineMargin = 5 * time.Minute
)

// posaoHRCounties are the site's county slices: Croatia's twenty counties, the City of Zagreb and
// "abroad". A posting filed under several counties, or under "all of Croatia", is listed in each.
var posaoHRCounties = []string{
	"bjelovarsko-bilogorska", "brodsko-posavska", "dubrovacko-neretvanska", "istarska",
	"karlovacka", "koprivnicko-krizevacka", "krapinsko-zagorska", "licko-senjska", "medimurska",
	"osjecko-baranjska", "pozesko-slavonska", "primorsko-goranska", "sibensko-kninska",
	"sisacko-moslavacka", "splitsko-dalmatinska", "varazdinska", "viroviticko-podravska",
	"vukovarsko-srijemska", "zadarska", "zagreb", "zagrebacka", posaoHRAbroad,
}

// NewPosaoHR builds the adapter over the given client, which must keep cookies.
func NewPosaoHR(c posaoHRGetter) Source {
	return posaoHR{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(posaoHRRequestInterval), 1),
		counties:       posaoHRCounties,
		maxNewPerRun:   posaoHRMaxNewPerRun,
		maxPages:       posaoHRMaxPages,
		deadlineMargin: posaoHRDeadlineMargin,
	}
}

func (posaoHR) Provider() string { return "posaohr" }

// One national board, no per-tenant id.
func (posaoHR) boardless() {}

// Hundreds of employers and agencies post here, each named per posting.
func (posaoHR) aggregator() {}

// posaoHRCard is one listing card: what the gate and the job need before the page is bought.
type posaoHRCard struct {
	id       int64
	url      string
	title    string
	company  string
	place    string
	domestic bool // listed under a Croatian county
	abroad   bool // listed under "abroad"
}

func (c posaoHRCard) externalID() string { return strconv.FormatInt(c.id, 10) }

// posaoHRPostingPath is a posting path: a slug and the posting id.
var posaoHRPostingPath = regexp.MustCompile(`^/oglasi/[^/]+/(\d+)/$`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s posaoHR) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s posaoHR) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks every county, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest first. A posting the pipeline will discard anyway is judged on
// its card and yielded body-less without a request. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s posaoHR) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	if _, err := s.http.GetHTML(ctx, posaoHRLanguageURL); err != nil {
		return nil, fmt.Errorf("posaohr: language switch: %w", err)
	}
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, c.company)
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []posaoHRCard
	for _, c := range cards {
		base := posaoHRBaseJob(c)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[c.company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		unseen = append(unseen, c)
	}
	// Newest first: ids grow with publication.
	sort.SliceStable(unseen, func(i, j int) bool { return unseen[i].id > unseen[j].id })
	deferred := 0
	if len(unseen) > s.maxNewPerRun {
		deferred = len(unseen) - s.maxNewPerRun
		unseen = unseen[:s.maxNewPerRun]
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, posaoHRDetailWorkers, func(c posaoHRCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("posaohr: posting %d failed; deferring: %v", c.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("posaohr: requested %d posting pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("posaohr: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("posaohr: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns.
func (s posaoHR) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks every county and merges the postings, which a multi-county posting repeats, in
// first-seen order. Any county failing fails the crawl: a partial listing would let the
// company-scoped sweep close the postings of the employers it did see in the counties it did not.
func (s posaoHR) list(ctx context.Context) ([]posaoHRCard, error) {
	var out []posaoHRCard
	index := make(map[int64]int)
	for _, county := range s.counties {
		cards, err := s.walk(ctx, county)
		if err != nil {
			return nil, err
		}
		for _, c := range cards {
			i, ok := index[c.id]
			if !ok {
				i = len(out)
				index[c.id] = i
				out = append(out, c)
			}
			if county == posaoHRAbroad {
				out[i].abroad = true
			} else {
				out[i].domestic = true
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("posaohr: the listing named no posting")
	}
	return out, nil
}

// walk reads one county's own postings: its pages up to the first that carries a republished
// employment-service card, which the site sorts after every posting of its own, or to the
// county's last page. A page without the pager is not a listing and fails the walk, as does a
// cardless page short of the last one.
func (s posaoHR) walk(ctx context.Context, county string) ([]posaoHRCard, error) {
	var out []posaoHRCard
	for page := 1; ; page++ {
		if page > s.maxPages {
			return nil, fmt.Errorf("posaohr: county %s runs past %d pages", county, s.maxPages)
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		u := posaoHRBaseURL + "/zupanije/" + county + "/"
		if page > 1 {
			u += "stranica/" + strconv.Itoa(page)
		}
		root, err := s.http.GetHTML(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("posaohr: county %s page %d: %w", county, page, err)
		}
		pages, ok := posaoHRPages(root)
		if !ok {
			return nil, fmt.Errorf("posaohr: county %s page %d is not a listing", county, page)
		}
		cards, republished := posaoHRCards(root)
		if len(cards) == 0 && !republished && page < pages {
			return nil, fmt.Errorf("posaohr: county %s page %d of %d is empty", county, page, pages)
		}
		out = append(out, cards...)
		if republished || page >= pages {
			return out, nil
		}
	}
}

// posaoHRPages reads the page count the listing's pager states ("Stranica 1 od ukupno 21"); ok is
// false for a page that is not a listing.
func posaoHRPages(root *html.Node) (int, bool) {
	pager := firstByClass(root, "pagination")
	if pager == nil {
		return 0, false
	}
	span := firstByTag(pager, "span")
	if span == nil {
		return 0, false
	}
	var last string
	walk(span, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "strong" {
			last = textContent(n)
			return false
		}
		return true
	})
	n, err := strconv.Atoi(last)
	return n, err == nil && n > 0
}

// posaoHRCards reads the site's own posting cards of one listing page, and reports whether the
// page also carries a republished employment-service card. A card is an anchor wrapping a
// div.search_grid; the employer carousel above the list links postings too, but not that way.
func posaoHRCards(root *html.Node) (cards []posaoHRCard, republished bool) {
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		grid := posaoHRFirstElementChild(n)
		if grid == nil || grid.Data != "div" || !HasClass(grid, "search_grid") {
			return true
		}
		if posaoHRRepublishedCard(grid) {
			republished = true
			return false
		}
		if c, ok := posaoHRCardFrom(n, grid); ok {
			cards = append(cards, c)
		}
		return false
	})
	return cards, republished
}

func posaoHRFirstElementChild(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

// posaoHRRepublishedCard reports whether a card is an employment-service (HZZ) vacancy: its title
// carries the marker as an <em> ("(HZZ oglas)", "(HZZ job ad)" in English).
func posaoHRRepublishedCard(grid *html.Node) bool {
	title := firstByClass(grid, "title")
	if title == nil {
		return false
	}
	em := firstByTag(title, "em")
	return em != nil && strings.Contains(textContent(em), "HZZ")
}

func posaoHRCardFrom(a, grid *html.Node) (posaoHRCard, bool) {
	u, err := url.Parse(strings.TrimSpace(Attr(a, "href")))
	if err != nil || u.Host != "www.posao.hr" {
		return posaoHRCard{}, false
	}
	m := posaoHRPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return posaoHRCard{}, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return posaoHRCard{}, false
	}
	c := posaoHRCard{id: id, url: posaoHRBaseURL + u.Path}
	if t := firstByClass(grid, "title"); t != nil {
		c.title = posaoHRClean(textContent(t))
	}
	if e := firstByClass(grid, "employer"); e != nil {
		c.company = posaoHRClean(textContent(e))
	}
	if l := firstByClass(grid, "location"); l != nil {
		c.place = posaoHRCardPlace(textContent(l))
	}
	if c.title == "" || c.company == "" {
		return posaoHRCard{}, false
	}
	return c, true
}

func posaoHRClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// posaoHRCardPlace cleans a card's place, which the card cuts at ~30 characters with "...": the
// cut and the separator before it are dropped. A hydrated posting takes its page's whole place.
func posaoHRCardPlace(s string) string {
	s = posaoHRClean(s)
	if t, cut := strings.CutSuffix(s, "..."); cut {
		s = strings.TrimRight(t, " ,/-")
	}
	return s
}

// posaoHRBaseJob is the body-less Job a card alone supports: the identity, and the facts the gate
// judges on. A posting the site files under a Croatian county and not abroad is in Croatia; one
// filed under both, or abroad only, states its country in its place, which the location
// dictionary reads.
func posaoHRBaseJob(c posaoHRCard) Job {
	j := Job{
		ExternalID: c.externalID(),
		URL:        c.url,
		Title:      c.title,
		Company:    c.company,
		Location:   c.place,
	}
	if c.domestic && !c.abroad {
		j.Countries = []string{"hr"}
	}
	return j
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410, or a redirect away from the posting); an error is the absence of an answer.
func (s posaoHR) detail(ctx context.Context, c posaoHRCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, final, err := s.http.GetHTMLResolved(ctx, c.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if u, err := url.Parse(final); err == nil && u.Path != strings.TrimPrefix(c.url, posaoHRBaseURL) {
		return Job{}, false, nil // a gone posting redirects to the home page
	}
	if lang := posaoHRLang(root); lang != "hr" {
		return Job{}, false, fmt.Errorf("page answered in language %q, not hr", lang)
	}
	header := firstByClass(root, "single_job_ad_contain")
	if header == nil {
		return Job{}, false, fmt.Errorf("posting page carries no posting")
	}
	if posaoHRRepublishedPage(root) {
		log.Printf("posaohr: posting %d is a republished employment-service vacancy; skipped", c.id)
		return Job{}, false, nil
	}
	facts := posaoHRHeader(header)
	j := posaoHRBaseJob(c)
	if h1 := firstByTag(root, "h1"); h1 != nil {
		if t := posaoHRClean(textContent(h1)); t != "" {
			j.Title = t
		}
	}
	if place := posaoHRClean(textContent(facts["location"])); place != "" {
		j.Location = place
	}
	body := posaoHRBody(root)
	if strings.TrimSpace(body) == "" {
		return Job{}, false, fmt.Errorf("posting page carries no body")
	}
	if pay := posaoHRSalary(facts["salary"]); pay != "" {
		body = "<p><strong>Plaća:</strong> " + html.EscapeString(pay) + "</p>" + body
	}
	j.Description = sanitizeHTML(body)
	j.IsTechHint = posaoHRTechCategory(facts["category"])
	rows := posaoHRRows(root, "header_jobs_about")
	j.EmploymentType = posaoHREmploymentType(rows["Vrsta posla"])
	if years, ok := posaoHRYears(posaoHRRows(root, "header_jobs_empl")["Potrebno radno iskustvo"]); ok {
		j.ExperienceYearsMin = &years
	}
	return j, true, nil
}

// posaoHRLang is the page's display language, the <html lang> attribute.
func posaoHRLang(root *html.Node) string {
	if h := firstByTag(root, "html"); h != nil {
		return Attr(h, "lang")
	}
	return ""
}

// posaoHRRepublishedPage reports whether a posting page is an employment-service vacancy: the
// title carries em.hzz, or the page links the vacancy at burzarada.hzz.hr. The words "HZZ" in a
// body are no sign — an own posting mentions the health insurer HZZO.
func posaoHRRepublishedPage(root *html.Node) bool {
	found := false
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode &&
			((n.Data == "em" && HasClass(n, "hzz")) ||
				(n.Data == "a" && strings.Contains(Attr(n, "href"), "burzarada.hzz.hr"))) {
			found = true
		}
		return !found
	})
	return found
}

// posaoHRHeader reads the page's fact block: each label div, classed by what it states, followed
// by its value div. The classes, not the label text, are read; of the employer, category, place,
// deadline and pay, the employer is the card's and the deadline is not stored.
func posaoHRHeader(header *html.Node) map[string]*html.Node {
	out := map[string]*html.Node{}
	for n := header.FirstChild; n != nil; n = n.NextSibling {
		if n.Type != html.ElementNode || !HasClass(n, "single_job_ad_left") {
			continue
		}
		value := n.NextSibling
		for value != nil && value.Type != html.ElementNode {
			value = value.NextSibling
		}
		if value == nil {
			break
		}
		for _, key := range []string{"category", "location", "salary"} {
			if HasClass(n, key) {
				out[key] = value
			}
		}
	}
	return out
}

// posaoHRSalary is the pay line as the site writes it ("od 1 500 € neto"), without the salary
// calculator link beside it. No field states the period, so it stays text.
func posaoHRSalary(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return posaoHRClean(b.String()) // Fields splits on the no-break space the site writes too
}

// posaoHRTechCategory reports whether the posting is filed under "Informatika i
// telekomunikacije" alone. A poster ticks up to three categories, so a second one leaves the
// posting to the title dictionary.
func posaoHRTechCategory(n *html.Node) bool {
	if n == nil {
		return false
	}
	var cats []string
	walk(n, func(x *html.Node) bool {
		if x.Type == html.ElementNode && x.Data == "a" {
			cats = append(cats, strings.TrimSuffix(Attr(x, "href"), "/"))
			return false
		}
		return true
	})
	return len(cats) == 1 && strings.HasSuffix(cats[0], "/djelatnosti/informatika-i-telekomunikacije")
}

// posaoHRBody renders the posting. A designed ad is the employer's own HTML (div.custom), read
// whole. A form posting is labelled rows in two sections, "O poslu" (the job) and "O
// zaposleniku" (the candidate); the third, the employer's contact details, is left out.
func posaoHRBody(root *html.Node) string {
	if custom := firstByClass(root, "custom"); custom != nil {
		return innerHTML(custom)
	}
	var b strings.Builder
	for _, section := range []string{"header_jobs_about", "header_jobs_empl"} {
		h2, rows := posaoHRSection(root, section)
		if h2 == nil {
			continue
		}
		b.WriteString("<h2>" + html.EscapeString(posaoHRClean(textContent(h2))) + "</h2>")
		for _, row := range rows {
			b.WriteString("<h3>" + html.EscapeString(row.label) + "</h3><div>" + innerHTML(row.value) + "</div>")
		}
	}
	return b.String()
}

type posaoHRRow struct {
	label string
	value *html.Node
}

// posaoHRSection finds the section whose heading carries the class, and its labelled rows.
func posaoHRSection(root *html.Node, class string) (*html.Node, []posaoHRRow) {
	var section, heading *html.Node
	walk(root, func(n *html.Node) bool {
		if section != nil || n.Type != html.ElementNode {
			return section == nil
		}
		if n.Data == "section" {
			if h := firstByTag(n, "h2"); h != nil && HasClass(h, class) {
				section, heading = n, h
			}
			return false
		}
		return true
	})
	if section == nil {
		return nil, nil
	}
	var rows []posaoHRRow
	walk(section, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !HasClass(n, "jobs_about_holder") {
			return true
		}
		left, right := firstByClass(n, "jobs_about_left"), firstByClass(n, "jobs_about_right")
		if left != nil && right != nil {
			rows = append(rows, posaoHRRow{label: strings.TrimSuffix(posaoHRClean(textContent(left)), ":"), value: right})
		}
		return false
	})
	return heading, rows
}

// posaoHRRows is a section's rows as label to plain text.
func posaoHRRows(root *html.Node, class string) map[string]string {
	_, rows := posaoHRSection(root, class)
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.label] = posaoHRClean(textContent(r.value))
	}
	return out
}

// posaoHREmploymentType reads "Vrsta posla", a multi-select of the contract's kinds. Only a lone
// "honorarni posao / freelance" names a vocabulary value; "stalni radni odnos" (open-ended) and
// "rad na određeno vrijeme" (fixed term) are durations, and "student servis" and "sezonski
// posao" have no counterpart.
func posaoHREmploymentType(kinds string) string {
	if strings.EqualFold(strings.TrimSpace(kinds), "honorarni posao / freelance") {
		return "contract"
	}
	return ""
}

// posaoHRNumbers are the number words the experience picklist writes ("dvije godine", "šest do
// deset godina").
var posaoHRNumbers = map[string]int{
	"jedna": 1, "jednu": 1, "dvije": 2, "tri": 3, "četiri": 4, "pet": 5,
	"šest": 6, "sedam": 7, "osam": 8, "devet": 9, "deset": 10,
}

// posaoHRYears reads the required experience's lower bound.
func posaoHRYears(s string) (int, bool) {
	words := strings.Fields(strings.ToLower(s))
	if len(words) == 0 {
		return 0, false
	}
	n, ok := posaoHRNumbers[words[0]]
	return n, ok
}
