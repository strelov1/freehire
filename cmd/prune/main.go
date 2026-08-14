// Command prune permanently removes jobs that do not belong on an IT job board, and
// reports the boards whose companies have never posted anything technical.
//
// It is the destructive half of the catalogue-pruning loop. Nothing it deletes can be
// recovered from the database — the archive table records identity and title only — so
// every path defaults to reporting and requires an explicit flag to act.
//
// --boards is the report the company-scoped rules depend on. Those rules have no
// counterpart at crawl time, so a deletion under one is undone by the next hourly crawl
// unless the board leaves sources/ in the same step. This lists the candidates: boards
// still listed whose postings were classified and none of them came out technical.
//
// Boards nothing of which has been classified are withheld and counted, not listed. Most
// of the catalogue carries no is_tech verdict — 62% of rows when this was measured — and
// a board whose postings the dictionaries could not place has shown no technical signal
// for want of any signal at all. Reading that as "never posted anything technical" struck
// live IT employers on the first run of this report.
//
// Retiring a board means MOVING its entry to sources/retired/<provider>.yml, not
// deleting it. Ingest takes one file by path and a glob does not descend, so an entry
// there is neither crawled nor seen by this guard — the retirement is expressed by where
// the line lives, and a board retired by mistake is restored by moving it back.
//
// Without --apply the worker scans, reports what it would remove, and exits. --apply is
// the only way to delete anything, and --limit caps how much a single run takes: the
// first live run should be a small fraction of what matches, not the whole campaign.
//
// Usage:
//
//	go run ./cmd/prune                       # dry run: what would go, and why
//	go run ./cmd/prune --apply --limit=50000 # remove at most 50k rows
//	go run ./cmd/prune --boards              # board-retirement report
//	go run ./cmd/prune --retire              # perform the move the report describes
//
// --retire edits the board files rather than the database, and the edit is line-based
// so the files keep their comments and the diff stays reviewable. It never moves a
// provider's last entry: that is the one irreversible step here, because a job nobody
// can re-crawl can never be pruned either.
//
// Needs DATABASE_URL; MEILI_URL/MEILI_MASTER_KEY when applying, so the search index
// loses the documents in the same step.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/skilltag"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

// scanPage is how many rows one keyset page carries, and deleteBatch how many ids one
// delete statement takes. The scan page is large because the rule is cheap and most
// rows do not match; the delete batch is small because each one is a transaction that
// cascades into user_jobs and its ids are mirrored into the search index.
const (
	scanPage    = 5000
	deleteBatch = 500
	// mirrorBatch is how many ids go to the search engine at once, and it is
	// deliberately far larger than the delete batch. Meilisearch runs one task per
	// index at a time and the worker waits for each, so a task per 500-row batch put
	// two hundred sequential waits in front of a 50k run — measured on prod at 505
	// rows in eight minutes, about thirteen hours for the whole run, with ingest's own
	// indexing queued behind it the entire time. The transaction size and the mirror
	// size answer different constraints and should not be the same number.
	mirrorBatch = 10000
	// progressEvery is how often the scan reports. A full pass reads ~4000 rows a
	// second whatever the page size — the cost is per-row I/O, not per-query — so it
	// runs for tens of minutes, and the first prod run was impossible to distinguish
	// from a hang.
	progressEvery = 200000
)

