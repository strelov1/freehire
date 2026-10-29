package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// optius adapts Optius (www.optius.com), a Slovenian general job board: ~1.3k open postings
// from employers and staffing agencies across every trade. It is boardless and an aggregator —
// the employer is read per posting. See the "Optius traps" section of AGENTS.md for the
// measurements behind each rule.
//
// The listing is server-rendered HTML, twenty cards a page, newest first; a card carries the
// title, the employer, the place and the application deadline, never the body. The body is
// bought from the posting page for postings the catalogue lacks, within a per-run budget, one
// request at a time: the site answers 429 to as few as two concurrent requests. The site's own
// "abroad" region listing (Tujina) is walked as well, so the Slovenian fallback country is never
// attached to a posting in Austria whose place the location dictionary cannot read.
type optius struct {
	http           HTMLGetter
	limiter        waiter
	maxNewPerRun   int
	maxPages       int
	deadlineMargin time.Duration
}

const (
	optiusBaseURL = "https://www.optius.com"
	optiusListing = "/iskalci/prosta-delovna-mesta/"
	// optiusAbroadListing is the region filter's "Tujina" (abroad) slice.
	optiusAbroadListing = optiusListing + "regija/tujina/"
	// optiusPageSize is the cards a listing page carries; the pager's s= is a card offset.
	optiusPageSize = 20
	// optiusMaxPages is a safety ceiling on one listing walk: 66 pages on 2026-10-29. Reaching
	// it fails the crawl rather than ending the walk as a success.
	optiusMaxPages = 300
	// optiusRequestInterval paces every request of a run, listing and posting page alike, at one
	// every three seconds. One request is in flight at a time, yet a walk paced at one a second
	// drew a 429 by its twelfth page, while requests back to back at ~4 s each never did.
	optiusRequestInterval = 3 * time.Second
	// optiusMaxNewPerRun bounds the posting pages one run reads. The listing walk takes ~5
	// minutes and a posting page ~3.5 s, so 400 pages fit the scheduler's 2,400 s crawl budget
	// with the writes after them. A cold catalogue (~1.3k) is taken over a few runs.
	optiusMaxNewPerRun   = 400
	optiusDeadlineMargin = 5 * time.Minute
)

// NewOptius builds the adapter over the given client.
func NewOptius(c HTMLGetter) Source {
	return optius{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(optiusRequestInterval), 1),
		maxNewPerRun:   optiusMaxNewPerRun,
		maxPages:       optiusMaxPages,
		deadlineMargin: optiusDeadlineMargin,
	}
}

func (optius) Provider() string { return "optius" }

// One national board, no per-tenant id.
func (optius) boardless() {}

// Hundreds of employers and agencies post here, each named per posting.
func (optius) aggregator() {}

// optiusCard is one listing card: what the gate and the job need before the page is bought.
type optiusCard struct {
	id      string
	url     string
	title   string
	company string
	place   string
	abroad  bool
}

