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

// jobbankca adapts Job Bank (jobbank.gc.ca), the Government of Canada's national job board,
// every trade and every province: ~67,000 open postings on 2026-11-22. It is boardless and an
// aggregator. Only part of it is posted by employers on Job Bank itself; the rest is fed in by
// partner boards (Talent.com, indeed.com, CareerBeacon, Québec emploi, ...), and every posting,
// whatever its origin, has a page of its own under /jobsearch/jobposting/<id> with a
// description. Each names its own employer. The English site is the only one read: the French
// one, guichetemplois.gc.ca, carries the same ids. See providers/jobbankca.md.
//
// robots.txt states no Disallow at all, only `Crawl-delay: 5`, so every request of a run is
// paced to one every five seconds, listing and posting pages alike. That pace, not the site, is
// the budget: a run is a bounded window of the listing plus a bounded number of posting pages,
// and the catalogue is covered by the postings drifting through that window, not by one walk.
//
// The listing is the site's own search sorted by date, 25 rows a page, newest first; a row
// carries nothing the posting page does not, so only its id is read. The posting page carries
// the facts as RDFa (title, datePosted, hiringOrganization, joblocation, baseSalary,
// employmentType, validThrough) and the body as a hidden `description` property, plain text or
// escaped HTML depending on the partner it came from. Postings are hydrated only when the
// catalogue lacks them (HydratingSource); the rest are refreshed by identity.
//
// The crawl host's own address times out on this site, so production reads it through the
// Scrapling tier (scraplingProviders), which served every page measured.
type jobbankca struct {
	http HTMLGetter
	// limiter paces every request of a run.
	limiter waiter
	// listingPages bounds the listing walk; freshPerRun and backlogPerRun bound the posting
	// pages one run buys (see jobbankcaFreshPerRun).
	listingPages  int
	freshPerRun   int
	backlogPerRun int
	// deadlineMargin is how much of the run's deadline the crawl leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNew returns, so one that runs into its deadline
	// stores nothing at all.
	deadlineMargin time.Duration
	// retryBackoff is the first pause before a refused request is asked again; it doubles.
	retryBackoff time.Duration
	now          func() time.Time
}

const (
	jobbankcaBase       = "https://www.jobbank.gc.ca"
	jobbankcaListingURL = jobbankcaBase + "/jobsearch/jobsearch?searchstring=&sort=D&page=%d"
	jobbankcaPostingURL = jobbankcaBase + "/jobsearch/jobposting/%s"

	// jobbankcaRequestInterval is robots.txt's Crawl-delay.
	jobbankcaRequestInterval = 5 * time.Second
	// jobbankcaListingPages is the window of the newest postings read each run: 160 pages of 25
	// is 4,000 postings and ~13 minutes at the pace. The site adds ~2,200 a day, so the window
	// holds about a day and a half of intake: a posting sits in it for several runs at any
	// sensible cadence, and it is the way into the catalogue for every posting the site lists.
	jobbankcaListingPages = 160
	// jobbankcaFreshPerRun and jobbankcaBacklogPerRun split one run's posting pages (~190 KB
	// each) into hydrationLanes' two lanes: 350 pages are ~29 minutes at the pace, which with
	// the listing window fits a one-hour run. The backlog lane covers the postings the
	// pipeline discards unsaved, which would otherwise be bought again by every run.
	jobbankcaFreshPerRun    = 300
	jobbankcaBacklogPerRun  = 50
	jobbankcaDeadlineMargin = 5 * time.Minute

	// A refused or failed request is asked again up to jobbankcaRetries times.
	jobbankcaRetries      = 2
	jobbankcaRetryBackoff = 5 * time.Second

	// jobbankcaSweepGrace is how long a posting may go unlisted before the sweep closes it. The
	// listing window is the newest postings only, so a stored posting stops being refreshed a
	// couple of days after it appears; postings run 7 to 30 days (validThrough) and one past its
	// deadline leaves search by that date, so the grace outlasts the longest advertised life.
	jobbankcaSweepGrace = 35 * 24 * time.Hour
)