func run() int {
	boardReport := flag.Bool("boards", false, "report board entries whose company has never posted anything technical")
	retire := flag.Bool("retire", false, "move the boards --boards lists into sources/retired/ (edits the source files; review the diff)")
	sourcesDir := flag.String("sources", "sources", "directory holding the board files")
	apply := flag.Bool("apply", false, "actually delete; without it the run only reports")
	flag.Bool("dry-run", false, "no-op: reporting is the default, --apply is what deletes")
	limit := flag.Int("limit", 0, "stop after this many target rows; required with --apply, -1 to run uncapped")
	sampleSize := flag.Int("sample", 200, "how many random matched titles the report prints")
	seed := flag.Uint64("seed", 1, "sampling seed, so a dry run can be reproduced")
	tracerClickDays := flag.Int("tracer-click-days", 180, "delete CV tracer clicks older than this many days (0 disables the sweep)")
	flag.Parse()

	// An uncapped run has to be asked for in as many words. The first live run should
	// be a small fraction of what matches, and a bare --apply (or a typo'd --limit=0)
	// would otherwise remove everything in one unattended pass.
	if *apply && *limit == 0 {
		log.Print("prune: --apply requires --limit (use --limit=-1 to run uncapped, deliberately)")
		return 1
	}
	if *sampleSize < 0 {
		log.Print("prune: --sample must not be negative")
		return 1
	}

	// Read the board files before touching the database. They gate the irreversible
	// rules, and an unreadable directory must stop the run before anything is removed
	// rather than yield an empty listing that reads as "every board is retired".
	brd, err := loadBoards(*sourcesDir)
	if err != nil {
		log.Printf("prune: %v", err)
		return 1
	}

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)

	// Company evidence is computed once, before any deletion, so a single run cannot
	// reclassify a company underneath itself as its own rows disappear.
	ev, err := q.CompanyTechEvidence(ctx)
	if err != nil {
		log.Printf("prune: company evidence: %v", err)
		return 1
	}

	if *boardReport {
		if err := reportBoards(ctx, os.Stdout, q, brd); err != nil {
			log.Printf("prune: write report: %v", err)
			return 1
		}
		return 0
	}

	// --retire performs the move the report describes. It edits source files, not the
	// database, and it is reversible by moving a line back — but it is still the step
	// that stops a board being crawled, so it prints the whole list before acting and
	// leaves the review to the diff.
	if *retire {
		list, withheld, err := boardsToRetire(ctx, q, brd)
		if err != nil {
			log.Printf("prune: board evidence: %v", err)
			return 1
		}
		log.Printf("prune: %d boards to retire (%d withheld for want of a verdict)", len(list), withheld)
		if len(list) == 0 {
			return 0
		}
		moved, held, err := retireBoards(*sourcesDir, list)
		if err != nil {
			log.Printf("prune: retire: %v (files already written stay written)", err)
			return 1
		}
		log.Printf("prune: moved %d entries into %s/retired/", moved, *sourcesDir)
		if len(held) > 0 {
			// Not a failure: the entries are real candidates whose turn has not come.
			log.Printf("prune: held back every entry of %s — moving them would leave the provider "+
				"with no board at all, and a job that cannot be re-crawled can never be pruned. "+
				"Prune their jobs first, then move these deliberately.", strings.Join(held, ", "))
		}
		return 0
	}

	// The tracer-click retention sweep. Age-based and unrelated to the job-pruning campaign below,
	// but it belongs here rather than in a worker of its own: this is the repository's single
	// hard-delete path, and a second binary for one DELETE is not warranted.
	//
	// It obeys --apply like everything else here, and it deletes only the clicks. The tokens stay:
	// an already-sent PDF must keep redirecting long after the clicks behind it have aged out.
	if *tracerClickDays > 0 {
		if err := sweepTracerClicks(ctx, q, *tracerClickDays, *apply); err != nil {
			log.Printf("prune: tracer clicks: %v", err)
			return 1
		}
	}

	var index docDeleter
	if *apply {
		if cfg.MeiliURL == "" || cfg.MeiliKey == "" {
			log.Print("prune: --apply needs MEILI_URL and MEILI_MASTER_KEY — deleting rows while the index keeps serving them would 404 every result")
			return 1
		}
		index = search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	}

	p, err := scan(ctx, q, ev, brd, *limit, *sampleSize, rand.New(rand.NewPCG(*seed, 0)))
	if err != nil {
		log.Printf("prune: scan: %v", err)
		return 1
	}

	if !*apply {
		if err := p.report(os.Stdout, false); err != nil {
			log.Printf("prune: write report: %v", err)
			return 1
		}
		log.Print("dry run — pass --apply to delete")
		return 0
	}

	// Print the plan before removing anything, so the run's own log records what it
	// was about to do even if it dies partway.
	if err := p.report(os.Stdout, false); err != nil {
		log.Printf("prune: write report: %v", err)
		return 1
	}

	code := 0
	if err := deleteTargets(ctx, q, index, p); err != nil {
		// Batches already committed stay committed, so the outcome has to be printed
		// on this path too — otherwise a failure leaves one error line and no record
		// of what went. pruned_jobs holds the durable version.
		log.Printf("prune: delete: %v (rows already removed are recorded in pruned_jobs)", err)
		code = 1
	}
	if err := p.report(os.Stdout, true); err != nil {
		log.Printf("prune: write report: %v", err)
		return 1
	}
	return code
}

