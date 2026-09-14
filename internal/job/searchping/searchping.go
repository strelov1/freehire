// Package searchping announces job URLs to the external search engines that offer a
// way to be told, instead of waiting to be crawled.
//
// It exists because waiting does not work at this catalogue's size. Measured
// 2026-09-14 on prod: Googlebot fetched 65 distinct job pages in a day against ~690k
// in the sitemap, and URL Inspection answers "URL is unknown to Google" for postings
// sampled from it — pages Google has never once fetched. A posting is closed as soon
// as the employer's listing disappears, so a page first crawled months later is stale
// before it is reachable.
//
// The budget is the design constraint, not the plumbing. Google's Indexing API grants
// 200 publish calls a day by default against ~14k new technical postings a day, so
// this package can never announce everything and must not pretend otherwise: it
// chooses the newest eligible postings each run and records what it sent, rather than
// draining a queue that would only grow. See migrations/0162_job_search_pings.sql.
package searchping

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Engine is one external search engine that accepts a URL announcement.
//
// Announce takes a batch and returns the URLs the engine ACCEPTED, because the two
// engines disagree about what a send is: Google's Indexing API takes one URL per HTTP
// call, so a batch is a loop and a partial failure is normal; IndexNow takes up to
// 10,000 in one POST and answers for all of them at once. Returning the accepted
// subset lets each engine do the right thing while the ledger stays exact — it
// records what an engine took, never what it was offered.
type Engine interface {
	// Name identifies the engine in the ledger and in logs. It is STORED, so it must
	// stay stable across releases: the anti-join that keeps a budget from being spent
	// twice reads it back.
	Name() string

	// DailyBudget bounds how many URLs one day's runs may send, or 0 for unbounded.
	DailyBudget() int

	// Announce sends the URLs and returns those the engine accepted, in any order.
	Announce(ctx context.Context, urls []string) ([]string, error)
}

// Candidate is a posting eligible to be announced.
type Candidate struct {
	JobID int64
	Slug  string
}

// Repository is the ledger: which postings an engine has already been told about, and
// the record that it has.
type Repository interface {
	// JobsToPing returns the newest eligible postings this engine has not been sent,
	// newest first. The eligibility gate lives in SQL beside the query — see
	// internal/platform/db/queries/job_search_pings.sql.
	JobsToPing(ctx context.Context, engine string, limit int32) ([]Candidate, error)

	// RecordPing marks one posting as announced to one engine. Idempotent.
	RecordPing(ctx context.Context, jobID int64, engine string) error

	// PingsSince counts what this engine has been sent since a moment, so a run that
	// shares a budget day with an earlier one does not overspend it.
	PingsSince(ctx context.Context, engine string, since time.Time) (int64, error)
}

// pacific is where Google's Indexing API quota resets — midnight Pacific, not UTC and
// not the host's clock. A budget day measured in UTC would let a run just after 00:00
// UTC spend an allowance that, for eight or nine months of the year, Google still
// considers yesterday's.
var pacific = mustLoadPacific()

func mustLoadPacific() *time.Location {
	// A host without tzdata would otherwise silently fall back to UTC, which is the
	// misreading this variable exists to prevent; the fixed offset is the winter one,
	// the conservative direction (it starts the budget day later than DST would).
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.FixedZone("PST", -8*60*60)
	}
	return loc
}

// budgetDayStart is the instant the current quota day began.
func budgetDayStart(now time.Time) time.Time {
	local := now.In(pacific)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, pacific)
}

// Report is what one run did, per engine.
type Report struct {
	Engine    string
	Offered   int
	Accepted  int
	Recorded  int
	Remaining int // budget left for the rest of the day, -1 when unbounded
	Err       error

	// URLs is filled by Preview only: the exact addresses Run would have sent. A dry
	// run exists to catch a URL that is wrong — a stale origin, a slug that is not the
	// canonical one — and a count cannot be wrong in a way anybody can see.
	URLs []string
}

// Runner announces the newest eligible postings to every configured engine.
type Runner struct {
	repo    Repository
	origin  string
	engines []Engine
}

// New builds a Runner. origin is the public site origin (FRONTEND_ORIGIN), because a
// URL announced to a search engine must be the one it can fetch — this package never
// derives it from a request.
func New(repo Repository, origin string, engines ...Engine) *Runner {
	return &Runner{repo: repo, origin: strings.TrimRight(strings.TrimSpace(origin), "/"), engines: engines}
}

