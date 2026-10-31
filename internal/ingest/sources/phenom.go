package sources

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
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
// The walk advances by what each page actually held and stops at the total the first page
// stated, or at an empty page. The empty page is also where a board over the listing's
// 10,000-result window ends (past it the API answers empty with a total of 0): such a board is
// read to the window and no further, which is all the API serves through this request.
//
// It is not a proof of the whole board, and the adapter does not claim fullBoardListing: the
// listing's order is not stable between requests, so one walk reads ~85-92% of a board's
// distinct postings (see TestPhenomIsNotFullBoardListing). Successive walks read different
// subsets, which is what keeps the 48h company-scoped sweep safe.
func (s phenom) list(ctx context.Context, board string) ([]phenomPosting, error) {
	url := fmt.Sprintf("https://%s/widgets", board)
	var postings []phenomPosting
	total := -1
	for page, from := 0, 0; ; page++ {
		if page == phenomMaxPages {
			return nil, fmt.Errorf("phenom: list board %s: %d pages without reaching the stated total %d", board, page, total)
		}
		body := map[string]any{
			"deviceType": "desktop",
			"ddoKey":     "refineSearch",
			"jobs":       true,
			"from":       from,
			"size":       phenomPageSize,
		}
		var resp struct {
			RefineSearch struct {
				TotalHits *int `json:"totalHits"`
				Data      struct {
					Jobs []phenomPosting `json:"jobs"`
				} `json:"data"`
			} `json:"refineSearch"`
		}
		if err := s.http.PostJSON(ctx, url, body, &resp); err != nil {
			return nil, fmt.Errorf("phenom: list board %s: %w", board, err)
		}
		if total < 0 {
			if resp.RefineSearch.TotalHits == nil {
				return nil, errListingAbsent("phenom", board, "totalHits")
			}
			total = *resp.RefineSearch.TotalHits
		}
		got := resp.RefineSearch.Data.Jobs
		postings = append(postings, got...)
		from += len(got)
		if len(got) == 0 || from >= total {
			return postings, nil
		}
	}
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
