package sources

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/time/rate"
)

// civilServiceJobs adapts Civil Service Jobs (www.civilservicejobs.service.gov.uk), the UK
// government's own vacancy board: every department and agency of the Civil Service, ~1.4k open
// postings. It is boardless and an aggregator — the employer is the department each card names.
// See the "Civil Service Jobs traps" section of AGENTS.md for the measurements behind each rule.
//
// Every path sits behind an ALTCHA proof-of-work check ("Quick check needed"): a session must
// find the number whose hash the challenge names before the site serves anything else. That
// is a computation, not a puzzle for a person — the site's own widget runs the same search in
// the browser — so the adapter solves it once per run (openSession) and carries the cookie.
type civilServiceJobs struct {
	http         csjHTTP
	limiter      waiter
	maxNewPerRun int
}

// csjHTTP is the transport the adapter needs: raw GETs and a raw form POST, both returning the
// body undecoded, since the site serves Windows-1252 and the shared HTML getters assume UTF-8.
// In production it is a cookie-jar client — the proof-of-work verdict is a session cookie.
type csjHTTP interface {
	TextGetter
	FormTextPoster
}

const csjBaseURL = "https://www.civilservicejobs.service.gov.uk"

// csjRequestInterval paces every request of a run, listing and detail alike, through one
// limiter. Unmeasured as a limit: the full 57-page listing at one page a second drew no
// refusal and no second challenge. It is courtesy toward a slow origin (a posting page took
// ~4 s to answer) rather than a budget we know.
const csjRequestInterval = time.Second

// csjDetailWorkers keeps several slow posting pages in flight so the limiter, not latency,
// sets the pace: at ~4 s a page, one worker would manage a quarter of a request a second.
const csjDetailWorkers = 4

// csjMaxNewPerRun bounds how many unseen postings one run hydrates. The first run faces the
// whole ~1.4k catalogue at one page a second, which fits the 50-minute run timeout only while
// the origin stays fast; a run that hits the timeout saves nothing, so the backlog is taken in
// slices. The postings past the bound are not stored this run and are hydrated by the next.
const csjMaxNewPerRun = 600

// csjMaxPages is the listing walk's safety ceiling — 25 postings a page, ~57 pages today.
// Reaching it without the pager running out FAILS the crawl, as fullCatalog requires.
var csjMaxPages = 200

// NewCivilServiceJobs builds the adapter over a cookie-jar client (see cookieSessionSource).
func NewCivilServiceJobs(c csjHTTP) Source {
	return civilServiceJobs{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(csjRequestInterval), 1),
		maxNewPerRun: csjMaxNewPerRun,
	}
}

func (civilServiceJobs) Provider() string { return "civilservicejobs" }

// One national board, no per-tenant id.
func (civilServiceJobs) boardless() {}

// Hundreds of departments and agencies post here, each named per card.
func (civilServiceJobs) aggregator() {}

// The empty search lists the whole catalogue, and listCards fails unless it read at least as
// many distinct postings as the site says it holds — so an unseen posting is gone.
func (civilServiceJobs) fullCatalog() {}

// csjCard is one listing row: everything the search page states about a posting.
type csjCard struct {
	id, title, department, location, salary string
}

func (c csjCard) url() string { return csjBaseURL + "/csr/jobs.cgi?jcode=" + c.id }

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s civilServiceJobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the whole catalogue, refreshes the postings already stored and hydrates up to
// maxNewPerRun of the rest. A posting whose page cannot be read is not yielded: nothing is
// stored for it yet, so the next run simply tries again.
func (s civilServiceJobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := s.listCards(ctx)
	if err != nil {
		return nil, err
	}
	var unseen []csjCard
	var refreshed []Job
	for _, c := range cards {
		if seen(c.id) {
			refreshed = append(refreshed, Job{ExternalID: c.id, URL: c.url(), Title: c.title, Company: c.department, SeenRefresh: true})
			continue
		}
		if len(unseen) < s.maxNewPerRun {
			unseen = append(unseen, c)
		}
	}
	jobs := fetchDetails(unseen, csjDetailWorkers, func(c csjCard) (Job, bool) {
		page, err := s.get(ctx, c.url())
		if err != nil {
			return Job{}, false
		}
		return csjDetail(page, c)
	})
	return append(jobs, refreshed...), nil
}

