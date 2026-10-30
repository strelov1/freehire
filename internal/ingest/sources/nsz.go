package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// nsz adapts the job search of Serbia's National Employment Service (Nacionalna služba za
// zapošljavanje, nsz.gov.rs): the vacancies Serbian employers publish through the public
// service, ~500 open at a time, every trade, mostly health care, education and local
// employers. It is boardless and an aggregator — the employer is read per posting. See the
// "NSZ traps" section of AGENTS.md for the measurements behind each rule.
//
// The search is a server-rendered page, 20 vacancies a page, newest first; a card carries the
// title, the employer and its town. The body and the vacancy's facts are on the vacancy's own
// page, bought for vacancies the catalogue lacks, within a per-run budget.
//
// The site is written in Serbian Cyrillic (an employer may write its own text in Latin), and
// its "LAT" switch transliterates the page in the browser; no request parameter serves Latin.
// The adapter does the same transliteration on every field it keeps.
type nsz struct {
	http           HTMLResolvedGetter
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

const (
	nszBaseURL = "https://nsz.gov.rs"
	// nszListURL is the unfiltered search; page 1 is the newest vacancies.
	nszListURL = nszBaseURL + "/employee/jobs/search?page=%d"
	// nszPostingURL is a vacancy's page. An id the service no longer shows redirects to the
	// home page.
	nszPostingURL = nszBaseURL + "/employee/jobs/preview/%s"

	// nszMaxPages is a safety ceiling far past the catalogue's size (510 vacancies, 26 pages,
	// on 2026-10-29); reaching it fails the crawl rather than ending it.
	nszMaxPages = 200
	// nszRequestInterval paces every request of a run at one a second — a courtesy ceiling
	// toward a public service, not a measured limit.
	nszRequestInterval = time.Second
	nszDetailWorkers   = 2
	// nszMaxNewPerRun bounds the vacancy pages one run buys: a page answers in ~1.5 s, so 1,200
	// is ~30 minutes, plus a ~2 minute listing walk and the writes, inside the scheduler's
	// default crawl budget. The whole catalogue is ~500, so a cold run takes it all.
	nszMaxNewPerRun   = 1200
	nszDeadlineMargin = 5 * time.Minute

	// nszITCategory is the site's occupational field "Информатика и програмирање".
	nszITCategory = "Информатика и програмирање"
)

// nszBelgrade is the zone the vacancy dates are written in.
var nszBelgrade = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Belgrade"); err == nil {
		return loc
	}
	return time.FixedZone("CET", 3600)
}()

// NewNSZ builds the adapter over the given client.
func NewNSZ(c HTMLResolvedGetter) Source {
	return nsz{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(nszRequestInterval), 1),
		maxNewPerRun:   nszMaxNewPerRun,
		deadlineMargin: nszDeadlineMargin,
	}
}

func (nsz) Provider() string { return "nsz" }

// One national search, no per-tenant id.
func (nsz) boardless() {}

// Hundreds of employers post here, each named per vacancy.
func (nsz) aggregator() {}

// nszCard is one vacancy as the search lists it, still in the site's script.
type nszCard struct {
	ID       string
	Title    string
	Employer string
	Town     string
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s nsz) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s nsz) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists every vacancy, refreshes the ones already stored, and reads up to
// maxNewPerRun of the rest, newest first. The card names the employer, so a vacancy the
// pipeline will discard anyway (a covered employer, a rejected title) is yielded body-less
// without a request. A vacancy whose page cannot be read is not yielded; a later run tries
// again.
func (s nsz) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, nszBaseJob(c).Company)
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []nszCard
	deferred := 0
	for _, c := range cards {
		base := nszBaseJob(c)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, c)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, nszDetailWorkers, func(c nszCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("nsz: vacancy %s failed; deferring: %v", c.ID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no vacancy page at all is broken, not quiet. "Answered" counts a vacancy
	// the site reported gone; vacancies left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("nsz: requested %d vacancy pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("nsz: %d new vacancies left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("nsz: %d new vacancies past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s nsz) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the search to its end: the first page past the last that page 1's pager names,
// proven by a search page carrying no card. Any page failing fails the crawl, and so does a
// page that is not the search, an empty page short of the pager's last, an empty catalogue,
// and reaching nszMaxPages: a partial listing would let the company-scoped sweep close the
// vacancies of employers it did see on the pages it did not. Vacancies are kept once each,
// in listing order.
func (s nsz) list(ctx context.Context) ([]nszCard, error) {
	seenIDs := map[string]bool{}
	var out []nszCard
	last := 0
	for page := 1; page <= nszMaxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		root, _, err := s.http.GetHTMLResolved(ctx, fmt.Sprintf(nszListURL, page))
		if err != nil {
			return nil, fmt.Errorf("nsz: listing page %d: %w", page, err)
		}
		if firstByClass(root, "search-jobs") == nil {
			return nil, fmt.Errorf("nsz: listing page %d is not the search page", page)
		}
		cards := nszCards(root)
		if page == 1 {
			if len(cards) == 0 {
				return nil, fmt.Errorf("nsz: the listing names no vacancy")
			}
			last = nszLastPage(root)
		}
		if len(cards) == 0 {
			if page <= last {
				return nil, fmt.Errorf("nsz: listing page %d is empty, short of the last page %d", page, last)
			}
			return out, nil
		}
		for _, c := range cards {
			if seenIDs[c.ID] {
				continue
			}
			seenIDs[c.ID] = true
			out = append(out, c)
		}
	}
	return nil, fmt.Errorf("nsz: listing still not at its end after %d pages", nszMaxPages)
}

// nszPreviewID matches a vacancy page's path and captures its id.
var nszPreviewID = regexp.MustCompile(`/employee/jobs/preview/([0-9]+)`)

// nszCards reads the vacancy cards of a search page. A card is a div.single-job whose inner
// div opens the vacancy page on click; its job-description paragraph is the employer and the
// employer's town, separated by a run of spaces.
func nszCards(root *xhtml.Node) []nszCard {
	var out []nszCard
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || !HasClass(n, "single-job") {
			return true
		}
		var c nszCard
		walk(n, func(m *xhtml.Node) bool {
			if m.Type != xhtml.ElementNode {
				return true
			}
			if c.ID == "" {
				if id := nszPreviewID.FindStringSubmatch(Attr(m, "onclick")); id != nil {
					c.ID = id[1]
				}
			}
			switch {
			case HasClass(m, "job-title"):
				c.Title = textContent(m)
			case HasClass(m, "job-description"):
				c.Employer, c.Town = nszSplitEmployer(textContent(m))
			}
			return true
		})
		if c.ID != "" {
			out = append(out, c)
		}
		return false
	})
	return out
}

