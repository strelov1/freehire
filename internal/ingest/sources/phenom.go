package sources

import (
	"context"
	"fmt"
	"log"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync/atomic"
)

// phenom adapts Phenom People career sites (e.g. careers.dhl.com). The board is the
// site host; both the listing and the per-job detail are POSTs to
// "https://<board>/widgets" distinguished by a "ddoKey" — refineSearch for the paged
// job list, jobDetail for one posting's full HTML description. The list carries only a
// teaser, so a posting needs a detail fetch the first time it is seen (see FetchNew).

// phenomPageSize is the refineSearch page size: the largest the API serves (asked for 1,000
// it answers 500, measured 2026-10-30 on careers.dhl.com). Every board of a run shares one
// pace (pacedPhenomGetter), so the listing's request count is the run's floor — ~1,250 pages
// for the ~514k postings 381 boards stated that day, five times fewer than at 100.
const phenomPageSize = 500

// phenomMaxPages is a ceiling on one board's walk. The listing serves nothing once from+size
// reaches 10,000, so no honest board needs more than 20 pages; reaching this means the API
// kept serving full pages past every total it stated, and the crawl fails rather than looping.
const phenomMaxPages = 100

// phenomMaxNewPerBoard bounds how many unseen postings one board hydrates per run, and
// phenomMaxNewPerRun how many the whole run does. Both exist because every board shares one
// pace: the run's listing pages must all fit inside its crawl deadline, and a detail request
// spent on a cold board is a listing page another board does not get. On 2026-10-30, 381
// boards stated ~514k postings against 122k stored, so the first crawls face a ~390k backlog;
// unbounded, the leading boards' bodies would again spend the whole deadline (see FetchNew).
// At these bounds the backlog drains over a few weeks of 3-hourly runs, steady-state churn
// (~5k new postings a day) fits easily, and the listing still reaches every board each run.
const (
	phenomMaxNewPerBoard = 300
	phenomMaxNewPerRun   = 3000
)

// phenomDateLayout is Phenom's postedDate format: RFC3339 with milliseconds and a
// numeric, colon-less zone offset ("2026-05-17T22:00:00.000+0000").
const phenomDateLayout = "2006-01-02T15:04:05.000-0700"

type phenom struct {
	http JSONPoster
	// runBudget is the run's remaining hydration allowance, shared by every board; the
	// registry builds the adapter once per run.
	runBudget *atomic.Int64
}

// NewPhenom builds the Phenom adapter over the given HTTP client.
func NewPhenom(c JSONPoster) Source {
	b := new(atomic.Int64)
	b.Store(phenomMaxNewPerRun)
	return phenom{http: c, runBudget: b}
}

func (phenom) Provider() string { return "phenom" }

// phenomPosting is one job from the refineSearch list (no full description here).
type phenomPosting struct {
	JobSeqNo   string `json:"jobSeqNo"`
	Title      string `json:"title"`
	CityState  string `json:"cityState"`
	Locale     string `json:"locale"`
	PostedDate string `json:"postedDate"`
}

// Fetch hydrates every listed posting, unbounded — the list-only fallback for a caller with
// no seen set (link resolution, tests); ingest goes through FetchNew.
func (s phenom) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.crawl(ctx, e, func(string) bool { return false }, math.MaxInt, nil)
}

// FetchNew lists the whole board but spends a detail request only on a posting the catalogue
// does not hold, within phenomMaxNewPerBoard and the run's phenomMaxNewPerRun. A seen posting
// comes back as a SeenRefresh carrying the listing's identity, so its liveness refreshes and
// its stored body is kept; an unseen posting past either bound is left for a later run.
//
// The detail was this adapter's whole cost. Until 2026-10-30 every run re-read every stored
// body, under a ~1 req/s pace shared by all boards; once the board count went from ~95 to 383
// (2026-10-21..30) a run reached ~18 boards before its deadline, the rest failed in its last
// seconds, and 66% of the provider's ~122k open postings went unseen for over a week.
func (s phenom) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.crawl(ctx, e, seen, phenomMaxNewPerBoard, s.runBudget)
}