// scan walks the catalogue by keyset and collects what the rule matches, stopping the
// collection (but not the count) at the cap.
func scan(ctx context.Context, q candidateSource, ev []db.CompanyTechEvidenceRow, brd boards, limit, sampleSize int, rnd *rand.Rand) (*plan, error) {
	type companyKey struct{ source, slug string }
	byCompany := make(map[companyKey]evidence, len(ev))
	for _, r := range ev {
		byCompany[companyKey{r.Source, r.CompanySlug}] = evidence{anyTech: r.AnyTech, anySkills: r.AnySkills}
	}

	p := newPlan(sampleSize, rnd)
	var after, scanned, reported int64
	start := time.Now()
	for {
		rows, err := q.PruneCandidates(ctx, db.PruneCandidatesParams{AfterID: after, PageSize: scanPage})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			log.Printf("prune: scanned %d rows, %d matched, %s elapsed — done", scanned, p.matched, time.Since(start).Round(time.Second))
			return p, nil
		}
		scanned += int64(len(rows))
		if scanned-reported >= progressEvery {
			reported = scanned
			log.Printf("prune: scanned %d rows, %d matched, %d refused, %s elapsed",
				scanned, p.matched, refusedTotal(p), time.Since(start).Round(time.Second))
		}
		for _, row := range rows {
			after = row.ID
			c := candidate{CompanySlug: row.CompanySlug, Title: row.Title, Category: row.Category}
			if row.IsTech.Valid {
				v := row.IsTech.Bool
				c.IsTech = &v
			}
			known := brd.knownProvider(row.Source)
			rule, ok := matchRule(c, byCompany[companyKey{row.Source, row.CompanySlug}],
				known, brd.crawls(row.Source, row.ExternalID))
			if !ok {
				// Surface what the source gate turned down. Without this the operator
				// sees only what would go, never what the guards held back — and the
				// guards are the reason to trust the number at all.
				if !known && wouldMatchButForTheSource(c) {
					p.refuse("source is not a crawled board platform: " + row.Source)
				}
				continue
			}
			if limit > 0 && len(p.targets) >= limit {
				// Counted, not taken: the report must say how much work is left. It is
				// deliberately NOT sampled — the sample exists to show what this run
				// will delete, and feeding the reservoir rows the cap excluded empties
				// it of real titles exactly when a cap is in use, which is every first
				// live run.
				p.matched++
				continue
			}
			p.add(row, rule)
		}
	}
}

