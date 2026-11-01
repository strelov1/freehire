// Command ghost-crosscheck maintains the `ats_absent` criterion of the ghost job
// signal: for every open posting that reached us through an AGGREGATOR, it asks
// whether the same role appears on that company's OWN crawled board, and stamps
// jobs.ats_absent_at when it does not.
//
// It is a run-once-and-exit worker (cron-scheduled beside ingest/liveness): page
// through open aggregator postings by keyset, group them by company, ask each
// company's board for its open titles once, decide, write, exit.
//
// DRY RUN BY DEFAULT — `--apply` is required to write anything. A dry run prints the calibration report the rollout gate reads:
// how many postings would be stamped, broken down by source and by company. That
// gate exists because this signal's known failure mode is staffing and consulting
// agencies, which advertise clients' roles that are legitimately absent from their
// own board — the exact population a previous attempt at this feature wrongly
// flagged. If the report is dominated by them, the answer is a dictionary
// exclusion, not a lowered threshold.
//
// The COVERAGE GATE is the correctness core: a company with no board of its own in
// our catalogue is skipped entirely. Absence is evidence only where we looked;
// without the gate the signal reports our own crawl coverage as the employer's
// fault.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/ghost"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const (
	// pageSize bounds one keyset page of candidate postings.
	pageSize = 2000
	// lockKey serializes runs. Two runs stamping the same catalogue concurrently
	// would not corrupt anything, but they would double the read load for no gain
	// and interleave their reports into nonsense. A run that cannot take the lock
	// exits cleanly. Arbitrary constant unique to this worker.
	lockKey = 0x66686763 // "fhgc" — freehire ghost crosscheck; the key list lives in internal/platform/migrate
	// sampleSize is how many stamped titles a dry run prints. A report is read by a
	// person deciding whether to open the gate, and they need examples, not a count.
	sampleSize = 25
)

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false,
		"write the stamps; without it the run only reports what it would do")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	queries := db.New(pool)

	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("acquire lock connection: %v", err)
		return 1
	}
	defer lockConn.Release()
	var locked bool
	if err := lockConn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(lockKey)).Scan(&locked); err != nil {
		log.Printf("crosscheck lock: %v", err)
		return 1
	}
	if !locked {
		log.Print("ghost-crosscheck: another run holds the lock — exiting")
		return 0
	}
	defer func() { _, _ = lockConn.Exec(ctx, "SELECT pg_advisory_unlock($1)", int64(lockKey)) }()

	// The provider taxonomy lives in Go adapter markers (sources.ProviderKind), so
	// the two source lists are derived here rather than restated in SQL.
	registry := sources.Taxonomy()
	aggregators := sources.AggregatorProviders(registry)
	boards := boardProviders(registry)
	if len(aggregators) == 0 || len(boards) == 0 {
		// `= ANY('{}')` matches nothing, so an empty list would silently produce a
		// run that judged the whole catalogue absent, or nothing at all. Neither is
		// a result worth writing.
		log.Printf("ghost-crosscheck: empty provider list (aggregators=%d boards=%d) — refusing to run",
			len(aggregators), len(boards))
		return 1
	}

	rep := &report{apply: *apply}
	if err := crosscheckAll(ctx, queries, aggregators, boards, rep); err != nil {
		log.Printf("crosscheck: %v", err)
		return 1
	}
	rep.print()
	return 0
}

