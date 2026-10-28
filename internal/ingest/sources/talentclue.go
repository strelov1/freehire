package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// talentclue adapts Talent Clue, a Spanish ATS whose customers — McDonald's Spain, Decathlon,
// Grupo Quirónsalud, Henneo/Hiberus, Basque industrial cooperatives — publish their openings at
// https://<tenant>.talentclue.com. The board is the customer's 32-hex CLIENT ID, the key the
// platform's own embeddable widget reads its listing with:
//
//	POST api.talentclue.com/jswidget-ajax/jswidget/jobs/<client id>/<base64 JSON filter>
//
// which answers every open job of the client in one response, without a description. The id is
// printed nowhere but the client's company page (data-client-id on <tenant>/es/<company slug>),
// so a board is added by hand from that page; no posting URL carries it.
//
// The body is on the posting's own page (div.job-description), bought for new jobs only.
// robots.txt declares "Crawl-delay: 10" for every agent on both the API host and the tenant
// hosts, and disallows nothing this adapter reads, so the listing calls share one 10-second
// limiter across the provider and each board's page fetches run at the same delay.
type talentclue struct {
	api   HeaderJSONPoster
	pages HTMLGetter
	// apiLimiter spaces the listing calls of every board on the one API host.
	apiLimiter waiter
	// pageDelay spaces one board's posting-page fetches; zero disables pacing (tests).
	pageDelay      time.Duration
	maxNewPerRun   int
	deadlineMargin time.Duration
}

// talentclueHTTP is the transport talentclue needs: the listing API is a JSON POST, the posting
// pages are HTML.
type talentclueHTTP interface {
	HeaderJSONPoster
	HTMLGetter
}

// NewTalentclue builds the Talent Clue adapter over the given HTTP client.
func NewTalentclue(c talentclueHTTP) Source {
	return talentclue{
		api:            c,
		pages:          c,
		apiLimiter:     rate.NewLimiter(rate.Every(talentclueCrawlDelay), 1),
		pageDelay:      talentclueCrawlDelay,
		maxNewPerRun:   talentclueMaxNewPerBoard,
		deadlineMargin: talentclueDeadlineMargin,
	}
}

func (talentclue) Provider() string { return "talentclue" }

// fullBoardListing: one API call answers the client's whole open listing, with no page or
// offset to walk; it either succeeds or fails the crawl. See the fullBoardListing interface.
func (talentclue) fullBoardListing() {}

const (
	// talentclueCrawlDelay is robots.txt's Crawl-delay on the API and the tenant hosts.
	talentclueCrawlDelay = 10 * time.Second
	// talentclueMaxNewPerBoard bounds the pages one board buys in a run: at the crawl delay, 150
	// pages is 25 minutes, so McDonald's 828 openings (measured 2026-10-27) arrive over six runs.
	talentclueMaxNewPerBoard = 150
	// talentclueDeadlineMargin is the time hydration leaves the pipeline to write what it read.
	talentclueDeadlineMargin = 5 * time.Minute
	talentclueAPIBase        = "https://api.talentclue.com/jswidget-ajax/jswidget/jobs/"
)

var (
	talentclueBoard  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	talentclueNodeID = regexp.MustCompile(`^[0-9]+$`)
	talentclueXHR    = map[string]string{"X-Requested-With": "XMLHttpRequest"}
)

// talentclueFilter is the widget's default filter: every open job of the client and of its
// subgroups, in Spanish. The API reads it base64-encoded from the path.
var talentclueFilter = func() string {
	f, _ := json.Marshal(map[string]any{
		"op": 1, "position": []string{}, "cdef": false, "subs": "1", "lang": "es",
		"industry": []string{}, "department": []string{}, "contract": false, "countries": nil,
		"provinces": []string{}, "includeOnlyClosedJobs": false, "closedJobsOfTheLastMonths": "12",
		"showArchivedJobs": false,
	})
	return base64.StdEncoding.EncodeToString(f)
}()

// talentclueListing is the API's answer. "jobs" is an object keyed by the posting's node id,
// or an empty ARRAY when the client has no opening, so it is decoded in a second step.
type talentclueListing struct {
	Jobs json.RawMessage `json:"jobs"`
}

// talentclueJob is one listed posting.
type talentclueJob struct {
	URL          string `json:"url"`
	PostedAt     string `json:"post_date_timestamp"`
	Title        string `json:"title"`
	City         string `json:"city"`
	Province     string `json:"province_label"`
	Country      string `json:"country_label"`
	CountryISO   any    `json:"country_iso"` // a code, or false when unset
	Subgroup     string `json:"subgroup"`
	WorkModality string `json:"work_modality"`
	Contract     string `json:"contract_label"`
	Shift        string `json:"shift_label"`
	Archived     string `json:"is_archived"`
}

// talentcluePosting is a listed posting with its identity and the employer it is filed under.
type talentcluePosting struct {
	id, url, company string
	job              talentclueJob
}

// Fetch reads the listing and every posting's page, within the per-board budget.
func (s talentclue) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the listing and buys a posting page only for postings the catalogue does not
// hold, at most maxNewPerRun of them and none once the run nears its deadline; a seen posting is
// re-listed as a SeenRefresh carrying the listing's fields, and an unseen one past the budget is
// left for a later run.
func (s talentclue) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []talentcluePosting
	deferred := 0
	for _, p := range postings {
		switch {
		case seen(p.id):
			j := p.listed()
			j.SeenRefresh = true
			jobs = append(jobs, j)
		case len(fresh) < s.maxNewPerRun:
			fresh = append(fresh, p)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("talentclue: board %q: hydration budget %d reached; %d new postings deferred to a later run",
			e.Board, s.maxNewPerRun, deferred)
	}
	// One worker: the board's pages are spaced by the crawl delay anyway.
	get := pacedHTMLGetter(s.pages, s.pageDelay, 1)
	outOfTime := 0
	for _, p := range fresh {
		if s.nearDeadline(ctx) {
			outOfTime++
			continue
		}
		if j, ok := s.detail(ctx, get, p); ok {
			jobs = append(jobs, j)
		}
	}
	if outOfTime > 0 {
		log.Printf("talentclue: board %q: %d new postings left unread as the run neared its deadline, deferred to later runs",
			e.Board, outOfTime)
	}
	return jobs, nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s talentclue) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list reads the client's open postings. An unknown client id is the API's 400 and fails the
