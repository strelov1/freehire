// Command backfill-link-contributions carries the rows #2357 left behind.
//
// That change moved the crowdsourced contribution lifecycle out of link_contributions and
// into boards + board_submissions — the read and write paths, but not the data. Measured
// on prod 2026-09-04: link_contributions holds 404 rows from 11 users, of which boards
// carries 3. So 401 contributions stopped being visible on their submitters' "my
// contributions" page, and 28 of them — 26 unclassified URLs and 2 recognized boards —
// stopped being actionable, because nothing lists them for triage any more.
//
// It is the precondition for dropping link_contributions, which the board-catalog design
// asks for and which would otherwise destroy those 28. Run it once, confirm the counts,
// then drop the table in a separate change — the same ordering cmd/backfill-marker-owner
// uses, and for the same reason: the drop is what makes a missed row unrecoverable.
//
// Each of the four statuses carries differently, because they mean different things:
//
//   - review    — an unclassified URL nobody has resolved into (provider, board). Goes to
//     board_submissions, which is exactly that inbox.
//   - pending   — a recognized board waiting to be onboarded. Goes to boards at
//     status='pending', which is crawled, so onboarding happens by itself.
//   - onboarded — already a catalog row, but the YAML backfill carried no attribution, so
//     the row names no submitter. UPDATEs the existing row rather than
//     inserting a second one. An onboarded contribution with NO catalog row
//     is reported separately (9 on prod): it is not history, it is a board
//     nothing crawls, and counting it as "already carried" would let the
//     drop take it.
//   - rejected  — NOT carried. A refusal buys nothing forward: migration 0049 already
//     freed the identity so the board can be re-contributed, and if it is,
//     it gets judged again on the day. Carrying 204 of them would put dead
//     rows in the catalog permanently and would overload boards' own
//     'rejected', which means "failed insert-time validation" — a different
//     thing wearing the same word. The drop lets them go, deliberately.
//
// Reports by default, writes under --apply, and is idempotent: every statement is
// conflict-guarded or NULL-guarded, so a re-run writes nothing and stopping it mid-way is
// free.
//
//	go run ./cmd/backfill-link-contributions            # report what would move
//	go run ./cmd/backfill-link-contributions --apply    # move it
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

// destination is where one contribution goes. Naming the five outcomes lets the run
// classify every row before it writes anything, and lets the report print what it found
// without a counter per branch.
type destination string

const (
	toSubmission   destination = "submission"
	toPendingBoard destination = "pendingBoard"
	toAttribution  destination = "attribution"
	dropRefusal    destination = "refusal"
	unplaceable    destination = "unplaceable"
)

// label is what the report prints. Kept apart from the value so an error or a failing test
// says `"attribution"` rather than a sentence.
var label = map[destination]string{
	toSubmission:   "unclassified URLs -> board_submissions",
	toPendingBoard: "recognized boards -> boards (pending)",
	toAttribution:  "already-onboarded boards -> attributed to their submitter",
	dropRefusal:    "refusals -> dropped (see the package doc)",
	unplaceable:    "NEEDS A HUMAN: a recognized status carrying no board",
}

// writes reports whether this destination has a statement behind it. The other two are
// decisions, not work: a refusal is deliberately let go, and an unplaceable row needs a
// human rather than a query.
func (d destination) writes() bool {
	return d == toSubmission || d == toPendingBoard || d == toAttribution
}

// order fixes the report's line order, since a map has none.
var order = []destination{toSubmission, toPendingBoard, toAttribution, dropRefusal, unplaceable}

func run() int {
	apply := flag.Bool("apply", false, "actually write; without it the run only reports")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)
	repo := boardcatalog.NewQueriesRepository(q)
	ins := boardcatalog.NewInserter(repo, sources.Taxonomy())

	rows, err := q.ListLinkContributionsForBackfill(ctx)
	if err != nil {
		log.Printf("backfill-link-contributions: list: %v", err)
		return 1
	}
	log.Printf("backfill-link-contributions: %d contributions", len(rows))

	counts := map[destination]int{}
	already, orphaned := 0, 0
	for _, row := range rows {
		dest, err := destinationOf(row)
		if err != nil {
			log.Printf("backfill-link-contributions: row %d: %v", row.ID, err)
			return 1
		}
		counts[dest]++
		if !*apply || !dest.writes() {
			continue
		}
		wrote, err := write(ctx, q, ins, row, dest)
		if err != nil {
			log.Printf("backfill-link-contributions: row %d (%s): %v", row.ID, row.Status, err)
			return 1
		}
		if wrote {
			continue
		}
		// An attribution that changed nothing is ambiguous — the row may already name a
		// submitter, or the catalog may hold no such board at all. Only the second is a
		// problem, and only a second query can tell them apart.
		if dest == toAttribution {
			listed, err := inCatalog(ctx, repo, row)
			if err != nil {
				log.Printf("backfill-link-contributions: row %d: %v", row.ID, err)
				return 1
			}
			if !listed {
				orphaned++
				continue
			}
		}
		already++
	}

	for _, d := range order {
		if counts[d] > 0 {
			log.Printf("backfill-link-contributions: %4d  %s", counts[d], d)
		}
	}
	if orphaned > 0 {
		log.Printf("backfill-link-contributions: %d onboarded contributions name a board the "+
			"catalog does not carry — they are boards nothing crawls, not history. "+
			"Resolve them before dropping link_contributions.", orphaned)
	}
	if !*apply {
		log.Print("backfill-link-contributions: dry run, nothing written. Re-run with --apply.")
		return 0
	}
	log.Printf("backfill-link-contributions: %d writable rows were already carried", already)
	// A row the run could not place is a per-item failure, and worker/AGENTS.md wants the
	// exit code to say so: a partially-failed run that returns 0 looks successful to cron,
	// and this one would be followed by a DROP.
	if counts[unplaceable] > 0 || orphaned > 0 {
		return 1
	}
	return 0
}