// nszEmployerTown matches the run of spaces between the employer and its town on a card.
var nszEmployerTown = regexp.MustCompile(`\s{2,}`)

// nszSplitEmployer splits a card's "employer  town" line at its last run of spaces; a line
// without one is all employer.
func nszSplitEmployer(s string) (employer, town string) {
	s = strings.TrimSpace(s)
	locs := nszEmployerTown.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return nszClean(s), ""
	}
	cut := locs[len(locs)-1]
	return nszClean(s[:cut[0]]), nszClean(s[cut[1]:])
}

// nszPagerPage matches a pager link and captures its page number.
var nszPagerPage = regexp.MustCompile(`/employee/jobs/search\?page=([0-9]+)$`)

// nszLastPage reads the highest page the search page's pager links to; a search that fits one
// page has no pager, and its last page is 1.
func nszLastPage(root *xhtml.Node) int {
	last := 1
	walk(root, func(n *xhtml.Node) bool {
		if n.Type == xhtml.ElementNode && n.Data == "a" {
			if m := nszPagerPage.FindStringSubmatch(Attr(n, "href")); m != nil {
				if p, err := strconv.Atoi(m[1]); err == nil && p > last {
					last = p
				}
			}
		}
		return true
	})
	return last
}

// nszBaseJob is the body-less Job a vacancy's card supports, in Latin script.
func nszBaseJob(c nszCard) Job {
	return Job{
		ExternalID: c.ID,
		URL:        fmt.Sprintf(nszPostingURL, c.ID),
		Title:      nszLatin(c.Title),
		Company:    nszLatin(c.Employer),
		Location:   nszLatin(c.Town),
		// The service is Serbia's: the town is a Serbian municipality, so the country is the
		// source's, not a reading of the place name.
		Countries: []string{"rs"},
	}
}

// nszFact is one row of a vacancy page's "Подаци о послу" table, label without its colon.
type nszFact struct{ label, value string }

// nszBodyFacts are the table rows rendered under the body, in the site's order. The dates and
// the place are read into the Job instead; "Напомена" (the note) is where the employer writes
// a named contact person, a phone and an e-mail, and is never copied.
var nszBodyFacts = map[string]bool{
	"Врста рада": true, "Број радника": true, "Радно искуство": true, "Степен образовања": true,
	"Образовање": true, "Рад на рачунару": true, "Језик": true,
}

// nszEmploymentTypes maps the kinds of work that are contract work. A permanent ("Неодређено")
// or fixed-term ("Одређено") job states nothing about working time, and volunteering
// ("Волонтирање") is none of the vocabulary.
var nszEmploymentTypes = map[string]string{
	"Уговор о делу":                  "contract",
	"Привремени и повремени послови": "contract",
	"Рад на пројекту":                "contract",
}

