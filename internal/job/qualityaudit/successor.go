package qualityaudit

import (
	"context"
	"html"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// The successor pass looks for chronic boards that did not die but moved: an employer leaves one
// ATS for another, the old host keeps answering (or refuses the crawler), and our catalogue keeps
// crawling a board whose postings now live elsewhere. It probes each board's newest posting page
// and its host's home, follows where they point, and names the board the target is, so a human
// can add it and retire the old one. Like the rest of this package it measures and never acts.

// ChronicBoard is one board the crawl has not reached for the successor window.
type ChronicBoard struct {
	Provider, Board, Region, Company string
	OpenJobs                         int64
	// LatestURL is the board's newest posting page; the pass probes it and its host's home.
	LatestURL    string
	FailingSince time.Time
	LastError    string
}

// Move is a chronic board whose pages point to another host. SuccessorProvider and
// SuccessorBoard are the target as the catalogue would key it, empty when the target is not a
// recognised ATS.
type Move struct {
	Board                             ChronicBoard
	ProbedURL, Target                 string
	Evidence                          string // EvidenceHTTPRedirect or EvidencePageRedirect
	SuccessorProvider, SuccessorBoard string
}

const (
	EvidenceHTTPRedirect = "http-redirect"
	EvidencePageRedirect = "page-redirect"
)

// maxPage is how much of a page is read: a career site's home with inline config (a Jibe page
// measured 345 KB) has to fit whole, or its platform marker is cut off.
const maxPage = 1 << 20

// SuccessorFinder probes chronic boards. Direct makes every request and must be a safehttp
// client: the second hop comes from a third party's page. Fallback, when set (the Scrapling
// tier), retries only a first-hop page the host REFUSED (403/429/503): such a host answered
// Direct, so it passed the SSRF guard, and the URL came from our own catalogue. A page's
// redirect target is never fetched through Fallback, which dials past the guard.
type SuccessorFinder struct {
	Direct, Fallback *http.Client
	// Name says which catalogue board a page is (provider and board, ok false when none): by its
	// URL, and by a self-hosted career site's platform marker in body when body is not empty. The
	// ATS knowledge lives in internal/ingest, a layer above this package, so the caller supplies it.
	Name          func(u *url.URL, body string) (provider, board string, ok bool)
	MaxRequests   int64
	HostInterval  time.Duration
	MaxCrawlDelay time.Duration
	HostWorkers   int
}

// Run probes every board and returns the moves found, in no particular order. Boards are
// grouped by their posting host, so one host's boards share its robots.txt and pacing.
func (f *SuccessorFinder) Run(ctx context.Context, boards []ChronicBoard) []Move {
	hosts := map[string][]ChronicBoard{}
	for _, b := range boards {
		u, err := url.Parse(b.LatestURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		key := u.Scheme + "://" + strings.ToLower(u.Host)
		hosts[key] = append(hosts[key], b)
	}
	order := make([]string, 0, len(hosts))
	for h := range hosts {
		order = append(order, h)
	}
	rand.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] }) //nolint:gosec // Only orders which hosts a capped run reaches.

	var budget atomic.Int64
	budget.Store(f.MaxRequests)
	var moves []Move
	var mu sync.Mutex
	var wg sync.WaitGroup
	work := make(chan string)
	for range max(f.HostWorkers, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for origin := range work {
				m := f.host(ctx, origin, hosts[origin], &budget)
				mu.Lock()
				moves = append(moves, m...)
				mu.Unlock()
			}
		}()
	}
	for _, h := range order {
		work <- h
	}
	close(work)
	wg.Wait()
	return moves
}