// NewJobBankCA builds the Job Bank adapter over the given client.
func NewJobBankCA(c HTMLGetter) Source {
	return jobbankca{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(jobbankcaRequestInterval), 1),
		listingPages:   jobbankcaListingPages,
		freshPerRun:    jobbankcaFreshPerRun,
		backlogPerRun:  jobbankcaBacklogPerRun,
		deadlineMargin: jobbankcaDeadlineMargin,
		retryBackoff:   jobbankcaRetryBackoff,
		now:            time.Now,
	}
}

func (jobbankca) Provider() string { return "jobbankca" }

// One national board, no per-tenant id.
func (jobbankca) Boardless() {}

// Thousands of employers post here, each named per posting.
func (jobbankca) Aggregator() {}

// The crawl lists a window of the board, not all of it; see jobbankcaSweepGrace.
func (jobbankca) SweepGrace() time.Duration { return jobbankcaSweepGrace }

// jobbankcaPostingID reads the id out of a listing link, /jobsearch/jobposting/<id>, which the
// site follows with a ;jsessionid=... path parameter and a query.
var jobbankcaPostingID = regexp.MustCompile(`/jobsearch/jobposting/(\d+)`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s jobbankca) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the newest listing window, refreshes the postings already stored and reads the
// pages hydrationLanes picks out of the rest. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s jobbankca) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	ids, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []string
	for _, id := range ids {
		if seen(id) {
			jobs = append(jobs, Job{ExternalID: id, URL: jobbankcaPostingLink(id), SeenRefresh: true})
			continue
		}
		unseen = append(unseen, id)
	}
	sortJobBankIDsNewestFirst(unseen)
	picked := hydrationLanes(unseen, s.freshPerRun, s.backlogPerRun, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, 1, func(id string) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, id)
		if err != nil {
			log.Printf("jobbankca: posting %s failed; deferring: %v", id, err)
			return unreadableDetail(id, jobbankcaPostingLink(id), ""), true
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if requested := int64(len(picked)) - outOfTime.Load(); requested > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("jobbankca: requested %d posting pages and none answered", requested)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("jobbankca: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred := len(unseen) - len(picked); deferred > 0 {
		log.Printf("jobbankca: %d new postings past this run's bound of %d, deferred to later runs", deferred, len(picked))
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin the crawl leaves to
// the pipeline's own writes.
func (s jobbankca) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

func jobbankcaPostingLink(id string) string { return fmt.Sprintf(jobbankcaPostingURL, id) }

// sortJobBankIDsNewestFirst orders posting ids so the hydration bound spends itself on the
// freshest: ids grow with the posting's date.
func sortJobBankIDsNewestFirst(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		a, _ := strconv.ParseInt(ids[i], 10, 64)
		b, _ := strconv.ParseInt(ids[j], 10, 64)
		return a > b
	})
}

// list walks the newest listing pages and returns their posting ids, deduplicated, newest
// first. The first page failing, or naming nothing, fails the crawl; a later page failing ends
// the walk with what was gathered, which is only a shorter window. The run's deadline ends it
// the same way, leaving the margin to the posting pages.
func (s jobbankca) list(ctx context.Context) ([]string, error) {
	var out []string
	listed := map[string]bool{}
	for page := 1; page <= s.listingPages; page++ {
		if page > 1 && s.nearDeadline(ctx) {
			log.Printf("jobbankca: listing stopped at page %d as the run neared its deadline", page)
			break
		}
		rows, err := s.listingPage(ctx, page)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("jobbankca: listing page 1: %w", err)
			}
			log.Printf("jobbankca: listing page %d failed; keeping the %d postings read: %v", page, len(out), err)
			break
		}
		if len(rows) == 0 {
			if page == 1 {
				return nil, errors.New("jobbankca: listing page 1 names no posting")
			}
			break // past the last page
		}
		for _, id := range rows {
			if !listed[id] {
				listed[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

// listingPage reads the posting ids of one listing page.
func (s jobbankca) listingPage(ctx context.Context, page int) ([]string, error) {
	root, err := s.get(ctx, fmt.Sprintf(jobbankcaListingURL, page))
	if err != nil {
		return nil, err
	}
	var ids []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" && hasClass(n, "resultJobItem") {
			if m := jobbankcaPostingID.FindStringSubmatch(Attr(n, "href")); m != nil {
				ids = append(ids, m[1])
			}
			return false
		}
		return true
	})
	return ids, nil
}