// destinationOf classifies one contribution. It reads the row and nothing else, so the
// rules below are testable without a database — which matters, because they are the part
// a mistake would silently misplace data through.
func destinationOf(row db.ListLinkContributionsForBackfillRow) (destination, error) {
	// 'review' is the only status the schema lets carry a NULL source, and its
	// destination needs no board at all.
	if row.Status == "review" {
		return toSubmission, nil
	}
	if !row.Source.Valid || !row.Board.Valid || row.Source.String == "" || row.Board.String == "" {
		// A recognized status with no board names nothing that can be carried. Counted
		// rather than dropped silently: a nonzero here means the schema's own assumption
		// no longer holds and the row needs a human.
		return unplaceable, nil
	}
	switch row.Status {
	case "onboarded":
		return toAttribution, nil
	case "pending":
		return toPendingBoard, nil
	case "rejected":
		return dropRefusal, nil
	}
	return "", fmt.Errorf("unknown status %q", row.Status)
}

// write performs one destination's statement, reporting whether it changed a row. False
// is the ordinary re-run answer, not a failure: every statement here is conflict- or
// NULL-guarded, which is what makes the whole worker idempotent.
func write(ctx context.Context, q *db.Queries, ins *boardcatalog.Inserter, row db.ListLinkContributionsForBackfillRow, dest destination) (bool, error) {
	switch dest {
	case toSubmission:
		n, err := q.InsertBoardSubmissionAt(ctx, db.InsertBoardSubmissionAtParams{
			URL:         row.URL,
			SubmittedBy: row.SubmittedBy,
			Surface:     row.Surface,
			CreatedAt:   row.CreatedAt,
		})
		return n > 0, err

	case toPendingBoard:
		// Through boardcatalog.Insert, not a direct INSERT: it trims the board id,
		// validates the provider against the adapter registry (storing a failure as
		// 'rejected' with a reason rather than adding an uncrawlable row), and folds the
		// board spellings the unique index cannot. A legacy contribution names a provider
		// that may have left the registry since, which is exactly what that check is for.
		b, err := ins.Insert(ctx, boardcatalog.InsertInput{
			Provider: row.Source.String,
			Board:    row.Board.String,
			// A contribution never carried a company name — recognition from a URL is
			// network-free — so it gets the same placeholder the live flow uses, and a
			// curator corrects it with cmd/add-board --rename.
			Company:     boardcatalog.PlaceholderCompany(row.Board.String),
			URL:         row.URL,
			SubmittedBy: &row.SubmittedBy,
			Surface:     row.Surface,
			CreatedAt:   row.CreatedAt.Time,
		}, boardcatalog.StatusPending)
		if errors.Is(err, boardcatalog.ErrDuplicateBoard) {
			return false, nil // already in the catalog: the re-run case
		}
		if err != nil {
			return false, err
		}
		if b.Status == boardcatalog.StatusRejected {
			log.Printf("backfill-link-contributions: %s/%s stored as rejected: %s",
				row.Source.String, row.Board.String, b.RejectedReason)
		}
		return true, nil

	case toAttribution:
		n, err := q.AttributeBoardToSubmitter(ctx, db.AttributeBoardToSubmitterParams{
			SubmittedBy: pgconv.Int8(&row.SubmittedBy),
			URL:         pgconv.Text(row.URL),
			Surface:     row.Surface,
			Provider:    row.Source.String,
			Board:       row.Board.String,
			Region:      contributionRegion,
		})
		return n > 0, err
	}
	return false, fmt.Errorf("destination %q has no statement", dest)
}

// contributionRegion is the region a contributed board is recorded under. The contribution
// flow has no region to record — it derives (provider, board) from a URL — so both it and
// cmd/add-board write the column's default. Naming it here keeps the attribution keyed on
// the catalog's full identity instead of quietly matching every region of a board.
const contributionRegion = ""

// inCatalog reports whether the catalog carries the board a contribution names. It answers
// the one question an attribution that changed nothing leaves open: already attributed, or
// no such board at all.
func inCatalog(ctx context.Context, repo boardcatalog.Repository, row db.ListLinkContributionsForBackfillRow) (bool, error) {
	listed, err := boardcatalog.LoadForProvider(ctx, repo, row.Source.String)
	if err != nil {
		return false, fmt.Errorf("load catalog for %s: %w", row.Source.String, err)
	}
	for _, e := range listed {
		if strings.EqualFold(e.Board, row.Board.String) {
			return true, nil
		}
	}
	return false, nil
}