// host probes one host's boards, one request at a time, within its robots.txt.
func (f *SuccessorFinder) host(ctx context.Context, origin string, boards []ChronicBoard, budget *atomic.Int64) []Move {
	p := &pacer{interval: f.HostInterval, budget: budget}
	robotsPage, ok := f.get(ctx, p, f.Direct, origin+"/robots.txt")
	if !ok {
		return nil
	}
	rules := robotsFromStatus(robotsPage.status, robotsPage.body)
	if rules.crawlDelay > f.MaxCrawlDelay {
		return nil
	}
	p.interval = max(p.interval, rules.crawlDelay)

	var moves []Move
	for _, b := range boards {
		m, found, ok := f.board(ctx, p, rules, b)
		if !ok {
			break
		}
		if found {
			moves = append(moves, m)
		}
	}
	return moves
}

// board probes one board's pages in turn and returns the first move to a recognised board, else
// the first move anywhere (a site that went to an ATS we have no adapter for reads the same). ok
// is false when the budget is spent or the run stopped.
func (f *SuccessorFinder) board(ctx context.Context, p *pacer, rules robots, b ChronicBoard) (m Move, found, ok bool) {
	var unknown *Move
	consider := func(move Move) bool {
		if sameBoard(b, move) {
			return false
		}
		target, _ := url.Parse(move.Target)
		probed, _ := url.Parse(move.ProbedURL)
		if target != nil && noisePage.MatchString(target.Host+target.Path) {
			return false
		}
		if move.SuccessorBoard == "" && sameSite(target, probed) {
			// A dead hosted tenant sent to its vendor's own site (acme.bamboohr.com -> bamboohr.com)
			// is the vendor saying the account is closed, not an employer moving.
			return false
		}
		if move.SuccessorBoard != "" {
			return true
		}
		if unknown == nil {
			unknown = &move
		}
		return false
	}
	for _, probe := range f.successorProbeURLs(b) {
		requested, _ := url.Parse(probe) // successorProbeURLs built it from a parsed URL
		if !rules.allowed(requested.RequestURI()) {
			continue
		}
		page, ok := f.get(ctx, p, f.Direct, probe)
		if !ok {
			return Move{}, false, false
		}
		if f.Fallback != nil && refused(page.status) {
			if page, ok = f.get(ctx, p, f.Fallback, probe); !ok {
				return Move{}, false, false
			}
		}
		if page.status == 0 {
			continue
		}
		move := Move{Board: b, ProbedURL: probe}
		if !sameHost(requested, page.final) {
			move.Evidence, move.Target = EvidenceHTTPRedirect, page.final.String()
			move.SuccessorProvider, move.SuccessorBoard = f.successorOf(requested, page.final, page.body)
			if consider(move) {
				return move, true, true
			}
			continue
		}
		for _, target := range pageRedirects(page.final, page.body) {
			if sameHost(target, page.final) {
				continue
			}
			// One request to a host this pass has no pacing for: it draws on the budget only.
			next, ok := f.get(ctx, &pacer{budget: p.budget}, f.Direct, target.String())
			if !ok {
				return Move{}, false, false
			}
			landed := target
			if next.status != 0 {
				landed = next.final
			}
			move.Evidence, move.Target = EvidencePageRedirect, landed.String()
			move.SuccessorProvider, move.SuccessorBoard = f.successorOf(requested, landed, next.body)
			if consider(move) {
				return move, true, true
			}
		}
	}
	if unknown != nil {
		return *unknown, true, true
	}
	return Move{}, false, true
}

// successorProbeURLs are the pages that show a move: the newest posting, the host's home when
// the host is the board itself (a tenant host, not a shared one like job-boards.greenhouse.io,
// whose home says nothing about this board), and iCIMS's search frame, where an iCIMS tenant
// that moved to Jibe sends the browser on with a script (bowheadcareers-uicalaska, 2026-11).
func (f *SuccessorFinder) successorProbeURLs(b ChronicBoard) []string {
	u, err := url.Parse(b.LatestURL)
	if err != nil || u.Host == "" {
		return nil
	}
	home := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}
	urls := []string{b.LatestURL}
	if src, board, ok := f.Name(home, ""); (ok && src == b.Provider && strings.EqualFold(board, b.Board)) ||
		strings.EqualFold(u.Hostname(), b.Board) {
		urls = append(urls, home.String())
	}
	if b.Provider == "icims" {
		urls = append(urls, u.Scheme+"://"+u.Host+"/jobs/search?in_iframe=1")
	}
	return urls
}