// boardProviders is every provider that is a company's OWN hiring surface — a
// multi-tenant ATS or a single-company careers page. These are what an aggregator
// posting is checked against.
func boardProviders(reg map[string]sources.Source) []string {
	var out []string
	for name := range reg {
		switch sources.ProviderKind(reg, name) {
		case sources.KindATS, sources.KindCompany:
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// croscheckAll walks each aggregator source separately, paging by keyset within it, and
// batches each company's postings so its board is read once rather than once per posting.
//
// Per SOURCE rather than across all of them: a single `source = ANY(...) ORDER BY id LIMIT n`
// defeats its own keyset — the planner answers it with a bitmap scan over every aggregator's
// postings plus a sort, so every page re-scans the whole set (measured: 28s per page on
// prod). One source at a time walks jobs_source_id_open_idx in id order and stops at LIMIT.
//
// Progress is logged per source. A run over a large catalogue takes long enough that a
// worker which printed nothing until the end could not be told from one that had hung.
func crosscheckAll(ctx context.Context, q *db.Queries, aggregators, boards []string, rep *report) error {
	for i, source := range aggregators {
		seen, err := crosscheckSource(ctx, q, source, boards, rep)
		if err != nil {
			return fmt.Errorf("source %s: %w", source, err)
		}
		log.Printf("ghost-crosscheck: [%d/%d] %s — %d postings considered (running totals: %d stamped, %d cleared, %d skipped)",
			i+1, len(aggregators), source, seen, rep.stamped, rep.cleared, rep.skips.Total())
	}
	return nil
}

// crosscheckSource reads one source's open postings in full, cross-checks them company
// by company, and returns how many it read.
//
// The whole source is held before anything is judged, because Crosscheck judges a
// company's batch AS A WHOLE — its gates ask how much of the batch the board carries
// — and postings arrive by id, so one company's are scattered across pages. Judging
// per page would hand it fragments and let a gate pass or fail on whichever fragment
// a page boundary cut. The cost is memory: the largest source (job104, ~426k open
// postings on 2026-11-01) holds a few tens of MB of ids and titles.
func crosscheckSource(ctx context.Context, q *db.Queries, source string, boards []string, rep *report) (int, error) {
	var afterID int64
	var seen int
	pending := map[string][]ghost.Posting{}

	for {
		rows, err := q.ListAggregatorJobsForCrosscheckBySource(ctx, db.ListAggregatorJobsForCrosscheckBySourceParams{
			Source:   source,
			AfterID:  afterID,
			PageSize: pageSize,
		})
		if err != nil {
			return seen, err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			afterID = r.ID
			seen++
			pending[r.CompanySlug] = append(pending[r.CompanySlug], ghost.Posting{
				ID:           r.ID,
				CompanySlug:  r.CompanySlug,
				Title:        r.Title,
				Stamped:      r.AtsAbsentAt.Valid,
				ViaRecruiter: r.ViaRecruiter,
			})
		}
		if len(rows) < pageSize {
			break
		}
	}
	return seen, judge(ctx, q, source, boards, pending, rep)
}

// judge cross-checks each buffered company against its own board and writes the
// verdict when the run applies.
func judge(ctx context.Context, q *db.Queries, source string, boards []string, pending map[string][]ghost.Posting, rep *report) error {
	for slug, postings := range pending {
		board, err := readBoard(ctx, q, slug, boards)
		if err != nil {
			return err
		}
		res := ghost.Crosscheck(postings, board)
		rep.add(source, slug, postings, res)

		if rep.apply {
			if len(res.Stamp) > 0 {
				if err := q.StampJobATSAbsent(ctx, res.Stamp); err != nil {
					return err
				}
			}
			if len(res.Clear) > 0 {
				if err := q.ClearJobATSAbsent(ctx, res.Clear); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// readBoard reads what Crosscheck needs of one company's own board. An empty slug is
// not a company: Crosscheck skips every such posting whatever the board says, so its
// board — every company-less board posting in the catalogue — is not worth reading.
// Nor is the recruiter count of a company we crawl no board of, which is skipped by
// the coverage gate first.
func readBoard(ctx context.Context, q *db.Queries, slug string, boards []string) (ghost.Board, error) {
	var b ghost.Board
	if slug == "" {
		return b, nil
	}
	titles, err := q.ListCompanyBoardTitles(ctx, db.ListCompanyBoardTitlesParams{
		CompanySlug:  slug,
		BoardSources: boards,
	})
	if err != nil || len(titles) == 0 {
		return b, err
	}
	b.Titles = titles
	b.RecruiterPostings, err = q.CountCompanyRecruiterPostings(ctx, slug)
	return b, err
}