// crawl, as does any posting whose link is not a Talent Clue page.
func (s talentclue) list(ctx context.Context, e CompanyEntry) ([]talentcluePosting, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !talentclueBoard.MatchString(board) {
		return nil, fmt.Errorf("talentclue: board %q is not a 32-hex client id", e.Board)
	}
	if err := s.apiLimiter.Wait(ctx); err != nil {
		return nil, err
	}
	var resp talentclueListing
	if err := s.api.PostJSONWithHeaders(ctx, talentclueAPIBase+board+"/"+talentclueFilter, talentclueXHR, struct{}{}, &resp); err != nil {
		return nil, fmt.Errorf("talentclue: board %s: %w", board, err)
	}
	raw := strings.TrimSpace(string(resp.Jobs))
	if raw == "" || raw == "null" {
		return nil, fmt.Errorf("talentclue: board %s: the answer carries no job list", board)
	}
	if raw == "[]" {
		return nil, nil
	}
	var byID map[string]talentclueJob
	if err := json.Unmarshal(resp.Jobs, &byID); err != nil {
		return nil, fmt.Errorf("talentclue: board %s: job list: %w", board, err)
	}
	out := make([]talentcluePosting, 0, len(byID))
	for id, j := range byID {
		if j.Archived == "1" {
			continue
		}
		if !talentclueNodeID.MatchString(id) {
			return nil, fmt.Errorf("talentclue: board %s: posting key %q is not a node id", board, id)
		}
		u, ok := talentcluePageURL(j.URL)
		if !ok {
			return nil, fmt.Errorf("talentclue: board %s: posting %s links outside the platform: %q", board, id, j.URL)
		}
		out = append(out, talentcluePosting{
			id: id, url: u, job: j,
			company: firstNonEmpty(strings.TrimSpace(j.Subgroup), e.Company),
		})
	}
	return out, nil
}

// talentcluePageURL pins a listed posting link to the platform's own hosts over https, so
// nothing the API returns can point the page fetch elsewhere.
func talentcluePageURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, ".talentclue.com") {
		return "", false
	}
	u.Scheme, u.Host, u.Fragment = "https", host, ""
	return u.String(), true
}

// listed maps what the listing says about a posting; the body comes from its page.
func (p talentcluePosting) listed() Job {
	l := p.job
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          strings.TrimSpace(html.UnescapeString(l.Title)),
		Company:        p.company,
		Location:       joinNonEmpty(l.City, talentclueProvince(l), l.Country),
		PostedAt:       talentclueTime(l.PostedAt),
		WorkMode:       talentclueWorkMode(l.WorkModality),
		EmploymentType: talentclueEmploymentType(l.Contract, l.Shift),
	}
	if code, ok := l.CountryISO.(string); ok {
		j.Countries = countryFromCode(code)
	}
	j.Remote = j.WorkMode == "remote"
	return j
}

// detail fetches a posting's page and reads its body: div.job-description, the company intro,
// the offer and its requirements. A 404/410 drops the posting; any other failure, or a page
// without the block, is an unreadable marker — the page is the only source of the body.
func (s talentclue) detail(ctx context.Context, get HTMLGetter, p talentcluePosting) (Job, bool) {
	root, err := get.GetHTML(ctx, p.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.id, p.url, p.company), true
		}
		return Job{}, false
	}
	block := firstByClass(root, "job-description")
	if block == nil {
		return unreadableDetail(p.id, p.url, p.company), true
	}
	j := p.listed()
	j.Description = sanitizeHTML(innerHTML(block))
	if strings.TrimSpace(j.Description) == "" {
		return unreadableDetail(p.id, p.url, p.company), true
	}
	return j, true
}

// talentclueProvince is the province unless it merely repeats the city ("Zaragoza, Zaragoza").
func talentclueProvince(j talentclueJob) string {
	if strings.EqualFold(strings.TrimSpace(j.Province), strings.TrimSpace(j.City)) {
		return ""
	}
	return j.Province
}

// talentclueTime reads the listing's Unix-seconds publication time.
func talentclueTime(s string) *time.Time {
	sec, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

// talentclueWorkMode maps the platform's work-modality label; an empty one stays unknown.
func talentclueWorkMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "en remoto", "remoto", "teletrabajo":
		return "remote"
	case "híbrida", "hibrida":
		return "hybrid"
	case "presencial":
		return "onsite"
	default:
		return ""
	}
}

// talentclueEmploymentType reads the employment type from the contract label where it names
// one (a traineeship, a freelance contract) and otherwise from the working-day label; a
// contract's duration ("Indefinido", "Temporal") says nothing about it.
func talentclueEmploymentType(contract, shift string) string {
	switch strings.ToLower(strings.TrimSpace(contract)) {
	case "beca", "formativo de prácticas", "prácticas", "practicas":
		return "internship"
	case "contratista/freelance", "freelance":
		return "contract"
	}
	switch strings.ToLower(strings.TrimSpace(shift)) {
	case "jornada completa":
		return "full_time"
	case "jornada parcial":
		return "part_time"
	}
	return ""
}
