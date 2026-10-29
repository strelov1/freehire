package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// hzz adapts the job exchange of the Croatian Employment Service (Hrvatski zavod za
// zapošljavanje, burzarada.hzz.hr): every vacancy a Croatian employer registers with the public
// service, ~7.5k open at a time, ~500 new each weekday, every trade, mostly local employers and the
// public sector. It is boardless and an aggregator — the employer is read per posting. See the
// "HZZ traps" section of AGENTS.md for the measurements behind each rule.
//
// The listing is the site's own RSS: one feed per county (21 of them) together names every
// domestic vacancy, with its title, category and publication day. The employer and the whole
// body are only on the vacancy's print page, bought for vacancies the catalogue lacks, newest
// first, within a per-run budget; the rest are refreshed by identity. The IT category's feed is
// read as well, only to mark its vacancies technical.
type hzz struct {
	http           hzzGetter
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

// hzzGetter is the transport the adapter needs: the feeds as XML (ISO-8859-2, served as
// text/xml) and a print page as an HTML tree. Both need the session cookie the site sets on a
// first request, so the adapter runs over a cookie client (see the registry).
type hzzGetter interface {
	XMLGetter
	HTMLGetter
}

const (
	hzzBaseURL = "https://burzarada.hzz.hr"
	// hzzCountyFeed is one county's feed. Counties 1-21 are Croatia's 20 counties and the City of
	// Zagreb; 1000 is "Poslovi u EU", vacancies abroad, which is not read.
	hzzCountyFeed = hzzBaseURL + "/rss/rsszup%d.xml"
	hzzCounties   = 21
	// hzzTechFeed is the category "Informatički, računalni i stručnjaci za Internet".
	hzzTechFeed  = hzzBaseURL + "/rss/rsskat1001.xml"
	hzzDetailURL = hzzBaseURL + "/RadnoMjesto_Ispis.aspx?WebSifra=%d"

	// hzzRequestInterval paces every request of a run, feed and page alike, at one a second — a
	// courtesy ceiling toward a public service, not a measured limit.
	hzzRequestInterval = time.Second
	hzzDetailWorkers   = 2
	// hzzMaxNewPerRun bounds the print pages one run buys: 1,200 at one a second is 20 minutes,
	// plus ~22 feed requests and the writes that follow, inside the scheduler's default 2,400 s
	// crawl budget. ~500 vacancies arrive each weekday, so a run every few hours keeps up and a
	// cold catalogue (~7.5k) is taken in about seven runs, newest first.
	hzzMaxNewPerRun   = 1200
	hzzDeadlineMargin = 5 * time.Minute
)

// NewHZZ builds the adapter over the given client, which must keep cookies.
func NewHZZ(c hzzGetter) Source {
	return hzz{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(hzzRequestInterval), 1),
		maxNewPerRun:   hzzMaxNewPerRun,
		deadlineMargin: hzzDeadlineMargin,
	}
}

func (hzz) Provider() string { return "hzz" }

// One national exchange, no per-tenant id.
func (hzz) boardless() {}

// Thousands of employers post here, each named per vacancy.
func (hzz) aggregator() {}

// hzzPosting is one vacancy the feeds name.
type hzzPosting struct {
	id     int64
	title  string
	posted *time.Time
	tech   bool
}