// hasClass reports whether the element's class list names class.
func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(Attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// get fetches one page at the pace, asking again when the answer is a refusal or a failure.
func (s jobbankca) get(ctx context.Context, url string) (*html.Node, error) {
	var root *html.Node
	err := retryWhile(ctx, jobbankcaRetries, s.retryBackoff, jobbankcaRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		var err error
		root, err = s.http.GetHTML(ctx, url)
		return err
	})
	return root, err
}

// jobbankcaRefused reports an answer worth asking again: a refusal, a rate limit or a server
// failure. Through the rotating proxy a refusal is one bad exit, not a verdict on the posting.
func jobbankcaRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the
// posting is gone (404/410) or a page whose posting carries no title, employer or body; an
// error is the absence of an answer, including a 200 that is not a posting page at all.
func (s jobbankca) detail(ctx context.Context, id string) (Job, bool, error) {
	root, err := s.get(ctx, jobbankcaPostingLink(id))
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if firstByAttr(root, "typeof", "JobPosting") == nil && firstByAttr(root, "property", "title") == nil {
		return Job{}, false, errors.New("page carries no posting")
	}
	job, ok := jobbankcaJob(root, id)
	return job, ok, nil
}

// jobbankcaProp is the text of the first element carrying RDFa property prop under root.
func jobbankcaProp(root *html.Node, prop string) string {
	if n := firstByAttr(root, "property", prop); n != nil {
		return collapseSpace(textContent(n))
	}
	return ""
}

// jobbankcaJob maps a posting page to a Job. ok is false for one with no title, employer or
// body — which is how a posting hidden by its partner (an empty description) reads too.
func jobbankcaJob(root *html.Node, id string) (Job, bool) {
	title := jobbankcaProp(root, "title")
	company := ""
	if org := firstByAttr(root, "property", "hiringOrganization"); org != nil {
		company = jobbankcaVisibleText(firstByAttr(org, "property", "name"))
	}
	body := jobbankcaBody(root)
	if title == "" || company == "" || body == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:  id,
		URL:         jobbankcaPostingLink(id),
		Title:       title,
		Company:     company,
		Description: body,
		Location:    jobbankcaPlaces(root),
		Countries:   countriesFromCodes([]string{"CA"}),
	}
	j.PostedAt = parseLayout("January 2, 2006", strings.TrimSpace(strings.TrimPrefix(jobbankcaProp(root, "datePosted"), "Posted on")))
	// Not parseLayout: it drops a date in the future, which a deadline is.
	if d, err := time.Parse("2006-01-02", jobbankcaProp(root, "validThrough")); err == nil {
		j.ApplicationDeadline = &d
	}
	switch mode := strings.ToLower(jobbankcaWorkLocation(root)); {
	case strings.Contains(mode, "hybrid"):
		j.WorkMode = "hybrid"
	case strings.Contains(mode, "remote"), strings.Contains(mode, "telework"):
		j.WorkMode, j.Remote = "remote", true
	}
	j.EmploymentType = jobbankcaEmploymentType(root)
	jobbankcaSalary(root, &j)
	return j, true
}

// jobbankcaBody is the posting's hidden `description` property: HTML, escaped, when the partner
// it came from sent markup, otherwise plain text. A direct Job Bank posting carries its
// structured sections flattened into that text.
func jobbankcaBody(root *html.Node) string {
	text := strings.TrimSpace(jobbankcaPropRaw(root, "description"))
	if text == "" {
		return ""
	}
	if jobbankcaMarkup.MatchString(text) {
		return sanitizeHTML(text)
	}
	return sanitizeHTML(plainTextToHTML(text))
}

// jobbankcaMarkup recognises a description that is markup: an opening block or line-break tag.
var jobbankcaMarkup = regexp.MustCompile(`(?i)<(p|div|br|ul|ol|li|h[1-6]|strong|b)[\s>/]`)