// jobURL is the public address of a posting. Announcing anything else — a redirect, an
// old domain — spends budget teaching an engine a URL it will only have to follow
// away from, which is the exact waste this fleet already measured on Googlebot (64% of
// its visits on 2026-09-14 answered 301 from retired hostnames).
func (r *Runner) jobURL(slug string) string {
	return r.origin + "/jobs/" + slug
}

// Run announces one batch per engine. An engine that fails does not stop another: the
// engines are independent services and a shared run is an implementation detail, not a
// transaction.
func (r *Runner) Run(ctx context.Context, batch int) []Report {
	reports := make([]Report, 0, len(r.engines))
	for _, engine := range r.engines {
		reports = append(reports, r.runOne(ctx, engine, batch))
	}
	return reports
}

// Preview resolves exactly what Run would send, and sends nothing. It reads the same
// budget the real run would, so a dry run against an engine whose day is spent reports
// an empty batch rather than the batch it would have sent yesterday.
func (r *Runner) Preview(ctx context.Context, batch int) []Report {
	reports := make([]Report, 0, len(r.engines))
	for _, engine := range r.engines {
		report := Report{Engine: engine.Name(), Remaining: -1}

		limit, err := r.remainingToday(ctx, engine, batch)
		if err != nil {
			report.Err = err
			reports = append(reports, report)
			continue
		}
		if engine.DailyBudget() > 0 {
			report.Remaining = limit
		}
		if limit > 0 {
			candidates, err := r.repo.JobsToPing(ctx, engine.Name(), int32(limit))
			if err != nil {
				report.Err = fmt.Errorf("list candidates: %w", err)
			}
			for _, c := range candidates {
				report.URLs = append(report.URLs, r.jobURL(c.Slug))
			}
			report.Offered = len(report.URLs)
		}
		reports = append(reports, report)
	}
	return reports
}

func (r *Runner) runOne(ctx context.Context, engine Engine, batch int) Report {
	report := Report{Engine: engine.Name(), Remaining: -1}

	limit, err := r.remainingToday(ctx, engine, batch)
	if err != nil {
		report.Err = err
		return report
	}
	if engine.DailyBudget() > 0 {
		report.Remaining = limit
	}
	if limit <= 0 {
		return report
	}

	candidates, err := r.repo.JobsToPing(ctx, engine.Name(), int32(limit))
	if err != nil {
		report.Err = fmt.Errorf("list candidates: %w", err)
		return report
	}
	if len(candidates) == 0 {
		return report
	}

	bySlug := make(map[string]Candidate, len(candidates))
	urls := make([]string, 0, len(candidates))
	for _, c := range candidates {
		url := r.jobURL(c.Slug)
		bySlug[url] = c
		urls = append(urls, url)
	}
	report.Offered = len(urls)

	accepted, err := engine.Announce(ctx, urls)
	report.Accepted = len(accepted)
	report.Err = err

	// Record whatever was accepted even when the batch also failed. A send that
	// happened and was not written down is the one outcome that costs budget twice.
	var recordErrs []error
	for _, url := range accepted {
		c, ok := bySlug[url]
		if !ok {
			// An engine that answers for a URL we did not offer is a bug in that
			// engine's adapter, not a reason to drop the rest of the batch.
			log.Printf("searchping: %s accepted an unoffered url %q", engine.Name(), url)
			continue
		}
		if err := r.repo.RecordPing(ctx, c.JobID, engine.Name()); err != nil {
			recordErrs = append(recordErrs, fmt.Errorf("record job %d: %w", c.JobID, err))
			continue
		}
		report.Recorded++
	}
	if len(recordErrs) > 0 {
		report.Err = errors.Join(report.Err, errors.Join(recordErrs...))
	}
	if report.Remaining >= 0 {
		report.Remaining -= report.Recorded
	}
	return report
}

// remainingToday is how many URLs this engine may still be sent, bounded by both the
// caller's batch size and what the day's budget has left. Reading the ledger rather
// than trusting a run to be the day's only one: the budget resets at midnight Pacific
// on Google's side and the timer here fires more than once a day, so a run that
// assumed it had the whole allowance would spend it again every time.
func (r *Runner) remainingToday(ctx context.Context, engine Engine, batch int) (int, error) {
	budget := engine.DailyBudget()
	if budget <= 0 {
		return batch, nil
	}
	sent, err := r.repo.PingsSince(ctx, engine.Name(), budgetDayStart(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("count today's pings: %w", err)
	}
	left := budget - int(sent)
	if left < 0 {
		left = 0
	}
	return min(left, batch), nil
}