// crawl is the walk behind Fetch and FetchNew: list the board, refresh the seen postings from
// the listing alone, and hydrate up to maxNew of the rest while the run budget lasts (nil:
// unbounded). The unseen postings left out are not yielded, so the pipeline reads the board as
// having pending work rather than as quiet.
//
// The board's share goes to a random sample rather than the listing's first postings: a
// posting whose detail never loads stays unseen, and a fixed prefix of those would take the
// share on every run and starve the postings behind it.
//
// Not yielding is safe because an unseen posting is almost always a new one. The exceptions
// are the stored rows the seen set withholds (INGEST_REFETCH_ALL, the hydration-retry window,
// BODY_REFRESH_DAYS): one left out of the sample gets no liveness refresh this run, and is
// refreshed by the next run that samples it or no longer withholds it.
func (s phenom) crawl(ctx context.Context, e CompanyEntry, seen func(string) bool, maxNew int, budget *atomic.Int64) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	var (
		jobs   []Job
		unseen []phenomPosting
		listed = make(map[string]bool, len(postings))
	)
	for _, p := range postings {
		if p.JobSeqNo == "" || listed[p.JobSeqNo] {
			continue
		}
		listed[p.JobSeqNo] = true
		if !seen(p.JobSeqNo) {
			unseen = append(unseen, p)
			continue
		}
		jobs = append(jobs, Job{
			ExternalID:  p.JobSeqNo,
			URL:         phenomJobURL(e.Board, p.Locale, p.JobSeqNo),
			Title:       strings.TrimSpace(p.Title),
			Company:     e.Company,
			SeenRefresh: true,
		})
	}
	want := min(len(unseen), maxNew)
	if budget != nil {
		want = int(claimBudget(budget, int64(want)))
	}
	if want < len(unseen) {
		log.Printf("phenom: board %q: hydrating %d of %d new postings this run, the rest deferred", e.Board, want, len(unseen))
		rand.Shuffle(len(unseen), func(i, j int) { unseen[i], unseen[j] = unseen[j], unseen[i] })
		unseen = unseen[:want]
	}
	hydrated := fetchDetails(unseen, defaultDetailWorkers, func(p phenomPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	})
	return append(jobs, hydrated...), nil
}

// claimBudget takes up to want from the shared budget and returns how much it got.
func claimBudget(budget *atomic.Int64, want int64) int64 {
	for {
		left := budget.Load()
		got := min(left, want)
		if got <= 0 {
			return 0
		}
		if budget.CompareAndSwap(left, left-got) {
			return got
		}
	}
}

// list pages the refineSearch widget to the board's stated total. lang/country are
// deliberately omitted from the body: they do not change the result set (verified), so the
// board host is the only per-tenant input.
//
// The listing serves nothing once from+size reaches phenomListingWindow (past it the API
// answers empty with a total of 0), so a board stating more is read one state at a time
// instead (see listByState); the first page asks for the per-state counts that needs, so a
// board under the window costs no extra request.
//
// It is not a proof of the whole board, and the adapter does not claim fullBoardListing: the
// listing's order is not stable between requests, so one walk reads ~85-92% of a board's
// distinct postings (see TestPhenomIsNotFullBoardListing). Successive walks read different
// subsets, which is what keeps the 48h company-scoped sweep safe.
func (s phenom) list(ctx context.Context, board string) ([]phenomPosting, error) {
	first, err := s.page(ctx, board, nil, 0, true)
	if err != nil {
		return nil, err
	}
	if first.total <= phenomListingWindow {
		return s.walk(ctx, board, nil, &first)
	}
	return s.listByState(ctx, board, first)
}

// phenomListingWindow is how far into one listing the API serves: a page reaching past the
// 10,000th result answers empty (measured 2026-10-30). Six of 382 boards stated more on
// 2026-10-31 — CVS and O'Reilly ~19.1k, Advance Auto Parts 16.5k, Dollar Tree 15.4k, two of
// them under two hostnames each.
const phenomListingWindow = 10000

// listByState reads a board over the listing window as one walk per state (the
// selected_fields facet filter), every one under the window. The partition is checked against
// the counts the board's first page stated before anything is read: every state must fit the
// window, and together they must account for the board's total, or the crawl fails rather than
// pass a partial read off as the board. The sum may exceed the total — a multi-location posting
// is counted, and listed, under each of its states (CVS: 23,224 for 19,099) — and crawl keeps
// one copy of each posting.
//
// State, not category or country, because it is the only facet that slices every board over
// the window: Advance Auto Parts and Dollar Tree each hold ~99% of their postings under one
// category, and every board's country is ~all US. It also reads more of a board: a slice
// that fits one page is read whole, where offset paging loses postings between pages (CVS
// 2026-10-31: 94.6% distinct by state, 86.4% by category).
func (s phenom) listByState(ctx context.Context, board string, first phenomPage) ([]phenomPosting, error) {
	counted := 0
	for state, n := range first.states {
		if n > phenomListingWindow {
			return nil, fmt.Errorf("phenom: list board %s: %d postings, and state %q alone holds %d, over the %d the listing serves", board, first.total, state, n, phenomListingWindow)
		}
		counted += n
	}
	if counted < first.total {
		return nil, fmt.Errorf("phenom: list board %s: %d postings over the %d the listing serves, and its states account for only %d", board, first.total, phenomListingWindow, counted)
	}
	var postings []phenomPosting
	for _, state := range slices.Sorted(maps.Keys(first.states)) {
		filter := map[string][]string{"state": {state}}
		got, err := s.walk(ctx, board, filter, nil)
		if err != nil {
			return nil, err
		}
		postings = append(postings, got...)
	}
	log.Printf("phenom: board %q states %d postings, over the listing window: read in %d state slices", board, first.total, len(first.states))
	return postings, nil
}