func (p hzzPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

func hzzPostingURL(id int64) string { return fmt.Sprintf(hzzDetailURL, id) }

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s hzz) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s hzz) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists every domestic vacancy, refreshes the ones already stored, and reads up to
// maxNewPerRun of the rest, newest first. The feeds name no employer, so only the catalogue
// filter can spare a page: a vacancy whose title it rejects is yielded body-less without a
// request. A vacancy whose page cannot be read is not yielded; a later run tries again.
func (s hzz) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []hzzPosting
	deferred := 0
	for _, p := range postings {
		base := hzzBaseJob(p)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if gate.Rejected != nil && gate.Rejected(base) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, p)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, hzzDetailWorkers, func(p hzzPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("hzz: vacancy %d failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a vacancy the
	// site reported expired; vacancies left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("hzz: requested %d vacancy pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("hzz: %d new vacancies left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("hzz: %d new vacancies past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s hzz) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// hzzBaseJob is the body-less Job a vacancy's feed entry supports.
func hzzBaseJob(p hzzPosting) Job {
	return Job{
		ExternalID:        p.externalID(),
		URL:               hzzPostingURL(p.id),
		Title:             p.title,
		PostedAt:          NotFuture(p.posted),
		IsTechHint:        p.tech,
		FallbackCountries: []string{"hr"},
	}
}

// list reads every county feed and returns their vacancies merged by id, newest first. Any
// county feed failing fails the crawl: a partial listing would let the company-scoped sweep close
// the vacancies of employers it did see in the counties it did not. The IT feed only marks
// vacancies technical, so its failing costs the mark, not the crawl.
func (s hzz) list(ctx context.Context) ([]hzzPosting, error) {
	byID := make(map[int64]*hzzPosting)
	for county := 1; county <= hzzCounties; county++ {
		doc, err := s.feed(ctx, fmt.Sprintf(hzzCountyFeed, county))
		if err != nil {
			return nil, fmt.Errorf("hzz: county feed %d: %w", county, err)
		}
		for _, it := range hzzFeedItems(doc) {
			if byID[it.id] == nil {
				byID[it.id] = &it
			}
		}
	}
	if len(byID) == 0 {
		return nil, fmt.Errorf("hzz: the county feeds name no vacancy")
	}
	if doc, err := s.feed(ctx, hzzTechFeed); err != nil {
		log.Printf("hzz: IT category feed failed; no vacancy is marked technical this run: %v", err)
	} else {
		for _, it := range hzzFeedItems(doc) {
			if p := byID[it.id]; p != nil {
				p.tech = true
			}
		}
	}
	out := make([]hzzPosting, 0, len(byID))
	for _, p := range byID {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// feed fetches and decodes one feed.
func (s hzz) feed(ctx context.Context, u string) (hzzFeedDoc, error) {
	var doc hzzFeedDoc
	if err := s.limiter.Wait(ctx); err != nil {
		return doc, err
	}
	err := s.http.GetXML(ctx, u, &doc)
	return doc, err
}

type hzzFeedDoc struct {
	Items []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		PubDate string `xml:"pubDate"`
	} `xml:"channel>item"`
}

var hzzVacancyID = regexp.MustCompile(`WebSifra=(\d+)`)

// hzzFeedItems reads a feed's vacancies. An item with an empty title is an EXPIRED vacancy the
// feed still carries — its page answers "Traženi oglas je istekao!" — so it is not listed, and
// the sweep closes it like any vacancy that left the feeds.
func hzzFeedItems(feed hzzFeedDoc) []hzzPosting {
	var out []hzzPosting
	for _, it := range feed.Items {
		m := hzzVacancyID.FindStringSubmatch(it.Link)
		if m == nil {
			continue
		}
		id, err := strconv.ParseInt(m[1], 10, 64)
		title := hzzClean(it.Title)
		if err != nil || id <= 0 || title == "" {
			continue
		}
		p := hzzPosting{id: id, title: title}
		if t, ok := hzzDate(it.PubDate); ok {
			p.posted = &t
		}
		out = append(out, p)
	}
	return out
}

func hzzClean(s string) string { return strings.Join(strings.Fields(html.UnescapeString(s)), " ") }

// hzzZagreb is the zone the site's dates are written in.
var hzzZagreb = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Zagreb"); err == nil {
		return loc
	}
	return time.FixedZone("CET", 60*60)
}()

// hzzDate reads "13.10.2026" (feed) or "2.11.2026." (page), a day in Croatian local time.
func hzzDate(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("2.1.2006", strings.TrimSuffix(strings.TrimSpace(s), "."), hzzZagreb)
	return t, err == nil
}

// hzzExpired is the notice the print page serves, HTTP 200, for a vacancy no longer open (and for
// an id that never existed).
const hzzExpired = "Traženi oglas je istekao"

// detail reads one print page. ok=false with a nil error is the site's answer that the vacancy is
// gone (its expiry notice, or a 404/410) or names no title, employer or body; an error is the
// absence of an answer.
func (s hzz) detail(ctx context.Context, p hzzPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.http.GetHTML(ctx, hzzPostingURL(p.id))
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if firstByID(root, "ctl00_MainContent_lblNazivPoslodavca") == nil {
		if strings.Contains(textContent(root), hzzExpired) {
			return Job{}, false, nil
		}
		// Neither a vacancy nor the expiry notice: a maintenance or error page, not an answer.
		return Job{}, false, fmt.Errorf("page carries no vacancy (%q)", titleText(root))
	}
	job, ok := hzzJob(root, p)
	return job, ok, nil
}

// hzzField is the text of one of the print page's labelled values.
func hzzField(root *html.Node, name string) string {
	if n := firstByID(root, "ctl00_MainContent_"+name); n != nil {
		return hzzClean(textContent(n))
	}
	return ""
}

// hzzJob maps a print page and its feed entry to a Job. ok is false for one with no title,
// employer or body.
func hzzJob(root *html.Node, p hzzPosting) (Job, bool) {
	j := hzzBaseJob(p)
	if h3 := firstByTag(root, "h3"); h3 != nil {
		if t := hzzClean(textContent(h3)); t != "" {
			j.Title = t
		}
	}
	j.Company = hzzCompanyName(hzzField(root, "lblNazivPoslodavca"))
	j.Description = hzzBody(root)
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	j.Location = hzzPlace(hzzField(root, "lblMjestoRada"))
	if t, ok := hzzDate(hzzField(root, "lblVrijediOd")); ok {
		j.PostedAt = NotFuture(&t)
	}
	j.EmploymentType = hzzEmploymentType(hzzField(root, "bulVrstaZaposlenja"), hzzField(root, "lblRadnoVrijeme"))
	if y, ok := hzzExperienceYears(hzzField(root, "lblRadnoIskustvo")); ok {
		j.ExperienceYearsMin = &y
	}
	if pay, ok := hzzSalary(hzzField(root, "lblPlaca")); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &pay, &pay, "EUR", "month"
	}
	return j, true
}

