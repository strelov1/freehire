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

	// Embeds the IANA zone database. Required, not defensive: the budget day is
	// Google's, measured in Pacific time with its daylight saving rule, and a host
	// without tzdata would otherwise make LoadLocation fail — see pacific below for
	// what each stand-in gets wrong.
	_ "time/tzdata"
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

	// Accepts reports whether this engine may be told about a kind of page at all.
	//
	// It exists because the restriction is real and one-sided: Google's Indexing API
	// admits only JobPosting and BroadcastEvent pages, so a company page sent there is
	// a terms violation whose penalty is the quota. IndexNow has no such rule. Asking
	// the engine rather than branching on its name keeps that fact where it belongs and
	// makes the next engine state its own answer.
	Accepts(kind Kind) bool

	// Announce sends the URLs and returns those the engine accepted, in any order.
	Announce(ctx context.Context, urls []string) ([]string, error)
}

// Candidate is a page eligible to be announced.
type Candidate struct {
	// JobID identifies a posting in the ledger; zero for a company page.
	JobID int64

	// Company identifies a company in the ledger; empty for a posting.
	Company string

	// Slug is the last path segment of the public URL — of the posting or the company.
	Slug string
}

// Kind is which event about a posting is being announced. Both are URL_UPDATED to the
// engine — a closed posting's page stays at HTTP 200 with its JobPosting markup and a
// retired validThrough, which is one of the three ways Google documents for taking a
// posting down, so a closure is a RE-CRAWL and never a deletion. Sending URL_DELETED for
// a page that is still online is a misuse of the API, and its penalty is the quota.
//
// The values are STORED in the ledger, so they must stay stable across releases.
type Kind string

const (
	// KindCreated is "this posting exists", announced once when it is first selected.
	KindCreated Kind = "created"

	// KindClosed is "read this page again", announced once after the posting closes so
	// the engine sees the validThrough that has moved into the past.
	KindClosed Kind = "closed"

	// KindCompany is "this employer's page exists". It has no closure event — a company
	// whose postings all close keeps a 200 page — and unlike the two above it is not
	// stored in a `kind` column: its ledger is company_search_pings, which needs no such
	// column because it holds only one event.
	KindCompany Kind = "company"
)

// isCompany reports whether a pass is about company pages rather than postings. The
// distinction decides three things at once: which ledger records it, which URL prefix it
// takes, and — through Engine.Accepts — whether an engine may be told about it at all.
func (k Kind) isCompany() bool { return k == KindCompany }

// Repository is the ledger: which postings an engine has already been told about, for
// which event, and the record that it has.
type Repository interface {
	// JobsToPing returns the newest eligible postings this engine has not been sent,
	// newest first. The eligibility gate lives in SQL beside the query — see
	// internal/platform/db/queries/job_search_pings.sql.
	JobsToPing(ctx context.Context, engine string, limit int32) ([]Candidate, error)

	// ClosedJobsToPing returns postings that closed after this engine was told they
	// existed, most recently closed first. Bounded by construction: it can never exceed
	// what has already been announced.
	ClosedJobsToPing(ctx context.Context, engine string, limit int32) ([]Candidate, error)

	// CompaniesToPing returns company pages this engine has not been told about, newest
	// first. Eligibility is the same job_count > 0 that puts a company in the sitemap.
	CompaniesToPing(ctx context.Context, engine string, limit int32) ([]Candidate, error)

	// RecordPing marks one page as announced to one engine, for one event. Idempotent.
	RecordPing(ctx context.Context, c Candidate, engine string, kind Kind) error

	// PingsSince counts every page — posting and company alike — this engine has been
	// sent since a moment, so a run that shares a budget day with an earlier one does
	// not overspend it. Both ledgers, because the budget belongs to the engine and a
	// company page costs it exactly what a posting does.
	PingsSince(ctx context.Context, engine string, since time.Time) (int64, error)
}