// get fetches a page through the run's limiter and transcodes it to UTF-8.
func (s civilServiceJobs) get(ctx context.Context, u string) (string, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return "", err
	}
	raw, err := s.http.GetText(ctx, u)
	if err != nil {
		return "", err
	}
	return csjDecode(raw)
}

// post is get's form counterpart.
func (s civilServiceJobs) post(ctx context.Context, u string, values url.Values) (string, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return "", err
	}
	raw, err := s.http.PostFormText(ctx, u, values)
	if err != nil {
		return "", err
	}
	return csjDecode(raw)
}

// csjDecode reads a page as Windows-1252, which is what the site serves without declaring it:
// "£" arrives as the single byte 0xA3 and the curly quotes and dashes as 0x91-0x97, all of
// which a UTF-8 parse turns into U+FFFD.
func csjDecode(raw string) (string, error) {
	s, err := charmap.Windows1252.NewDecoder().String(raw)
	if err != nil {
		return "", fmt.Errorf("civilservicejobs: decode Windows-1252: %w", err)
	}
	return s, nil
}

// listCards runs the empty search and walks every result page. Any page failing fails the
// crawl, and so does a walk that read fewer distinct postings than the first page's own total:
// the provider is swept by source (fullCatalog), so a short walk would close what it missed.
// More than the total is fine — a posting published mid-walk shifts the pages and the total
// was read before it existed.
func (s civilServiceJobs) listCards(ctx context.Context) ([]csjCard, error) {
	home, err := s.openSession(ctx)
	if err != nil {
		return nil, err
	}
	action, values, err := csjSearchForm(home)
	if err != nil {
		return nil, err
	}
	page, err := s.post(ctx, action, values)
	if err != nil {
		return nil, fmt.Errorf("civilservicejobs: search: %w", err)
	}
	var cards []csjCard
	ids := make(map[string]bool)
	total := -1
	for n := 1; ; n++ {
		root, err := html.Parse(strings.NewReader(page))
		if err != nil {
			return nil, fmt.Errorf("civilservicejobs: results page %d: %w", n, err)
		}
		if total < 0 {
			if total, err = csjTotal(root); err != nil {
				return nil, err
			}
		}
		for _, c := range csjCards(root) {
			if !ids[c.id] {
				ids[c.id] = true
				cards = append(cards, c)
			}
		}
		next := csjNextPage(root)
		if next == "" {
			break
		}
		if n >= csjMaxPages {
			return nil, fmt.Errorf("civilservicejobs: still paging after %d pages", csjMaxPages)
		}
		if page, err = s.get(ctx, next); err != nil {
			return nil, fmt.Errorf("civilservicejobs: results page %d: %w", n+1, err)
		}
	}
	if len(cards) < total {
		return nil, fmt.Errorf("civilservicejobs: read %d distinct postings of the %d the search reported", len(cards), total)
	}
	return cards, nil
}

// errCSJChallenge is a page that is still the proof-of-work interstitial.
var errCSJChallenge = errors.New("civilservicejobs: the site answered with its proof-of-work check")