// hzzBody is the vacancy's prose: the "Opis posla" field when the employer filled it, then the
// "Ostale informacije" block, which is free HTML between its label and the next field divider.
func hzzBody(root *html.Node) string {
	var b strings.Builder
	if n := firstByID(root, "ctl00_MainContent_lblOpisPosla"); n != nil {
		// Plain text whose lines are the employer's list items.
		if inner := strings.TrimSpace(innerHTML(n)); inner != "" {
			b.WriteString("<p>" + strings.ReplaceAll(inner, "\n", "<br>") + "</p>")
		}
	}
	if label := firstByID(root, "ctl00_MainContent_lblOstaleInformacijeText"); label != nil {
		for n := label.NextSibling; n != nil; n = n.NextSibling {
			if n.Type == html.ElementNode && n.Data == "hr" {
				break
			}
			var sb strings.Builder
			if err := html.Render(&sb, n); err == nil {
				b.WriteString(sb.String())
			}
		}
	}
	return sanitizeHTML(b.String())
}

// hzzPlace reads "SINJ, SPLITSKO-DALMATINSKA ŽUPANIJA" — the town and its county, in capitals.
func hzzPlace(s string) string {
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = hzzTitleCase(strings.TrimSpace(p))
	}
	return strings.Trim(strings.Join(parts, ", "), ", ")
}

// hzzTitleCase turns the site's capitals into a readable name ("SPLITSKO-DALMATINSKA ŽUPANIJA" →
// "Splitsko-Dalmatinska Županija"); a value already in mixed case is kept.
func hzzTitleCase(s string) string {
	if s != strings.ToUpper(s) {
		return s
	}
	rs := []rune(strings.ToLower(s))
	start := true
	for i, r := range rs {
		if start {
			rs[i] = []rune(strings.ToUpper(string(r)))[0]
		}
		start = r == ' ' || r == '-' || r == '('
	}
	return string(rs)
}

