package qualityaudit

import (
	"cmp"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// statusBlind are the sources whose status the canary cannot vouch for: they 404 an id that
// never existed, so their host calibrates, but keep a CLOSED posting at 200 behind a banner — so
// every sampled posting would read live and the source 0% dead, the most confidently wrong
// number this audit could publish. Their samples are reported unmeasured. Each is measured in
// cmd/liveness, which reads that source's own closed signal; moving those readers into a shared
// package is the way to give one of these a number.
var statusBlind = map[string]string{
	"hh":     "an archived vacancy answers 200; the archived flag is in the page (cmd/liveness/hh.go)",
	"djinni": "a removed posting answers 200 with a 'no longer active' alert (cmd/liveness/djinni.go)",
	"torre":  "a closed posting answers 200 with its state in the page (cmd/liveness/torre.go)",
}

// Posting is one sampled posting: its stored URL is what gets probed.
type Posting struct {
	ID         int64
	Source     string
	ExternalID string
	URL        string
	Slug       string
}

// Result is what the audit learned about one posting.
type Result struct {
	Posting Posting
	Outcome Outcome
	Reason  Reason
}

// Auditor probes a sample of postings at their sources. The zero value is not usable: Client,
// MaxRequests, HostInterval, MaxCrawlDelay and HostWorkers must be set.
type Auditor struct {
	// Client makes every request. In production it is a safehttp client: stored URLs come from
	// third parties, Telegram posts among them.
	Client *http.Client
	// MaxRequests bounds the whole run — robots.txt, canaries and postings alike.
	MaxRequests int64
	// HostInterval spaces consecutive requests to one host; a longer Crawl-delay wins.
	HostInterval time.Duration
	// MaxCrawlDelay is the longest Crawl-delay the audit will wait out; a host asking for more
	// is skipped rather than holding a worker for the whole run.
	MaxCrawlDelay time.Duration
	// HostWorkers is how many hosts are probed at once. Within a host requests are sequential.
	HostWorkers int
}

// maxRobots is how much of a robots.txt is read: the 500 KiB RFC 9309 asks a crawler to parse at
// least, so a long file's later Disallow lines are not silently dropped.
const maxRobots = 500 << 10

// maxRedirects bounds the redirect hops one request follows (RFC 9309 asks for at least five).
const maxRedirects = 5

// maxDrain is how much of a response body is read before closing it, so a small page can return
// its connection to the pool. Nothing reads what is drained.
const maxDrain = 64 << 10

// Run probes every posting and returns one Result for each, in no particular order.
func (a *Auditor) Run(ctx context.Context, postings []Posting) []Result {
	results := make([]Result, 0, len(postings))
	hosts := map[string][]Posting{}
	for _, p := range postings {
		if _, blind := statusBlind[p.Source]; blind {
			results = append(results, Result{Posting: p, Reason: ReasonStatusBlind})
			continue
		}
		u, err := url.Parse(p.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			results = append(results, Result{Posting: p, Reason: ReasonBadURL})
			continue
		}
		key := u.Scheme + "://" + strings.ToLower(u.Host)
		hosts[key] = append(hosts[key], p)
	}

	// Shuffled, so when the budget runs out it is a different set of hosts each week that goes
	// unmeasured rather than always the same alphabetical tail.
	order := make([]string, 0, len(hosts))
	for h := range hosts {
		order = append(order, h)
	}
	rand.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	var budget atomic.Int64
	budget.Store(a.MaxRequests)
	work := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range max(a.HostWorkers, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for origin := range work {
				r := a.host(ctx, origin, hosts[origin], &budget)
				mu.Lock()
				results = append(results, r...)
				mu.Unlock()
			}
		}()
	}
	for _, h := range order {
		work <- h
	}
	close(work)
	wg.Wait()
	return results
}

// host probes one host's postings in order, one request at a time: robots.txt, then a canary,
// then the postings.
func (a *Auditor) host(ctx context.Context, origin string, postings []Posting, budget *atomic.Int64) []Result {
	host := origin[strings.Index(origin, "://")+3:]
	out := make([]Result, 0, len(postings))
	rest := func(from int, reason Reason) []Result {
		for _, p := range postings[from:] {
			out = append(out, Result{Posting: p, Reason: reason})
		}
		return out
	}
	p := pacer{interval: a.HostInterval, budget: budget}

	status, _, body, ok := p.get(ctx, p.client(a.Client, host, robots{}), origin+"/robots.txt", true)
	if !ok {
		return rest(0, p.stopReason(ctx))
	}
	if status == http.StatusForbidden || status == http.StatusTooManyRequests {
		// RFC 9309 reads a 4xx robots.txt as "no rules", but a host refusing or metering us at
		// its front door is not inviting the canary and the postings behind it.
		return rest(0, ReasonRefused)
	}
	rules := robotsFromStatus(status, body)
	client := p.client(a.Client, host, rules)
	if rules.crawlDelay > a.MaxCrawlDelay {
		return rest(0, ReasonRobots)
	}
	p.interval = max(p.interval, rules.crawlDelay)

	allowed := postings[:0:0]
	for _, post := range postings {
		u, _ := url.Parse(post.URL) // Run already parsed it
		if rules.allowed(u.RequestURI()) {
			allowed = append(allowed, post)
		} else {
			out = append(out, Result{Posting: post, Reason: ReasonRobots})
		}
	}
	postings = allowed
	if len(postings) == 0 {
		return out
	}

	if calibrated, stopped := calibrate(ctx, &p, client, rules, postings); !calibrated {
		return rest(0, cmp.Or(stopped, ReasonUncalibrated))
	}

	for i, post := range postings {
		status, final, _, ok := p.get(ctx, client, post.URL, false)
		if !ok {
			return rest(i, p.stopReason(ctx))
		}
		requested, _ := url.Parse(post.URL)
		outcome, reason := readStatus(requested, status, final)
		if reason == ReasonRefused {
			// The host is metering us. Its answer is not about the posting, and asking again is
			// how a polite probe becomes a ban for the crawl that shares its IP.
			return rest(i, ReasonRefused)
		}
		out = append(out, Result{Posting: post, Outcome: outcome, Reason: reason})
	}
	return out
}