// jobbankcaPropRaw is the text of the first element carrying property prop, whitespace kept.
func jobbankcaPropRaw(root *html.Node, prop string) string {
	if n := firstByAttr(root, "property", prop); n != nil {
		return textContent(n)
	}
	return ""
}

// jobbankcaWorkLocation is the posting's stated work arrangement, the value beside the "Work
// location" label ("On site", "Hybrid"). Partners' postings read "On site" by default, so only
// the two values that state something are used.
func jobbankcaWorkLocation(root *html.Node) string {
	var value string
	walk(root, func(n *html.Node) bool {
		if value != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "li" {
			if text := collapseSpace(textContent(n)); strings.HasPrefix(text, "Work location") {
				value = strings.TrimSpace(strings.TrimPrefix(text, "Work location"))
				return false
			}
		}
		return true
	})
	return value
}

// jobbankcaEmploymentType reads the hours, the second line of the terms ("Permanent employment",
// then "Full time"); the first line is the tenure and says nothing the vocabulary holds.
func jobbankcaEmploymentType(root *html.Node) string {
	terms := firstByAttr(root, "property", "employmentType")
	if terms == nil {
		return ""
	}
	text := strings.ToLower(collapseSpace(textContent(terms)))
	switch {
	case strings.HasSuffix(text, "full time"):
		return "full_time"
	case strings.HasSuffix(text, "part time"):
		return "part_time"
	}
	return ""
}

// jobbankcaSalary reads the structured pay: the figures sit in the `content` attributes (with
// thousands separators), the period in unitText. Job Bank states the lowest figure alone for a
// fixed wage and both for a range. A period outside freehire's four is left to the dictionaries.
func jobbankcaSalary(root *html.Node, j *Job) {
	min := jobbankcaAmount(root, "minValue")
	max := jobbankcaAmount(root, "maxValue")
	if min == nil {
		return
	}
	period := ""
	switch strings.ToUpper(jobbankcaProp(root, "unitText")) {
	case "HOUR":
		period = "hour"
	case "DAY":
		period = "day"
	case "MONTH":
		period = "month"
	case "YEAR":
		period = "year"
	}
	if period == "" {
		return
	}
	j.SalaryMin, j.SalaryMax = min, max
	j.SalaryCurrency, j.SalaryPeriod = "CAD", period
}

// jobbankcaAmount reads one figure from an element's `content` attribute, nil when absent.
func jobbankcaAmount(root *html.Node, prop string) *int {
	n := firstByAttr(root, "property", prop)
	if n == nil {
		return nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(Attr(n, "content")), ",", ""), 64)
	if err != nil {
		return nil
	}
	return roundSalaryPart(v)
}

// jobbankcaVisibleText is the text of n without the screen-reader notes the site writes beside a
// link ("This link will open in a new window"), which a plain text read of an employer's name
// would otherwise append.
func jobbankcaVisibleText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode && hasClass(n, "wb-inv") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return collapseSpace(b.String())
}

// jobbankcaPlaces renders each address of the posting as "City, PR, Canada" and joins the
// distinct ones with "; " (a "Various locations" posting names several). The region is written
// ", QC" after a comma of its own, so stray commas are trimmed. A posting that names no address
// carries a plain label ("Montréal Island") beside the "Location" heading, which is read instead.
func jobbankcaPlaces(root *html.Node) string {
	var places []string
	seen := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && Attr(n, "property") == "address" {
			region := strings.Trim(jobbankcaProp(n, "addressRegion"), ", ")
			place := joinNonEmpty(jobbankcaProp(n, "addressLocality"), region, "Canada")
			if !seen[place] {
				seen[place] = true
				places = append(places, place)
			}
			return false
		}
		return true
	})
	if len(places) > 0 {
		return strings.Join(places, "; ")
	}
	var label string
	walk(root, func(n *html.Node) bool {
		if label != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "li" {
			if text := collapseSpace(textContent(n)); strings.HasPrefix(text, "Location") {
				label = strings.TrimSpace(strings.TrimPrefix(text, "Location"))
				return false
			}
		}
		return true
	})
	return joinNonEmpty(label, "Canada")
}