// phenomPage is one refineSearch answer: the total the listing states, the page's postings,
// and — when asked for — the per-state counts.
type phenomPage struct {
	total  int
	jobs   []phenomPosting
	states map[string]int
}

// walk pages one listing — the whole board, or the slice filter selects — to the total its
// first page states, advancing by what each page actually held and stopping there or at an
// empty page. first is that page when the caller already holds it, nil when it does not.
func (s phenom) walk(ctx context.Context, board string, filter map[string][]string, first *phenomPage) ([]phenomPosting, error) {
	if first == nil {
		p, err := s.page(ctx, board, filter, 0, false)
		if err != nil {
			return nil, err
		}
		first = &p
	}
	if first.total < 0 {
		return nil, errListingAbsent("phenom", board, "totalHits")
	}
	postings := first.jobs
	from := len(first.jobs)
	for pages := 1; len(postings) > 0 && from < first.total; pages++ {
		if pages == phenomMaxPages {
			return nil, fmt.Errorf("phenom: list board %s: %d pages without reaching the stated total %d", board, pages, first.total)
		}
		next, err := s.page(ctx, board, filter, from, false)
		if err != nil {
			return nil, err
		}
		if len(next.jobs) == 0 {
			break
		}
		postings = append(postings, next.jobs...)
		from += len(next.jobs)
	}
	return postings, nil
}

// page reads one refineSearch page at from, narrowed to filter when it is set; withStates asks
// for the board's per-state counts beside it.
func (s phenom) page(ctx context.Context, board string, filter map[string][]string, from int, withStates bool) (phenomPage, error) {
	body := map[string]any{
		"deviceType": "desktop",
		"ddoKey":     "refineSearch",
		"jobs":       true,
		"from":       from,
		"size":       phenomPageSize,
	}
	if filter != nil {
		body["selected_fields"] = filter
	}
	if withStates {
		body["counts"] = true
		body["all_fields"] = []string{"state"}
	}
	var resp struct {
		RefineSearch struct {
			TotalHits *int `json:"totalHits"`
			Data      struct {
				Jobs         []phenomPosting `json:"jobs"`
				Aggregations []struct {
					Field string         `json:"field"`
					Value map[string]int `json:"value"`
				} `json:"aggregations"`
			} `json:"data"`
		} `json:"refineSearch"`
	}
	if err := s.http.PostJSON(ctx, fmt.Sprintf("https://%s/widgets", board), body, &resp); err != nil {
		return phenomPage{}, fmt.Errorf("phenom: list board %s: %w", board, err)
	}
	p := phenomPage{total: -1, jobs: resp.RefineSearch.Data.Jobs}
	if resp.RefineSearch.TotalHits != nil {
		p.total = *resp.RefineSearch.TotalHits
	}
	for _, a := range resp.RefineSearch.Data.Aggregations {
		if a.Field == "state" {
			p.states = a.Value
		}
	}
	return p, nil
}

// detail fetches one posting's jobDetail and maps it to a Job, returning ok=false when
// the request fails or carries no description so the caller skips just that posting.
func (s phenom) detail(ctx context.Context, e CompanyEntry, p phenomPosting) (Job, bool) {
	url := fmt.Sprintf("https://%s/widgets", e.Board)
	body := map[string]any{
		"deviceType": "desktop",
		"ddoKey":     "jobDetail",
		"pageName":   "job-details",
		"jobSeqNo":   p.JobSeqNo,
	}
	var resp struct {
		JobDetail struct {
			Data struct {
				Job struct {
					Description string `json:"description"`
					Title       string `json:"title"`
				} `json:"job"`
			} `json:"data"`
		} `json:"jobDetail"`
	}
	if err := s.http.PostJSON(ctx, url, body, &resp); err != nil {
		return Job{}, false
	}
	job := resp.JobDetail.Data.Job
	if job.Description == "" {
		return Job{}, false
	}

	return Job{
		ExternalID:  p.JobSeqNo,
		URL:         phenomJobURL(e.Board, p.Locale, p.JobSeqNo),
		Title:       firstNonEmpty(p.Title, job.Title),
		Company:     e.Company,
		Location:    p.CityState,
		Description: sanitizeHTML(job.Description),
		Remote:      isRemote(p.CityState),
		PostedAt:    parseLayout(phenomDateLayout, p.PostedDate),
	}, true
}

// phenomJobURL builds a posting's public page from its locale and sequence id. Phenom
// paths are "/<country>/<lang>/job/<jobSeqNo>" and the list locale is "<lang>_<COUNTRY>"
// (e.g. "en_GLOBAL" -> "/global/en/job/..."). A locale without an underscore falls back
// to the unlocalized "/job/<jobSeqNo>".
func phenomJobURL(board, locale, seq string) string {
	if lang, country, ok := strings.Cut(locale, "_"); ok && lang != "" && country != "" {
		return fmt.Sprintf("https://%s/%s/%s/job/%s", board, strings.ToLower(country), strings.ToLower(lang), seq)
	}
	return fmt.Sprintf("https://%s/job/%s", board, seq)
}
