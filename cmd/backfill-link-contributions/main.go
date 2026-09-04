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
//     inserting a second one.
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
	"flag"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
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
	toSubmission   destination = "unclassified URLs -> board_submissions"
	toPendingBoard destination = "recognized boards -> boards (pending)"
	toAttribution  destination = "already-onboarded boards -> attributed to their submitter"
	dropRefusal    destination = "refusals -> dropped (see the package doc)"
	unplaceable    destination = "UNPLACEABLE: a recognized status carrying no board"
)

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

	rows, err := q.ListLinkContributionsForBackfill(ctx)
	if err != nil {
		log.Printf("backfill-link-contributions: list: %v", err)
		return 1
	}
	log.Printf("backfill-link-contributions: %d contributions", len(rows))

	counts := map[destination]int{}
	already := 0
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
		wrote, err := write(ctx, q, row, dest)
		if err != nil {
			log.Printf("backfill-link-contributions: row %d (%s): %v", row.ID, row.Status, err)
			return 1
		}
		if !wrote {
			already++
		}
	}

	for _, d := range order {
		if counts[d] > 0 {
			log.Printf("backfill-link-contributions: %4d  %s", counts[d], d)
		}
	}
	if *apply {
		log.Printf("backfill-link-contributions: %d of the writable rows were already carried", already)
		return 0
	}
	log.Print("backfill-link-contributions: dry run, nothing written. Re-run with --apply.")
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
func write(ctx context.Context, q *db.Queries, row db.ListLinkContributionsForBackfillRow, dest destination) (bool, error) {
	var n int64
	var err error
	switch dest {
	case toSubmission:
		n, err = q.InsertBoardSubmissionAt(ctx, db.InsertBoardSubmissionAtParams{
			URL:         row.URL,
			SubmittedBy: row.SubmittedBy,
			Surface:     row.Surface,
			CreatedAt:   row.CreatedAt,
		})
	case toPendingBoard:
		n, err = q.InsertPendingBoardAt(ctx, db.InsertPendingBoardAtParams{
			Provider: row.Source.String,
			Board:    row.Board.String,
			// A contribution never carried a company name — recognition from a URL is
			// network-free — so it gets the same placeholder the live flow uses, and a
			// curator corrects it with cmd/add-board --rename.
			Company:     boardcatalog.PlaceholderCompany(row.Board.String),
			URL:         pgconv.Text(row.URL),
			SubmittedBy: pgconv.Int8(&row.SubmittedBy),
			Surface:     row.Surface,
			CreatedAt:   row.CreatedAt,
		})
	case toAttribution:
		n, err = q.AttributeBoardToSubmitter(ctx, db.AttributeBoardToSubmitterParams{
			SubmittedBy: pgconv.Int8(&row.SubmittedBy),
			URL:         pgconv.Text(row.URL),
			Surface:     row.Surface,
			Provider:    row.Source.String,
			Board:       row.Board.String,
		})
	}
	return n > 0, err
}