// openSession loads the home page, passing the proof-of-work check when the site asks for it,
// and returns the home page itself — which carries the search form.
func (s civilServiceJobs) openSession(ctx context.Context) (string, error) {
	page, err := s.get(ctx, csjBaseURL+"/csr/index.cgi")
	if err != nil {
		return "", fmt.Errorf("civilservicejobs: home page: %w", err)
	}
	if !csjIsChallenge(page) {
		return page, nil
	}
	token := csjRequestTokenPattern.FindStringSubmatch(page)
	if token == nil {
		return "", errors.New("civilservicejobs: the proof-of-work page carries no originalRequestToken")
	}
	raw, err := s.get(ctx, csjBaseURL+"/csr/index.cgi?ProtectCaptcha=1")
	if err != nil {
		return "", fmt.Errorf("civilservicejobs: proof-of-work challenge: %w", err)
	}
	var ch altchaChallenge
	if err := json.Unmarshal([]byte(raw), &ch); err != nil {
		return "", fmt.Errorf("civilservicejobs: proof-of-work challenge: %w", err)
	}
	solution, err := solveAltcha(ch)
	if err != nil {
		return "", fmt.Errorf("civilservicejobs: %w", err)
	}
	home, err := s.post(ctx, csjBaseURL+"/csr/index.cgi", url.Values{
		"altcha":               {solution},
		"originalRequestToken": {token[1]},
	})
	if err != nil {
		return "", fmt.Errorf("civilservicejobs: proof-of-work answer: %w", err)
	}
	if csjIsChallenge(home) {
		return "", errCSJChallenge
	}
	return home, nil
}

// csjIsChallenge recognises the interstitial by the form that carries the widget.
func csjIsChallenge(page string) bool {
	return strings.Contains(page, `id="captcha-form"`)
}

var csjRequestTokenPattern = regexp.MustCompile(`name="originalRequestToken" value="([^"]+)"`)