// deleteTargets removes the planned rows and mirrors them out of the search indexes.
//
// The two batch sizes are separate on purpose. The database batch is small because each
// one is a transaction that cascades into user data; the mirror batch is twenty times
// larger because Meilisearch runs one task per index at a time and the worker waits for
// each, so a task per transaction serialises the whole run behind the search engine —
// and puts ingest's own indexing in the queue behind it.
//
// Rows are always removed from Postgres first. A document left in the index is served
// for a row that no longer exists until the next reindex; a row deleted from the index
// but still in Postgres is invisible to search and repaired by the same reindex. The
// index is rebuildable and Postgres is not, so the ordering follows that.
//
// The index deletions are ENQUEUED, not awaited. Meilisearch runs one task per index at
// a time and a delete-by-id rebuilds the affected parts of the inverted index, so its
// cost tracks index size rather than batch size — measured on prod, the database sat
// idle for minutes per flush while a task ran. Awaiting them spent almost the whole
// campaign not deleting. The accepted cost is that search serves rows that are gone
// until the tasks drain, which is why the campaign ends in a full reindex.
func deleteTargets(ctx context.Context, q batchDeleter, index docDeleter, p *plan) error {
	start := time.Now()
	var pending []int64

	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := index.SubmitJobDeletion(ctx, pending); err != nil {
			return err
		}
		// search is served straight from Meilisearch with no Postgres hydration, so a
		// document left in the index keeps appearing in results whose row is gone.
		log.Printf("prune: deleted %d rows, %d index deletions enqueued, %s elapsed",
			p.deleted, len(pending), time.Since(start).Round(time.Second))
		pending = pending[:0]
		return nil
	}

	for from := 0; from < len(p.targets); from += deleteBatch {
		batch := p.targets[from:min(from+deleteBatch, len(p.targets))]

		ids := make([]int64, len(batch))
		rules := make([]string, len(batch))
		for i, t := range batch {
			ids[i], rules[i] = t.id, t.rule
		}

		deleted, err := q.PruneJobs(ctx, db.PruneJobsParams{Ids: ids, Rules: rules})
		if err != nil {
			return err
		}
		p.deleted += len(deleted)
		pending = append(pending, deleted...)
		if len(pending) >= mirrorBatch {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// refusedTotal sums the guard's refusals for the progress line.
func refusedTotal(p *plan) int {
	var n int
	for _, c := range p.refused {
		n += c
	}
	return n
}

// wouldMatchButForTheSource reports whether a rule would have fired had the posting come
// from a crawled board platform. It exists only so the report can count what the source
// gate held back; it must never decide a deletion.
func wouldMatchButForTheSource(c candidate) bool {
	_, ok := matchRule(c, evidence{}, true, false)
	return ok
}

// The two dependencies the destructive path needs, named so it can be tested without a
// database or a search engine.
type (
	candidateSource interface {
		PruneCandidates(context.Context, db.PruneCandidatesParams) ([]db.PruneCandidatesRow, error)
	}
	batchDeleter interface {
		PruneJobs(context.Context, db.PruneJobsParams) ([]int64, error)
	}
	docDeleter interface {
		SubmitJobDeletion(context.Context, []int64) error
	}
)

// boardVerdict is what one board's postings collectively said about it.
//
// The two fields answer different questions and the report needs both. A board is a
// retirement candidate only when something was classified AND none of it was technical;
// with only the first field, "nothing was classified" is indistinguishable from
// "everything was classified as non-technical".
type boardVerdict struct {
	// technical is the evidence that keeps a board: a posting resolved as technical,
	// or one carrying a tagged ENGINEERING skill. Any tag will not do — the dictionary
	// covers the recruiting, HR, finance, legal and operations craft a technical company
	// hires for, so a recruiting coordinator carries skills without saying anything
	// about the employer.
	technical bool
	// determined is whether ANY posting got an is_tech verdict either way. is_tech is
	// tri-state on purpose — jobderive leaves it NULL rather than coercing, so that the
	// unclassified mass stays measurable — and this is where that third state is read.
	determined bool
}

// boardsToRetire walks the catalogue once and sorts every listed board into the three
// states boardVerdict describes, returning the retirement candidates and how many were
// withheld for want of any verdict. Both --boards and --retire go through it, so the
// list an operator reads and the list the mover acts on cannot diverge.
func boardsToRetire(ctx context.Context, q candidateSource, brd boards) ([]boardKey, int, error) {
	evidence := map[boardKey]boardVerdict{}
	var after int64
	// Same accounting the scan above keeps, and for the same reason: this walks the
	// whole catalogue too, so without it the report is twenty silent minutes in which
	// a working run and a wedged one look identical.
	var scanned, reported int64
	start := time.Now()
	for {
		rows, err := q.PruneCandidates(ctx, db.PruneCandidatesParams{AfterID: after, PageSize: scanPage})
		if err != nil {
			return nil, 0, err
		}
		if len(rows) == 0 {
			break
		}
		scanned += int64(len(rows))
		if scanned-reported >= progressEvery {
			reported = scanned
			log.Printf("prune: board report scanned %d rows, %d boards seen, %s elapsed",
				scanned, len(evidence), time.Since(start).Round(time.Second))
		}
		for _, row := range rows {
			after = row.ID
			board, ok := brd.boardOf(row.Source, row.ExternalID)
			if !ok {
				continue // not from a listed board: nothing to retire
			}
			key := boardKey{Provider: row.Source, Board: board}
			v := evidence[key]
			v.technical = v.technical || (row.IsTech.Valid && row.IsTech.Bool) || skilltag.HasEngineering(row.Skills)
			v.determined = v.determined || row.IsTech.Valid
			evidence[key] = v
		}
	}

	var retire []boardKey
	var withheld int
	for key, v := range evidence {
		switch {
		case v.technical:
			// Something technical: the board stays, as before.
		case v.determined:
			retire = append(retire, key)
		default:
			withheld++
		}
	}
	sort.Slice(retire, func(i, j int) bool {
		if retire[i].Provider != retire[j].Provider {
			return retire[i].Provider < retire[j].Provider
		}
		return retire[i].Board < retire[j].Board
	})
	return retire, withheld, nil
}

// reportBoards lists the boards still in the source files whose postings were classified
// and none of them came out technical — no technical title or category, and not one
// tagged engineering skill. Each is a candidate for the retirement PR — move the entry to
// sources/retired/<provider>.yml — which is the precondition for pruning its jobs under
// a company-scoped rule.
//
// A board no posting of which has been classified is WITHHELD, not listed. The report's
// premise is "this board has never posted anything technical", and absence of a
// technical signal only carries that meaning where a signal was possible at all: a
// posting the dictionaries could not place leaves is_tech NULL, which is not evidence of
// anything. Measured on prod the distinction decided most of the report — 11023 of 17841
// listed boards had no verdict on a single posting, 62% of the list, against 10.6% among
// the boards the same run kept. Retiring on that basis would have struck live IT
// employers whose only fault was a title the dictionary does not carry.
//
// It groups by BOARD rather than by company because that is the identity the source
// files and the catalogue share exactly; the company slug diverges wherever an adapter
// takes the name from the posting payload, which on some providers is most of them.
func reportBoards(ctx context.Context, w io.Writer, q candidateSource, brd boards) error {
	retire, withheld, err := boardsToRetire(ctx, q, brd)
	if err != nil {
		return err
	}
	// Say what the guard held back. A report that silently shrinks reads as "this is
	// everything", and the withheld boards never come back into view — the same reason
	// the scan counts its own refusals rather than dropping them.
	if withheld > 0 {
		if _, err := fmt.Fprintf(w,
			"withheld %d boards: no posting of theirs has been classified either way, so nothing is known about them\n\n",
			withheld); err != nil {
			return err
		}
	}
	if len(retire) == 0 {
		_, err := fmt.Fprintln(w, "every listed board that has been classified has posted something technical")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprint(w, "move these entries to sources/retired/<provider>.yml:\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, "PROVIDER\tBOARD"); err != nil {
		return err
	}
	for _, k := range retire {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", k.Provider, k.Board); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return warnDrainedProviders(w, retire, brd)
}

// warnDrainedProviders names the providers the list above would leave with no boards at
// all. Those entries are still genuine candidates, but the order in which they move is
// load-bearing and irreversible: cmd/ingest takes one board file by path, so a provider
// with nothing left in sources/ is never crawled again — and the company-scoped rules
// refuse a job they cannot re-crawl, so its postings can never be pruned either. Move
// such an entry and the dead weight is permanent.
//
// sources/retired/README.md states the rule (prune the provider's jobs first, move its
// last entry after), but a rule that lives only in prose is enforced by whoever happens
// to read it. The report is the thing an operator actually has in front of them.
func warnDrainedProviders(w io.Writer, retire []boardKey, brd boards) error {
	retiring := map[string]int{}
	for _, k := range retire {
		retiring[k.Provider]++
	}
	var drained []string
	for provider, n := range retiring {
		if n == len(brd.byProvider[provider]) {
			drained = append(drained, provider)
		}
	}
	if len(drained) == 0 {
		return nil
	}
	sort.Strings(drained)
	_, err := fmt.Fprintf(w,
		"\nCAUTION — every listed board of these providers is above, so moving them all "+
			"empties the provider: %s\n"+
			"Prune their jobs FIRST. A provider with no entry in sources/ is never crawled "+
			"again, and the company-scoped rules refuse a job they cannot re-crawl — its\n"+
			"postings would become permanently un-prunable. Move the last entry of each only "+
			"after its jobs are gone.\n",
		strings.Join(drained, ", "))
	return err
}

// sweepTracerClicks removes click records past the retention window, or reports what it would
// remove. Counting first costs one extra query and is what lets a dry run say a number.
func sweepTracerClicks(ctx context.Context, q *db.Queries, days int, apply bool) error {
	window := pgtype.Interval{Days: int32(days), Valid: true}
	if !apply {
		n, err := q.CountExpiredTracerClicks(ctx, window)
		if err != nil {
			return err
		}
		if n > 0 {
			log.Printf("prune: %d tracer click(s) older than %dd would be deleted (--apply to remove)", n, days)
		}
		return nil
	}
	n, err := q.DeleteExpiredTracerClicks(ctx, window)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("prune: deleted %d tracer click(s) older than %dd", n, days)
	}
	return nil
}