var (
	// hzzLegalFormSpelled is a registered legal form written out, which the register's own names
	// often do ("CONER društvo s ograničenom odgovornošću za proizvodnju").
	hzzLegalFormSpelled = []struct{ long, short string }{
		{"jednostavno društvo s ograničenom odgovornošću", "j.d.o.o."},
		{"društvo s ograničenom odgovornošću", "d.o.o."},
		{"dioničko društvo", "d.d."},
	}
	// hzzActivityTail is the registered activity a company's full register name carries after its
	// legal form: "EUROTIM d.o.o. za proizvodnju i trgovinu".
	hzzActivityTail = regexp.MustCompile(`(?i)^(.*?\b(?:j\.\s?d\.\s?o\.\s?o\.|d\.\s?o\.\s?o\.|d\.\s?d\.))\s+(?:za|u)\s.*$`)
	// hzzCraftQuoted is a craft (obrt) whose register name quotes its trading name:
	// `OBRT ZA UGOSTITELJSTVO ... "GT LUDBREG"- Sanja Stručić, Ludbreg, Kratka 2`.
	hzzCraftQuoted = regexp.MustCompile(`["'„“”]([^"'„“”]{2,})["'„“”]`)
)

// hzzCompanyName reduces an employer's full register name to the name it trades under. The
// register spells the legal form out and appends the company's registered activity, so one
// employer reads differently from every other source that names it; and a craft (obrt) carries
// its owner's name and home address, which is personal data rather than an employer name.
func hzzCompanyName(s string) string {
	name := hzzClean(s)
	lower := strings.ToLower(name)
	for _, f := range hzzLegalFormSpelled {
		if i := strings.Index(lower, f.long); i >= 0 {
			name = name[:i] + f.short + name[i+len(f.long):]
			lower = strings.ToLower(name)
		}
	}
	if m := hzzActivityTail.FindStringSubmatch(name); m != nil {
		name = m[1]
	}
	if strings.Contains(strings.ToLower(name), "obrt") {
		if q := hzzCraftQuoted.FindStringSubmatch(name); q != nil {
			name = q[1]
		} else if i := strings.Index(name, ","); i > 0 {
			name = name[:i]
		}
	}
	return strings.Join(strings.Fields(name), " ")
}

// hzzEmploymentType reads the structured contract kinds and working time. "pripravnik" is a
// traineeship; otherwise full-time working time is full_time and any stated hours part_time.
// "Na određeno" (fixed term) says nothing about the hours and is not mapped to contract work.
func hzzEmploymentType(kinds, hours string) string {
	k, h := strings.ToLower(kinds), strings.ToLower(hours)
	switch {
	case strings.Contains(k, "pripravni"), strings.Contains(k, "stručno osposobljavanje"):
		return "internship"
	case strings.Contains(h, "puno radno vrijeme"):
		return "full_time"
	case strings.Contains(h, "nepuno"), strings.Contains(h, "sati tjedno"):
		return "part_time"
	}
	return ""
}

var hzzYears = regexp.MustCompile(`^(\d{1,2})\s+godin`)

// hzzExperienceYears reads "2 godine" / "1 godinu"; "Nije važno" and a trainee call state none.
func hzzExperienceYears(s string) (int, bool) {
	m := hzzYears.FindStringSubmatch(strings.ToLower(strings.TrimSpace(s)))
	if m == nil {
		return 0, false
	}
	y, err := strconv.Atoi(m[1])
	return y, err == nil && y > 0
}

var hzzEuro = regexp.MustCompile(`^(\d[\d.]*)(?:,\d{1,2})?\s*€$`)

// hzzSalary reads the structured "Bruto plaća" field, "1210 €": the employer's stated monthly
// gross pay in euros — the form asks for the monthly gross figure. Croatia's minimum gross wage
// is ~970 € a month, so a figure far below it is a placeholder, and one above the ceiling is a
// typo or an annual figure; both are dropped rather than stored as a month's pay.
func hzzSalary(s string) (int, bool) {
	m := hzzEuro.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.ReplaceAll(m[1], ".", ""))
	if err != nil || v < hzzMinMonthlyPay || v > hzzMaxMonthlyPay {
		return 0, false
	}
	return v, true
}

const (
	hzzMinMonthlyPay = 500
	hzzMaxMonthlyPay = 20000
)
