// Package searchping announces job URLs to the external search engines that offer a
// way to be told, instead of waiting to be crawled.
//
// Waiting to be crawled does not work at this catalogue's size: Googlebot fetches a tiny
// fraction of the sitemap per day, and a posting closes as soon as the employer's listing
// disappears, so a page first crawled months later is stale before it is reachable.
//
// The budget is the design constraint. Google's Indexing API grants 200 publish calls a
// day by default, so this package can never announce everything: it picks the newest
// eligible postings each run (technical first, then the rest with what that left) and
// records what it sent rather than draining a queue that would only grow. See
// migrations/0162_job_search_pings.sql.
package searchping

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	// Embeds the IANA zone database. Required: the budget day is Google's, in Pacific time
	// with DST, and a host without tzdata would make LoadLocation fail (see pacific).
	_ "time/tzdata"
)

// Engine is one external search engine that accepts a URL announcement.
//
// Announce takes a batch and returns the URLs the engine ACCEPTED: Google's Indexing API
// takes one URL per call, so partial failure is normal; IndexNow takes up to 10,000 in
// one POST. The ledger records what an engine took, never what it was offered.
type Engine interface {
	// Name identifies the engine in the ledger and in logs. It is STORED, so it must
	// stay stable across releases: the anti-join that keeps a budget from being spent
	// twice reads it back.
	Name() string

	// DailyBudget bounds how many URLs one day's runs may send, or 0 for unbounded.
	DailyBudget() int

	// Accepts reports whether this engine may be told about a kind of page at all.
	//
	// Google's Indexing API admits only JobPosting and BroadcastEvent pages, so a company
	// page sent there is a terms violation whose penalty is the quota. IndexNow has no such
	// rule. Asking the engine beats branching on its name.
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
// engine: a closed posting's page stays at HTTP 200 with a retired validThrough, so a
// closure is a re-crawl, never a deletion. URL_DELETED for a live page is a misuse whose
// penalty is the quota.
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

// Tier is which half of the catalogue a new-posting pass draws from. It is not stored; it
// orders the passes and names them in the report.
//
// The split is a PRIORITY, not an eligibility rule: every indexed posting is eligible, but
// the technical tier gets Google's scarce budget first and the rest gets what that left.
type Tier string

const (
	// TierTech is the postings is_tech confidently marks technical.
	TierTech Tier = "tech"

	// TierBeyondTech is every other posting with a resolved category.
	TierBeyondTech Tier = "beyond-tech"
)

// isCompany reports whether a pass is about company pages rather than postings. The
// distinction decides three things at once: which ledger records it, which URL prefix it
// takes, and — through Engine.Accepts — whether an engine may be told about it at all.
func (k Kind) isCompany() bool { return k == KindCompany }

// Repository is the ledger: which postings an engine has already been told about, for
// which event, and the record that it has.
type Repository interface {
	// JobsToPing returns the newest eligible postings of one tier this engine has not
	// been sent, newest first. The eligibility gate and the tier predicate live in SQL
	// beside the query — see internal/platform/db/queries/job_search_pings.sql.
	JobsToPing(ctx context.Context, engine string, tier Tier, limit int32) ([]Candidate, error)

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

// pacific is where Google's Indexing API quota resets: midnight Pacific, not UTC or the
// host's clock.
//
// The zone must be the real one with DST, NOT a fixed offset: a fixed -8 puts the boundary
// an hour late during DST (pings in that hour go uncounted and the run overspends), and a
// fixed -7 fails the same way in winter. time/tzdata embeds the zone database so this does
// not depend on the host.
var pacific = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Unreachable with time/tzdata imported. Panic rather than fall back: every stand-in
		// is wrong for part of the year, silently, as budget that was never spent.
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
	Engine string
	Kind   Kind
	// Tier names which half of the catalogue a KindCreated pass drew from; empty for
	// the other kinds, which are not split.
	Tier      Tier
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
// ACCEPTED, not recorded: the engine consumed a call when it took the URL, so charging only
// recorded ones would let a failed ledger write hand the next pass already-counted budget.
// Preview accepts nothing and fills URLs instead; exactly one of the two is non-zero.
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

// pageURL is the public address of a posting or company. Announcing a redirect or an old
// domain wastes budget teaching an engine a URL it must follow away from.
func (r *Runner) pageURL(kind Kind, slug string) string {
	if kind.isCompany() {
		return r.origin + "/companies/" + slug
	}
	return r.origin + "/jobs/" + slug
}

// pass is one (event, tier) pair competing for an engine's day.
type pass struct {
	kind Kind
	tier Tier // KindCreated only
}

// passes is the order the events compete for one engine's day; the order is the policy.
//
// A new posting first (it brings a visitor), technical before beyond-IT so Google's 200
// calls go to the core and the rest gets what that left. Then the company page, the page
// that actually wins impressions ("<company name> careers"); it sits after postings
// because a posting is perishable and a company page is not. Closures last: they tidy an
// index we do not own.
//
// Google never reaches the company pass: Accepts declines it. The ordering matters for
// IndexNow, which accepts everything and has no quota.
//
// An unbounded engine is not rationed by this order at all: walk gives each of its
// passes a full batch, so IndexNow hears about both tiers every run.
var passes = []pass{
	{kind: KindCreated, tier: TierTech},
	{kind: KindCreated, tier: TierBeyondTech},
	{kind: KindCompany},
	{kind: KindClosed},
}

// Run announces one batch per engine, per event. An engine that fails does not stop
// another.
func (r *Runner) Run(ctx context.Context, batch int) []Report {
	return r.walk(ctx, batch, r.runPass)
}

// Preview resolves what Run would send, and sends nothing. It reads the same budget, so a
// dry run against a spent engine reports an empty batch.
func (r *Runner) Preview(ctx context.Context, batch int) []Report {
	return r.walk(ctx, batch, r.previewPass)
}

// walk runs every (engine, event) pair, threading one engine's remaining allowance through
// its passes in order. Shared by Run and Preview so a dry run cannot drift from the real one.
func (r *Runner) walk(ctx context.Context, batch int, run func(context.Context, Engine, pass, int) Report) []Report {
	reports := make([]Report, 0, len(r.engines)*len(passes))
	for _, engine := range r.engines {
		left, err := r.remainingToday(ctx, engine, batch)
		if err != nil {
			reports = append(reports, Report{Engine: engine.Name(), Remaining: -1, Err: err})
			continue
		}
		for _, p := range passes {
			// An engine that will not take this kind of page is not offered it, and the pass is
			// not reported: "google/company: nothing to announce" would read like an empty
			// catalogue rather than a rule.
			if !engine.Accepts(p.kind) {
				continue
			}
			// An unbounded engine shares nothing, so each pass gets the full batch.
			limit := batch
			if engine.DailyBudget() > 0 {
				limit = left
			}
			report := run(ctx, engine, p, limit)
			reports = append(reports, report)
			left -= report.spent()
			// Passes are in priority order, so a budgeted engine whose candidate lookup failed
			// stops there: the empty answer would hand the day's allowance to the lower tiers.
			// A pass that failed after offering something was already charged, so it does not
			// stop the rest. An unbounded engine carries on.
			if report.Err != nil && report.Offered == 0 && engine.DailyBudget() > 0 {
				break
			}
		}
	}
	return reports
}

func (r *Runner) candidates(ctx context.Context, engine Engine, p pass, limit int) ([]Candidate, error) {
	switch p.kind {
	case KindClosed:
		return r.repo.ClosedJobsToPing(ctx, engine.Name(), int32(limit))
	case KindCompany:
		return r.repo.CompaniesToPing(ctx, engine.Name(), int32(limit))
	default:
		return r.repo.JobsToPing(ctx, engine.Name(), p.tier, int32(limit))
	}
}

// plan opens a pass: its report and what it may send. An empty candidate slice covers every
// way a pass has nothing to do (budget spent, query failed, nothing eligible); the report's
// own Err tells them apart.
func (r *Runner) plan(ctx context.Context, engine Engine, p pass, limit int) (Report, []Candidate) {
	report := Report{Engine: engine.Name(), Kind: p.kind, Tier: p.tier, Remaining: -1}
	if engine.DailyBudget() > 0 {
		// Clamped: Remaining is a tri-state where -1 means unbounded, so a negative leaking in
		// would make a spent engine read as unbounded.
		report.Remaining = max(limit, 0)
	}
	if limit <= 0 {
		return report, nil
	}

	candidates, err := r.candidates(ctx, engine, p, limit)
	if err != nil {
		report.Err = fmt.Errorf("list candidates: %w", err)
		return report, nil
	}
	return report, candidates
}

func (r *Runner) previewPass(ctx context.Context, engine Engine, p pass, limit int) Report {
	report, candidates := r.plan(ctx, engine, p, limit)
	for _, c := range candidates {
		report.URLs = append(report.URLs, r.pageURL(p.kind, c.Slug))
	}
	report.Offered = len(report.URLs)
	return report
}

func (r *Runner) runPass(ctx context.Context, engine Engine, p pass, limit int) Report {
	report, candidates := r.plan(ctx, engine, p, limit)
	if len(candidates) == 0 {
		return report
	}

	bySlug := make(map[string]Candidate, len(candidates))
	urls := make([]string, 0, len(candidates))
	for _, c := range candidates {
		url := r.pageURL(p.kind, c.Slug)
		bySlug[url] = c
		urls = append(urls, url)
	}
	report.Offered = len(urls)

	accepted, err := engine.Announce(ctx, urls)
	report.Accepted = len(accepted)
	report.Err = err

	// Record whatever was accepted even when the batch also failed: a send that was not
	// written down is the one outcome that costs budget twice.
	//
	// AT-LEAST-ONCE on purpose: an HTTP call cannot join the transaction that records it.
	// Sending first costs a bounded, visible duplicate; recording first would lose the
	// announcement permanently and silently, since a recorded row is never selected again.
	var recordErrs []error
	for _, url := range accepted {
		c, ok := bySlug[url]
		if !ok {
			// An engine that answers for a URL we did not offer is a bug in that
			// engine's adapter, not a reason to drop the rest of the batch.
			log.Printf("searchping: %s accepted an unoffered url %q", engine.Name(), url)
			continue
		}
		if err := r.repo.RecordPing(ctx, c, engine.Name(), p.kind); err != nil {
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
// caller's batch size and what the day's budget has left. It reads the ledger because the
// timer fires more than once a day and the budget resets at midnight Pacific.
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