// altchaChallenge is the challenge an ALTCHA server issues: find the number in [0, maxnumber]
// whose hash, salted, equals challenge. The signature is the server's own and is echoed back.
type altchaChallenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	MaxNumber int    `json:"maxnumber"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// solveAltcha searches the challenge's number range and returns the base64 JSON payload the
// widget submits in its "altcha" form field. With maxnumber 300000 the search is a fraction of
// a second.
func solveAltcha(ch altchaChallenge) (string, error) {
	var newHash func() hash.Hash
	switch ch.Algorithm {
	case "SHA-512":
		newHash = sha512.New
	case "SHA-256":
		newHash = sha256.New
	case "SHA-1":
		newHash = sha1.New
	default:
		return "", fmt.Errorf("proof-of-work: unknown algorithm %q", ch.Algorithm)
	}
	want, err := hex.DecodeString(ch.Challenge)
	if err != nil {
		return "", fmt.Errorf("proof-of-work: challenge is not hex: %w", err)
	}
	h := newHash()
	for n := 0; n <= ch.MaxNumber; n++ {
		h.Reset()
		h.Write([]byte(ch.Salt + strconv.Itoa(n)))
		if !bytes.Equal(h.Sum(nil), want) {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"algorithm": ch.Algorithm,
			"challenge": ch.Challenge,
			"number":    n,
			"salt":      ch.Salt,
			"signature": ch.Signature,
		})
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(payload), nil
	}
	return "", fmt.Errorf("proof-of-work: no number up to %d solves the challenge", ch.MaxNumber)
}

// csjSearchForm reads the home page's quick-search form — its action and every hidden field —
// and leaves "what" and "where" empty, which is the site's own way of asking for everything.
// The hidden fields are read rather than restated so a renamed one follows the site.
func csjSearchForm(home string) (string, url.Values, error) {
	root, err := html.Parse(strings.NewReader(home))
	if err != nil {
		return "", nil, fmt.Errorf("civilservicejobs: home page: %w", err)
	}
	form := firstByID(root, "ID_context_search_form")
	if form == nil {
		return "", nil, errors.New("civilservicejobs: the home page carries no search form")
	}
	values := url.Values{}
	walk(form, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "input" && Attr(n, "type") == "hidden" {
			values.Set(Attr(n, "name"), Attr(n, "value"))
		}
		return true
	})
	values.Set("what", "")
	values.Set("where", "")
	values.Set("search_button", "Search for jobs")
	return Attr(form, "action"), values, nil
}

var csjTotalPattern = regexp.MustCompile(`^([\d,]+) Search results?$`)

// csjTotal reads the result count from the page heading ("1,412 Search results").
func csjTotal(root *html.Node) (int, error) {
	h1 := firstByTag(root, "h1")
	if h1 == nil {
		return 0, errors.New("civilservicejobs: the results page has no heading")
	}
	m := csjTotalPattern.FindStringSubmatch(textContent(h1))
	if m == nil {
		return 0, fmt.Errorf("civilservicejobs: unreadable result count %q", textContent(h1))
	}
	return strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
}

// csjNextPage returns the pager's "next" link, "" on the last page.
func csjNextPage(root *html.Node) string {
	var next string
	walk(root, func(n *html.Node) bool {
		if next == "" && n.Type == html.ElementNode && n.Data == "a" && Attr(n, "title") == "Go to next search results page" {
			next = Attr(n, "href")
		}
		return next == ""
	})
	return next
}

// csjRefPattern reads a card's posting id from the title link's aria-describedby, which names
// the hidden "<department>, Reference : <ref>" element as dref-<ref>-<jcode>. The link's own
// href is a signed, session-bound SID; jcode is what the stable jobs.cgi?jcode= URL takes.
var csjRefPattern = regexp.MustCompile(`^dref-[^-]+-(\d+)$`)

// csjLocationTail is what the site appends to a location it truncated for the card.
const csjLocationTail = "... see the job advert for full location information"

// csjCards reads every result card on a page.
func csjCards(root *html.Node) []csjCard {
	var cards []csjCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !HasClass(n, "search-results-job-box") {
			return true
		}
		title := firstByClass(n, "search-results-job-box-title")
		if title == nil {
			return false
		}
		a := firstByTag(title, "a")
		if a == nil {
			return false
		}
		m := csjRefPattern.FindStringSubmatch(Attr(a, "aria-describedby"))
		if m == nil {
			return false
		}
		c := csjCard{
			id:         m[1],
			title:      textContent(a),
			department: csjCardField(n, "search-results-job-box-department"),
			location:   strings.TrimSpace(strings.TrimSuffix(csjCardField(n, "search-results-job-box-location"), csjLocationTail)),
			salary:     csjCardField(n, "search-results-job-box-salary"),
		}
		if c.title != "" {
			cards = append(cards, c)
		}
		return false
	})
	return cards
}

// csjCardField reads a card field's value without its label: the <h4> heading and the
// aria-hidden " : " separator are skipped.
func csjCardField(card *html.Node, class string) string {
	field := firstByClass(card, class)
	if field == nil {
		return ""
	}
	var b strings.Builder
	walk(field, func(n *html.Node) bool {
		if n.Type == html.ElementNode && (n.Data == "h4" || Attr(n, "aria-hidden") == "true") {
			return false
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		return true
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

// csjFacts are the side-panel fields worth keeping in the body, in the page's own order.
var csjFacts = []string{"Job grade", "Contract type", "Length of employment", "Business area", "Type of role", "Working pattern", "Number of jobs available"}

// csjDetail builds the posting from its page. A page without the posting's title is not a
// posting — a closed one answers 200 with the sign-in page — and is not yielded.
func csjDetail(page string, c csjCard) (Job, bool) {
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return Job{}, false
	}
	h1 := firstByID(root, "id_common_page_title_h1")
	panel := firstByClass(root, "vac_display_panel_main_inner")
	if h1 == nil || panel == nil {
		return Job{}, false
	}
	fields := csjFields(root)

	var body strings.Builder
	body.WriteString("<ul>")
	for _, label := range csjFacts {
		if v := fields[label]; v != "" {
			fmt.Fprintf(&body, "<li>%s: %s</li>", html.EscapeString(label), html.EscapeString(v))
		}
	}
	body.WriteString("</ul>")
	for n := panel.FirstChild; n != nil; n = n.NextSibling {
		// The table of contents only links to the sections that follow it.
		if id := Attr(n, "id"); id == "main_panel_contents" || id == "top_section_link_list" {
			continue
		}
		_ = html.Render(&body, n)
	}

	j := Job{
		ExternalID:        c.id,
		URL:               c.url(),
		Title:             textContent(h1),
		Company:           c.department,
		Location:          c.location,
		Description:       sanitizeHTML(body.String()),
		EmploymentType:    csjEmploymentType(fields["Working pattern"]),
		FallbackCountries: []string{"gb"},
	}
	if j.Company == "" {
		if sub := firstByClass(root, "csr-page-subtitle"); sub != nil {
			j.Company = textContent(sub)
		}
	}
	if lo, hi, ok := csjSalary(c.salary); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "GBP", "year"
	}
	return j, j.Company != ""
}

// csjFields maps each labelled side-panel field (<h3>label</h3> + its value) to its text.
func csjFields(root *html.Node) map[string]string {
	fields := make(map[string]string)
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !HasClass(n, "vac_display_field") {
			return true
		}
		label, value := firstByTag(n, "h3"), firstByClass(n, "vac_display_field_value")
		if label != nil && value != nil {
			if k := textContent(label); fields[k] == "" {
				fields[k] = csjFieldValue(value)
			}
		}
		return false
	})
	return fields
}

// csjFieldValue reads a field's text, joining the lines of a multi-valued one with ", " — a
// role open on several terms lists them one per line ("Permanent", "Loan", "Secondment"), and
// plain text extraction would run them together.
func csjFieldValue(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) bool {
		switch {
		case c.Type == html.TextNode:
			b.WriteString(c.Data)
		case c.Type == html.ElementNode && (c.Data == "br" || c.Data == "li" || c.Data == "p" || c.Data == "div"):
			b.WriteString("\n")
		}
		return true
	})
	var lines []string
	for _, line := range strings.Split(b.String(), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, ", ")
}

// csjEmploymentType maps the "Working pattern" field when it names one schedule. It lists every
// pattern the role allows ("Flexible working, Full-time, Job share, Part-time" on most postings),
// so a role offered both full- and part-time states no single type. Flexible working and
// compressed hours are arrangements of a schedule rather than one.
func csjEmploymentType(pattern string) string {
	var types []string
	for _, p := range strings.Split(pattern, ",") {
		switch strings.TrimSpace(p) {
		case "Full-time":
			types = append(types, "full_time")
		case "Part-time", "Job share":
			types = append(types, "part_time")
		}
	}
	if len(types) == 0 {
		return ""
	}
	for _, t := range types[1:] {
		if t != types[0] {
			return ""
		}
	}
	return types[0]
}

var csjPoundPattern = regexp.MustCompile(`£([\d,]+)`)

// csjAnnualFloor separates an annual salary from the rest. The card states an amount and
// no period; across the whole catalogue (1,409 cards with a salary, 2026-10-23) every bound
// was an annual figure except two, £475 and £1,000, which are not. Below the floor the
// amount is not published rather than published under the wrong period.
const csjAnnualFloor = 5000

// csjSalary parses the card's "£59,207 to £70,958" (or a single "£34,000").
func csjSalary(s string) (lo, hi int, ok bool) {
	m := csjPoundPattern.FindAllStringSubmatch(s, -1)
	if len(m) == 0 || len(m) > 2 {
		return 0, 0, false
	}
	lo, err := strconv.Atoi(strings.ReplaceAll(m[0][1], ",", ""))
	if err != nil {
		return 0, 0, false
	}
	hi, err = strconv.Atoi(strings.ReplaceAll(m[len(m)-1][1], ",", ""))
	if err != nil || lo < csjAnnualFloor || hi < lo {
		return 0, 0, false
	}
	return lo, hi, true
}