// calibrate asks the host for a posting that cannot exist, built from the first posting whose id
// can be rewritten, and reports whether the host answered 404/410 — the proof that its 404 means
// "no such posting" and its 200 means "this posting exists". stopped is set when the budget or
// the run ended before the canary could be asked.
func calibrate(ctx context.Context, p *pacer, client *http.Client, rules robots, postings []Posting) (calibrated bool, stopped Reason) {
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	for _, post := range postings {
		canary, ok := canaryURL(post.URL, post.ExternalID, rng)
		if !ok {
			continue
		}
		cu, err := url.Parse(canary)
		if err != nil || !rules.allowed(cu.RequestURI()) {
			continue
		}
		status, final, _, ok := p.get(ctx, client, canary, false)
		if !ok {
			return false, p.stopReason(ctx)
		}
		outcome, _ := readStatus(cu, status, final)
		return outcome == Dead, ""
	}
	return false, ""
}

// errStopped ends a redirect chain whose next hop the pacer would not grant.
var errStopped = errors.New("qualityaudit: budget spent or run stopped")

// client returns a copy of base that follows a redirect only while it stays on host, on a path
// rules allow, within maxRedirects hops — and each hop waits its turn and draws on the budget like
// any other request to the host. Any other redirect is handed back unfollowed (and reads as
// moved): the robots check and the pacing were done for THIS host, and following a posting off to
// somebody else's site would probe it with neither.
func (p *pacer) client(base *http.Client, host string, rules robots) *http.Client {
	c := *base
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// via holds every request made so far, so the fifth redirect sees five.
		if len(via) > maxRedirects || !strings.EqualFold(req.URL.Host, host) || !rules.allowed(req.URL.RequestURI()) {
			return http.ErrUseLastResponse
		}
		if !p.turn(req.Context()) {
			return errStopped
		}
		return nil
	}
	return &c
}

// pacer spaces one host's requests and draws each from the run's shared budget.
type pacer struct {
	interval time.Duration
	budget   *atomic.Int64
	last     time.Time
}

// get waits its turn, takes one request from the budget and fetches rawURL. ok is false when the
// budget is spent or the run was stopped — not when the fetch failed, which is status 0. withBody
// returns up to maxRobots of the body as text (robots.txt); otherwise up to maxDrain is drained
// and dropped.
func (p *pacer) get(ctx context.Context, client *http.Client, rawURL string, withBody bool) (status int, final *url.URL, body string, ok bool) {
	if !p.turn(ctx) {
		return 0, nil, "", false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, "", true
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if errors.Is(err, errStopped) {
		return 0, nil, "", false
	}
	if err != nil {
		return 0, nil, "", true
	}
	defer resp.Body.Close()
	limit := int64(maxDrain)
	if withBody {
		limit = maxRobots
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil && withBody {
		// A robots.txt cut short is not one we can read; RFC 9309 treats it as unreachable.
		return 0, resp.Request.URL, "", true
	}
	if withBody {
		body = string(b)
	}
	return resp.StatusCode, resp.Request.URL, body, true
}

// turn waits until the host's interval has passed since the last request and takes one request
// from the budget. False means the budget is spent or the run was stopped.
func (p *pacer) turn(ctx context.Context) bool {
	if !p.last.IsZero() {
		if wait := time.Until(p.last.Add(p.interval)); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return false
			case <-t.C:
			}
		}
	}
	if ctx.Err() != nil || p.budget.Add(-1) < 0 {
		return false
	}
	p.last = time.Now()
	return true
}

// stopReason names why get refused to make a request.
func (p *pacer) stopReason(ctx context.Context) Reason {
	if ctx.Err() != nil {
		return ReasonCanceled
	}
	return ReasonBudget
}

// LivenessCounts is one source's tally. Sampled - Live - Dead is the unmeasured share, and a
// source with Live + Dead == 0 was not measurable at all.
type LivenessCounts struct {
	Sampled, Live, Dead int64
}

// TallyLiveness counts results per source.
func TallyLiveness(results []Result) map[string]LivenessCounts {
	out := map[string]LivenessCounts{}
	for _, r := range results {
		c := out[r.Posting.Source]
		c.Sampled++
		switch r.Outcome {
		case Live:
			c.Live++
		case Dead:
			c.Dead++
		}
		out[r.Posting.Source] = c
	}
	return out
}