// successorOf names the board a page is: by its URL first, then by a self-hosted career site's
// platform marker. A marker is read only off the probed page's own site: on the old vendor's site
// it is that vendor's own (recruitee.com carries "teamtailor", and a dead Recruitee tenant is sent
// there).
func (f *SuccessorFinder) successorOf(probed, u *url.URL, body string) (provider, board string) {
	if sameSite(probed, u) {
		body = ""
	}
	provider, board, _ = f.Name(u, body)
	return provider, board
}

// sameBoard reports a target that is the old board again (a redirect to another of its own
// hosts), which is not a move.
func sameBoard(b ChronicBoard, m Move) bool {
	return m.SuccessorProvider == b.Provider && strings.EqualFold(m.SuccessorBoard, b.Board)
}

// pageRedirectRes find a redirect a page performs itself: a script assigning the location, or a
// meta refresh. iCIMS writes `window.top.location.href = 'https:\/\/…'` with escaped slashes.
var pageRedirectRes = []*regexp.Regexp{
	regexp.MustCompile(`location(?:\.href)?\s*=\s*["']([^"']+)["']`),
	regexp.MustCompile(`location\.(?:replace|assign)\(\s*["']([^"']+)["']`),
	regexp.MustCompile(`(?i)<meta[^>]+http-equiv=["']?refresh["']?[^>]*content=["'][^"']*?url=([^"'>\s]+)`),
}

// pageRedirects returns the absolute http(s) URLs a page redirects to, resolved against base.
func pageRedirects(base *url.URL, body string) []*url.URL {
	var out []*url.URL
	for _, re := range pageRedirectRes {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			raw := html.UnescapeString(strings.ReplaceAll(m[1], `\/`, "/"))
			u, err := base.Parse(raw)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				continue
			}
			out = append(out, u)
		}
	}
	return out
}

// noisePage is a target that is a vendor's own login, sign-up, maintenance or error page: a dead
// or suspended tenant sent there has not moved (community.workday.com/maintenance-page,
// login.prismhr-hire.com, app.careerplug.com/user/sign_in, measured 2026-11-26).
// It matches whole host labels or path segments, so /jobs/registered-nurse is not one.
var noisePage = regexp.MustCompile(`(?i)(^|[/.])(log-?in|sign[-_]?(in|up)|register|maintenance-page|invalid-url)([/.]|$)`)

// sameSite reports two URLs under one registrable domain.
func sameSite(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	da, errA := publicsuffix.EffectiveTLDPlusOne(a.Hostname())
	db, errB := publicsuffix.EffectiveTLDPlusOne(b.Hostname())
	return errA == nil && errB == nil && strings.EqualFold(da, db)
}

func sameHost(a, b *url.URL) bool {
	return strings.EqualFold(strings.TrimPrefix(a.Hostname(), "www."), strings.TrimPrefix(b.Hostname(), "www."))
}

// refused is a host turning the crawler away rather than answering about the page.
func refused(status int) bool {
	return status == http.StatusForbidden || status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

type page struct {
	status int // 0 when the fetch failed
	final  *url.URL
	body   string
}

// get waits its turn and fetches rawURL, following redirects, with up to maxPage of the body. ok
// is false only when the budget is spent or the run stopped.
func (f *SuccessorFinder) get(ctx context.Context, p *pacer, client *http.Client, rawURL string) (page, bool) {
	if !p.turn(ctx) {
		return page{}, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return page{}, true
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return page{}, true
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	return page{status: resp.StatusCode, final: resp.Request.URL, body: string(b)}, true
}