// pacific is where Google's Indexing API quota resets — midnight Pacific, not UTC and
// not the host's clock. A budget day measured in UTC would let a run just after 00:00
// UTC spend an allowance that, for eight or nine months of the year, Google still
// considers yesterday's.
//
// The zone must be the real one, with its daylight saving rule, and NOT a fixed -8
// offset standing in for it. During DST a fixed PST puts the boundary an hour LATE, so
// the pings sent in that hour are not counted while Google counts them — the run reads
// more allowance left than it has and overspends. (A fixed -7 fails the same way in
// winter, in the opposite hour.) Hence time/tzdata below: it embeds the zone database
// in the binary, so this cannot depend on whether the host happens to carry one.
var pacific = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Unreachable with time/tzdata imported, and a panic rather than a fallback on
		// purpose: every stand-in for this zone is wrong for part of the year, and being
		// wrong here is silent — it looks like budget that was never spent.
		panic("searchping: America/Los_Angeles unavailable despite time/tzdata: " + err.Error())
	}
	return loc
}()

// budgetDayStart is the instant the current quota day began.
func budgetDayStart(now time.Time) time.Time {
	local := now.In(pacific)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, pacific)
}

// Report is what one run did, for one engine and one event.
type Report struct {
	Engine    string
	Kind      Kind
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

// spent is how much of an engine's day this pass consumed.
//
// ACCEPTED, not recorded: the engine consumed a call the moment it took the URL, whether
// or not the ledger write that followed succeeded. Charging only for recorded ones would
// let a failed write hand the next pass budget the engine has already counted against
// the day. Preview accepts nothing and fills URLs instead, and exactly one of the two is
// ever non-zero.
func (r Report) spent() int { return r.Accepted + len(r.URLs) }

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

// pageURL is the public address of a posting or of a company. Announcing anything else —
// a redirect, an old domain — spends budget teaching an engine a URL it will only have to
// follow away from, which is the exact waste this fleet already measured on Googlebot
// (64% of its visits on 2026-09-14 answered 301 from retired hostnames).
func (r *Runner) pageURL(kind Kind, slug string) string {
	if kind.isCompany() {
		return r.origin + "/companies/" + slug
	}
	return r.origin + "/jobs/" + slug
}

// passes is the order the three events compete for one engine's day, and the order is
// the policy.
//
// A new posting first: it is what brings a visitor. Then the company page, which is the
// page that actually WINS — measured 2026-09-15, Bing's highest-impression pages are
// /companies/<slug> and the queries behind them are all "<company name> careers", and
// Google's own August data agrees. It sits second only because a posting is perishable
// and a company page is not: the posting's moment passes, the employer's page keeps.
// Closures last: they tidy an index we do not own.
//
// Google never reaches the second pass at all — Accepts declines it, because its
// Indexing API admits only JobPosting pages. The ordering matters for IndexNow, which
// accepts everything and has no quota, and for whatever engine comes next.
var passes = []Kind{KindCreated, KindCompany, KindClosed}

// Run announces one batch per engine, per event. An engine that fails does not stop
// another: the engines are independent services and a shared run is an implementation
// detail, not a transaction.
func (r *Runner) Run(ctx context.Context, batch int) []Report {
	return r.walk(ctx, batch, r.runPass)
}

// Preview resolves exactly what Run would send, and sends nothing. It reads the same
// budget the real run would, so a dry run against an engine whose day is spent reports
// an empty batch rather than the batch it would have sent yesterday.
func (r *Runner) Preview(ctx context.Context, batch int) []Report {
	return r.walk(ctx, batch, r.previewPass)
}

// walk runs every (engine, event) pair, threading one engine's remaining allowance
// through its passes in order. Shared for Run and Preview so a dry run cannot drift from
// what the real one would choose — the budget arithmetic is the part most worth seeing
// before it spends anything.
func (r *Runner) walk(ctx context.Context, batch int, pass func(context.Context, Engine, Kind, int) Report) []Report {
	reports := make([]Report, 0, len(r.engines)*len(passes))
	for _, engine := range r.engines {
		left, err := r.remainingToday(ctx, engine, batch)
		if err != nil {
			reports = append(reports, Report{Engine: engine.Name(), Remaining: -1, Err: err})
			continue
		}
		for _, kind := range passes {
			// An engine that will not take this kind of page is not offered it, and the
			// pass is not reported either — a line saying "google/company: nothing to
			// announce" would read like an empty catalogue rather than a rule.
			if !engine.Accepts(kind) {
				continue
			}
			// An unbounded engine is not spending anything shared, so each of its passes
			// gets the full batch rather than the leftovers of the one before.
			limit := batch
			if engine.DailyBudget() > 0 {
				limit = left
			}
			report := pass(ctx, engine, kind, limit)
			reports = append(reports, report)
			left -= report.spent()
		}
	}
	return reports
}

func (r *Runner) candidates(ctx context.Context, engine Engine, kind Kind, limit int) ([]Candidate, error) {
	switch kind {
	case KindClosed:
		return r.repo.ClosedJobsToPing(ctx, engine.Name(), int32(limit))
	case KindCompany:
		return r.repo.CompaniesToPing(ctx, engine.Name(), int32(limit))
	default:
		return r.repo.JobsToPing(ctx, engine.Name(), int32(limit))
	}
}

// plan opens a pass: the report it will be reported under, and what it may send. An
// empty candidate slice covers all three ways a pass has nothing to do — the day's
// budget is spent, the query failed, or nothing is eligible — so neither caller needs to
// tell them apart, and the one that did is the report's own Err.
func (r *Runner) plan(ctx context.Context, engine Engine, kind Kind, limit int) (Report, []Candidate) {
	report := Report{Engine: engine.Name(), Kind: kind, Remaining: -1}
	if engine.DailyBudget() > 0 {
		// Clamped, though walk never passes a negative: Remaining is a tri-state where
		// -1 means "unbounded", so a negative leaking in here would make a spent engine
		// report the opposite of the truth.
		report.Remaining = max(limit, 0)
	}
	if limit <= 0 {
		return report, nil
	}

	candidates, err := r.candidates(ctx, engine, kind, limit)
	if err != nil {
		report.Err = fmt.Errorf("list candidates: %w", err)
		return report, nil
	}
	return report, candidates
}

func (r *Runner) previewPass(ctx context.Context, engine Engine, kind Kind, limit int) Report {
	report, candidates := r.plan(ctx, engine, kind, limit)
	for _, c := range candidates {
		report.URLs = append(report.URLs, r.pageURL(kind, c.Slug))
	}
	report.Offered = len(report.URLs)
	return report
}

func (r *Runner) runPass(ctx context.Context, engine Engine, kind Kind, limit int) Report {
	report, candidates := r.plan(ctx, engine, kind, limit)
	if len(candidates) == 0 {
		return report
	}

	bySlug := make(map[string]Candidate, len(candidates))
	urls := make([]string, 0, len(candidates))
	for _, c := range candidates {
		url := r.pageURL(kind, c.Slug)
		bySlug[url] = c
		urls = append(urls, url)
	}
	report.Offered = len(urls)

	accepted, err := engine.Announce(ctx, urls)
	report.Accepted = len(accepted)
	report.Err = err

	// Record whatever was accepted even when the batch also failed. A send that
	// happened and was not written down is the one outcome that costs budget twice.
	//
	// This is AT-LEAST-ONCE and deliberately so: an HTTP call cannot join the
	// transaction that records it, so one of the two orderings has to lose. Sending
	// first and failing to record costs a duplicate announcement — bounded, visible in
	// the run's error, and harmless to the engine. Recording first and failing to send
	// would cost the posting its announcement permanently and silently, because a
	// recorded row is never selected again. The loud, bounded failure is the one to
	// keep.
	var recordErrs []error
	for _, url := range accepted {
		c, ok := bySlug[url]
		if !ok {
			// An engine that answers for a URL we did not offer is a bug in that
			// engine's adapter, not a reason to drop the rest of the batch.
			log.Printf("searchping: %s accepted an unoffered url %q", engine.Name(), url)
			continue
		}
		if err := r.repo.RecordPing(ctx, c, engine.Name(), kind); err != nil {
			recordErrs = append(recordErrs, fmt.Errorf("record %s: %w", url, err))
			continue
		}
		report.Recorded++
	}
	if len(recordErrs) > 0 {
		report.Err = errors.Join(report.Err, errors.Join(recordErrs...))
	}
	if report.Remaining >= 0 {
		report.Remaining -= report.spent()
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
	return min(max(budget-int(sent), 0), batch), nil
}