// optiusIDPath is a posting path: a slug ending in the posting id, written twice.
var optiusIDPath = regexp.MustCompile(`^/iskalci/prosta-delovna-mesta/[a-z0-9-]+-(\d+)-(\d+)/$`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s optius) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s optius) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the listing, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest first. A posting the pipeline will discard anyway is judged
// on its card and yielded body-less without a request. A posting whose page cannot be read is
// not yielded; a later run tries again.
func (s optius) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
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
	var unseen []optiusCard
	deferred := 0
	for _, c := range cards {
		base := optiusBaseJob(c)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[c.company] || (gate.Rejected != nil && gate.Rejected(base)) {
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
	// One worker: the site refuses concurrent requests (see the type comment).
	read := fetchDetails(unseen, 1, func(c optiusCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("optius: posting %s failed; deferring: %v", c.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("optius: requested %d posting pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("optius: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("optius: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns.
func (s optius) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the abroad slice, then the whole listing, returning its postings in listing order
// (newest first) with the abroad ones marked. Any page failing fails the crawl: a partial
// listing would let the company-scoped sweep close the postings of the employers it did see on
// the pages it did not.
func (s optius) list(ctx context.Context) ([]optiusCard, error) {
	abroad, err := s.walk(ctx, optiusAbroadListing)
	if err != nil {
		return nil, err
	}
	abroadIDs := make(map[string]bool, len(abroad))
	for _, c := range abroad {
		abroadIDs[c.id] = true
	}
	cards, err := s.walk(ctx, optiusListing)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("optius: the listing named no posting")
	}
	for i := range cards {
		cards[i].abroad = abroadIDs[cards[i].id]
	}
	return cards, nil
}

// walk reads one listing to its end: the first page carrying no card at an offset past the
// total the page itself states. A cardless page without the result counter (a challenge or an
// error page answered 200), or one short of the stated total, is not the listing's end and
// fails the walk. The total is the cardless page's own, so postings that expired while the walk
// ran do not fail it.
func (s optius) walk(ctx context.Context, path string) ([]optiusCard, error) {
	seen := make(map[string]bool)
	var out []optiusCard
	for page := 0; ; page++ {
		if page == s.maxPages {
			return nil, fmt.Errorf("optius: %s runs past %d pages", path, s.maxPages)
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		root, err := s.http.GetHTML(ctx, optiusBaseURL+path+"?s="+strconv.Itoa(page*optiusPageSize))
		if err != nil {
			return nil, fmt.Errorf("optius: %s page %d: %w", path, page+1, err)
		}
		total, ok := optiusTotal(root)
		if !ok {
			return nil, fmt.Errorf("optius: %s page %d is not a listing", path, page+1)
		}
		cards := optiusCards(root)
		if len(cards) == 0 {
			if offset := page * optiusPageSize; offset < total {
				return nil, fmt.Errorf("optius: %s page %d is empty at offset %d of %d postings", path, page+1, offset, total)
			}
			return out, nil
		}
		for _, c := range cards {
			// A posting published mid-walk shifts the pages by one, repeating a card.
			if !seen[c.id] {
				seen[c.id] = true
				out = append(out, c)
			}
		}
	}
}

// optiusTotal reads the postings count a listing page states in its result counter; ok is
// false for a page that is not a listing.
func optiusTotal(root *html.Node) (int, bool) {
	counter := firstByClass(root, "result-count")
	if counter == nil {
		return 0, false
	}
	span := firstByTag(counter, "span")
	if span == nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.ReplaceAll(optiusClean(textContent(span)), ".", ""))
	return n, err == nil
}

// optiusCards reads the posting cards of one listing page.
func optiusCards(root *html.Node) []optiusCard {
	var out []optiusCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !HasClass(n, "item") || !strings.HasPrefix(Attr(n, "id"), "job") {
			return true
		}
		if c, ok := optiusCardFrom(n); ok {
			out = append(out, c)
		}
		return false
	})
	return out
}

func optiusCardFrom(n *html.Node) (optiusCard, bool) {
	a := firstByClass(n, "hover-link")
	if a == nil {
		return optiusCard{}, false
	}
	path := strings.TrimSpace(Attr(a, "href"))
	m := optiusIDPath.FindStringSubmatch(path)
	if m == nil {
		return optiusCard{}, false
	}
	c := optiusCard{id: m[2], url: optiusBaseURL + path}
	if h := firstByTag(n, "h3"); h != nil {
		c.title = optiusClean(textContent(h))
	}
	if name := firstByClass(n, "company-name"); name != nil {
		c.company = optiusClean(textContent(name))
	}
	if infos := firstByClass(n, "job-infos"); infos != nil {
		walk(infos, func(x *html.Node) bool {
			if x.Type != html.ElementNode || x.Data != "li" {
				return true
			}
			left, right := firstByClass(x, "left"), firstByClass(x, "right")
			if left != nil && right != nil && optiusClean(textContent(left)) == "Kraj dela" {
				c.place = optiusPlace(textContent(right))
			}
			return false
		})
	}
	if c.title == "" || c.company == "" {
		return optiusCard{}, false
	}
	return c, true
}

func optiusClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// optiusPlateCode is the vehicle-registration code the site's region names carry ("Podravska -
// MB"). Read as a geography token it is a foreign state or country (MB is Manitoba, KR Korea).
var optiusPlateCode = regexp.MustCompile(`\s+-\s+[A-Z]{2}\s*(,|$)`)

// optiusPlace cleans a card's "Kraj dela" (place of work): the region's plate code is dropped,
// and the space the site leaves before a comma with it.
func optiusPlace(s string) string {
	s = optiusPlateCode.ReplaceAllString(optiusClean(s), "$1")
	return strings.ReplaceAll(s, " ,", ",")
}

// optiusBaseJob is the body-less Job a card alone supports: the identity, and the facts the gate
// judges on. Slovenia is the fallback country of every posting outside the abroad slice; it
// applies only when the place, the title and the body name no country of their own.
func optiusBaseJob(c optiusCard) Job {
	j := Job{
		ExternalID: c.id,
		URL:        c.url,
		Title:      c.title,
		Company:    c.company,
		Location:   c.place,
	}
	if !c.abroad {
		j.FallbackCountries = []string{"si"}
	}
	return j
}

// optiusLD is the part of the posting page's schema.org JobPosting the adapter reads. Its strings
// carry raw newlines, which ldJobPosting repairs. The text fields are the body of a posting
// whose page shows only an image or an embedded third-party page.
type optiusLD struct {
	DatePosted             string `json:"datePosted"`
	Description            string `json:"description"`
	Responsibilities       string `json:"responsibilities"`
	JobBenefits            string `json:"jobBenefits"`
	ExperienceRequirements string `json:"experienceRequirements"`
	WorkHours              string `json:"workHours"`
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410, or a listing in place of the posting); an error is the absence of an answer.
func (s optius) detail(ctx context.Context, c optiusCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.http.GetHTML(ctx, c.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld optiusLD
	hasLD := ldJobPosting(root, &ld)
	section := firstByClass(root, "job-detail-col") // the page's main column
	if section == nil && !hasLD {
		if firstByClass(root, "result-count") != nil {
			return Job{}, false, nil
		}
		return Job{}, false, fmt.Errorf("posting page carries no posting")
	}
	j := optiusBaseJob(c)
	if section != nil {
		j.Description = sanitizeHTML(optiusBody(section))
	}
	if strings.TrimSpace(j.Description) == "" && hasLD {
		j.Description = sanitizeHTML(optiusLDBody(ld))
	}
	if strings.TrimSpace(j.Description) == "" {
		return Job{}, false, fmt.Errorf("posting page carries no body")
	}
	if hasLD {
		if t, err := time.Parse("2006-01-02", strings.TrimSpace(ld.DatePosted)); err == nil {
			j.PostedAt = NotFuture(&t)
		}
		j.EmploymentType = optiusEmploymentType(ld.WorkHours)
		if years, ok := optiusYears(ld.ExperienceRequirements); ok {
			j.ExperienceYearsMin = &years
		}
	}
	return j, true, nil
}

// optiusBody renders the posting from the page's main column: the employer's introduction
// under its name, then each section heading ("Opis delovnega mesta", "Pričakujemo", "Ponujamo")
// and labelled fact ("Delovni čas", "Zahtevana izobrazba") in page order. Employers with their
// own page template keep the same headings and fact rows, so one reading covers both. A page
// whose posting is an image or an embedded third-party page yields nothing here.
func optiusBody(section *html.Node) string {
	var b strings.Builder
	heading := ""
	writeHeading := func(n *html.Node) {
		heading = optiusClean(textContent(n))
		if heading != "" {
			b.WriteString("<h3>" + html.EscapeString(heading) + "</h3>")
		}
	}
	walk(section, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case HasClass(n, "company-header-wrapper"):
			for p := n.FirstChild; p != nil; p = p.NextSibling {
				if p.Type == html.ElementNode && p.Data == "p" {
					b.WriteString("<p>" + innerHTML(p) + "</p>")
				}
			}
			return false
		case n.Data == "h3" && HasClass(n, "section-title"):
			writeHeading(n)
			return false
		case n.Data == "div" && HasClass(n, "info-cols"):
			left, right := firstByClass(n, "col-left"), firstByClass(n, "col-right")
			if left == nil || right == nil {
				return false
			}
			if h := firstByClass(left, "section-title"); h != nil {
				writeHeading(h)
			} else if label := strings.TrimSuffix(optiusClean(textContent(left)), ":"); label != "" && label != heading {
				b.WriteString("<p><strong>" + html.EscapeString(label) + "</strong></p>")
			}
			b.WriteString("<div>" + innerHTML(right) + "</div>")
			return false
		}
		return true
	})
	return b.String()
}

// optiusLDBody renders the JobPosting's text fields, for a posting whose page shows no text of
// its own. The list fields put one item a line.
func optiusLDBody(ld optiusLD) string {
	var b strings.Builder
	if d := strings.TrimSpace(ld.Description); d != "" {
		b.WriteString("<p>" + html.EscapeString(d) + "</p>")
	}
	for _, f := range []struct{ heading, text string }{
		{"Opis delovnega mesta", ld.Responsibilities},
		{"Ponujamo", ld.JobBenefits},
	} {
		var items []string
		for _, line := range strings.Split(f.text, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				items = append(items, "<li>"+html.EscapeString(line)+"</li>")
			}
		}
		if len(items) > 0 {
			b.WriteString("<h3>" + f.heading + "</h3><ul>" + strings.Join(items, "") + "</ul>")
		}
	}
	return b.String()
}

// optiusEmploymentType reads the JobPosting's working-time picklist. Its employmentType is the
// contract's term (fixed or open-ended), which freehire's vocabulary does not carry.
func optiusEmploymentType(workHours string) string {
	switch strings.ToLower(strings.TrimSpace(workHours)) {
	case "polni delovni čas":
		return "full_time"
	case "skrajšani delovni čas":
		return "part_time"
	}
	return ""
}

// optiusYearsRe reads the experience picklist: "0 - 1 leto", "1 - 3 let", "6 - 9 let".
var optiusYearsRe = regexp.MustCompile(`^(\d+)\s*-\s*\d+\s+let`)

// optiusYears returns the experience band's lower bound; "0 - 1 leto" (none required) is not a
// minimum worth stating.
func optiusYears(s string) (int, bool) {
	m := optiusYearsRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n == 0 {
		return 0, false
	}
	return n, true
}
