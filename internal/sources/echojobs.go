package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/skilltag"
)

// echojobs adapts echojobs.io, a large multi-employer tech-jobs aggregator whose postings resolve
// to the employer's own ATS URL (Workday, Greenhouse, and others show up in the feed) rather than
// to an echojobs.io-hosted page. Boardless: one global feed, sorted newest-first by posted_at, no
// per-tenant board.
//
// At last count the feed's reported total sat around 320k postings. Walking it exhaustively every
// cron run would mean thousands of requests per cycle, so Fetch instead bounds the walk to a
// rolling freshness window (echojobsFreshnessWindow) rather than an arbitrary page count: the feed
// is sorted, so paging stops as soon as it runs past that window. This is the same "cannot be
// crawled exhaustively" shape as whatjobs.go, with one structural difference worth naming: because
// the ordering key is posted_at itself, a posting that ages out of the window can never drift back
// into it on a later run the way a whatjobs posting can re-enter its keyword's visible page depth —
// once it exits, this adapter stops seeing it for good. sweepGrace still matters for exactly the
// reason whatjobs' does (a posting the crawl stops reaching is not evidence it closed), it just
// delays an otherwise-inevitable close rather than bridging a gap the next run repairs.
type echojobs struct {
	http JSONGetter
}

const (
	echojobsBaseURL = "https://echojobs.io/api/jobs"
	// echojobsPageSize is the per-page count requested. The feed defaults to 20 but accepts at
	// least 100, and a larger page means fewer requests to cover the same freshness window.
	echojobsPageSize = 100
	// echojobsFreshnessWindow bounds how far back the walk reads, since the feed cannot be
	// crawled to its end every run (see the type doc). 14 days matches whatjobs' bound for a
	// source with the same "cannot verify liveness, cannot crawl exhaustively" shape.
	echojobsFreshnessWindow = 14 * 24 * time.Hour
	// echojobsSweepGrace widens the post-run unseen sweep to match echojobsFreshnessWindow: a
	// posting that ages past the window stops being reported by this adapter regardless of
	// whether it is still open, so the sweep must not read "the crawl stopped reaching it" as
	// "it closed" on the default window. See the sweepGrace type doc in source.go.
	echojobsSweepGrace = echojobsFreshnessWindow
)

// NewEchoJobs builds the echojobs adapter over the given HTTP client.
func NewEchoJobs(c JSONGetter) Source { return echojobs{http: c} }

func (echojobs) Provider() string { return "echojobs" }

// echojobs is a marketplace with one global feed, so its config entries carry no board.
func (echojobs) boardless() {}

// echojobs aggregates postings from many companies, so it stays in the source facet.
func (echojobs) aggregator() {}

func (echojobs) sweepGrace() time.Duration { return echojobsSweepGrace }

// echojobsPosting is one posting from the /api/jobs feed. The upstream response carries several
// more fields (domain_name, first_seen_at, countries, states, job_function, role, seniority,
// salary_min_usd/salary_max_usd, employment_type, employee_count, funding) that this adapter
// deliberately does not read: none of them has a home in the Job shape today, and guessing one
// (e.g. folding salary into Description) is not this adapter's call to make.
type echojobsPosting struct {
	Title          string   `json:"title"`
	CompanyName    string   `json:"company_name"`
	URL            string   `json:"url"`
	JobHandle      string   `json:"job_handle"`
	PostedAt       int64    `json:"posted_at"`
	Locations      []string `json:"locations"`
	RemoteType     string   `json:"remote_type"`
	RequiredSkills []string `json:"required_skills"`
}

// Fetch walks the feed newest-first, stopping once a page's oldest posting (its last entry) falls
// outside echojobsFreshnessWindow. Per the house pagination rule, a failure on the first page is a
// board-level error; a failure on a later page ends the walk with what was already gathered.
func (e echojobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	cutoff := time.Now().Add(-echojobsFreshnessWindow)
	var jobs []Job
	for page := 1; ; page++ {
		var resp struct {
			Jobs []echojobsPosting `json:"jobs"`
		}
		if err := e.http.GetJSON(ctx, echojobsPageURL(page), &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("echojobs: page %d: %w", page, err)
			}
			log.Printf("echojobs: page %d failed, stopping with %d jobs gathered: %v", page, len(jobs), err)
			return jobs, nil
		}
		if len(resp.Jobs) == 0 {
			return jobs, nil
		}
		stale := false
		for _, p := range resp.Jobs {
			postedAt := parseEpochMillis(p.PostedAt)
			if postedAt != nil && postedAt.Before(cutoff) {
				stale = true
				continue
			}
			jobs = append(jobs, p.toJob(postedAt))
		}
		if stale {
			return jobs, nil
		}
	}
}

func echojobsPageURL(page int) string {
	return fmt.Sprintf("%s?page=%d&per_page=%d", echojobsBaseURL, page, echojobsPageSize)
}

func (p echojobsPosting) toJob(postedAt *time.Time) Job {
	mode, remote := echojobsWorkMode(p.RemoteType)
	return Job{
		ExternalID: p.JobHandle,
		URL:        p.URL,
		Title:      p.Title,
		Company:    p.CompanyName,
		Location:   strings.Join(p.Locations, "; "),
		Remote:     remote,
		WorkMode:   mode,
		PostedAt:   postedAt,
		Skills:     skilltag.Canonicalize(p.RequiredSkills),
	}
}

// echojobsWorkMode passes through the feed's own remote_type: it already speaks freehire's
// vocabulary (remote/hybrid/onsite), so there is no bucketing to do — just a check that the value
// is one we recognize, leaving WorkMode empty for anything else rather than passing through junk.
func echojobsWorkMode(remoteType string) (mode string, remote bool) {
	switch remoteType {
	case "remote", "hybrid", "onsite":
		return remoteType, remoteType == "remote"
	default:
		return "", false
	}
}