// nszPeriod reads the "Временско трајање огласа за посао" value: "29.10.2026. - 28.11.2026.".
var nszPeriod = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})\.(\d{4})\.?\s*-`)

// nszGenderNotice is the opening of the paragraph every body ends with: a statement that the
// masculine forms used in the ad apply to women too. It says nothing about the vacancy.
const nszGenderNotice = "НАПОМЕНА: Сви изрази, појмови"

// detail reads one vacancy's page. ok=false with a nil error is the site's answer that the
// vacancy is gone — a redirect away from the page asked for; an error is the absence of an
// answer, and so is a page that does not carry the vacancy.
func (s nsz) detail(ctx context.Context, c nszCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	u := fmt.Sprintf(nszPostingURL, c.ID)
	root, final, err := s.http.GetHTMLResolved(ctx, u)
	if err != nil {
		return Job{}, false, err
	}
	if final != "" && final != u {
		return Job{}, false, nil
	}
	job, err := nszJob(root, c)
	if err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

// nszJob maps a vacancy page and its card to a Job.
func nszJob(root *xhtml.Node, c nszCard) (Job, error) {
	preview := firstByClass(root, "job-preview")
	if preview == nil {
		return Job{}, fmt.Errorf("nsz: vacancy %s: the page carries no vacancy", c.ID)
	}
	j := nszBaseJob(c)
	if t := firstByClass(preview, "job-title"); t != nil {
		if title := nszLatin(textContent(t)); title != "" {
			j.Title = title
		}
	}
	if e := firstByClass(preview, "job-description"); e != nil {
		if employer := nszLatin(textContent(e)); employer != "" {
			j.Company = employer
		}
	}
	// The location line opens with the vacancy's occupational field in its own span:
	// "<span>Информатика и програмирање</span> - <span>Београд; </span>".
	if loc := firstByClass(preview, "job-location"); loc != nil {
		for c := loc.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == xhtml.ElementNode && c.Data == "span" {
				j.IsTechHint = nszClean(textContent(c)) == nszITCategory
				break
			}
		}
	}
	facts := nszFacts(preview)
	var listed []string
	for _, f := range facts {
		switch f.label {
		case "Временско трајање огласа за посао":
			j.PostedAt = NotFuture(nszDate(f.value))
		case "Место рада":
			if place := nszPlace(f.value); place != "" {
				j.Location = place
			}
		case "Врста рада":
			j.EmploymentType = nszEmploymentTypes[f.value]
		}
		if nszBodyFacts[f.label] && f.value != "" {
			listed = append(listed, "<li>"+html.EscapeString(f.label)+": "+html.EscapeString(f.value)+"</li>")
		}
	}
	var body strings.Builder
	hasText := false
	if content := firstByClass(preview, "job-content"); content != nil {
		for n := content.FirstChild; n != nil; n = n.NextSibling {
			text := textContent(n)
			if n.Type == xhtml.ElementNode && strings.HasPrefix(text, nszGenderNotice) {
				continue
			}
			hasText = hasText || strings.TrimSpace(text) != ""
			_ = xhtml.Render(&body, n)
		}
	}
	if len(listed) > 0 {
		body.WriteString("<ul>" + strings.Join(listed, "") + "</ul>")
	}
	j.Description = sanitizeHTML(normalize.LatinizeSerbian(strings.TrimSpace(body.String())))
	if j.Title == "" || j.Company == "" || !hasText {
		return Job{}, fmt.Errorf("nsz: vacancy %s: the page carries no title, employer or body", c.ID)
	}
	return j, nil
}

// nszFacts reads the "Подаци о послу" table: each div.table-row holds a label column and a
// value column. Values are read as text with whitespace collapsed.
func nszFacts(root *xhtml.Node) []nszFact {
	var out []nszFact
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || !HasClass(n, "table-row") {
			return true
		}
		var cols []*xhtml.Node
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == xhtml.ElementNode && HasClass(c, "table-col") {
				cols = append(cols, c)
			}
		}
		if len(cols) == 2 {
			label := strings.TrimSuffix(nszClean(textContent(cols[0])), ":")
			out = append(out, nszFact{label: strings.TrimSpace(label), value: nszClean(textContent(cols[1]))})
		}
		return false
	})
	return out
}

// nszDate reads the first date of a vacancy's period, midnight in Belgrade.
func nszDate(s string) *time.Time {
	m := nszPeriod.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	day, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	year, _ := strconv.Atoi(m[3])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return nil
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, nszBelgrade)
	return &t
}

// nszPlace reads "Место рада", one or more municipalities each closed by a semicolon
// ("Ниш; Лесковац; "), as a Latin list.
func nszPlace(s string) string {
	var places []string
	for _, p := range strings.Split(s, ";") {
		if p = strings.TrimSpace(p); p != "" {
			places = append(places, p)
		}
	}
	return nszLatin(strings.Join(places, "; "))
}

// nszLatin cleans a field and writes it in Latin script.
func nszLatin(s string) string { return normalize.LatinizeSerbian(nszClean(s)) }

func nszClean(s string) string { return strings.Join(strings.Fields(html.UnescapeString(s)), " ") }
