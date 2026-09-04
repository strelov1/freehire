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
	log.Printf("backfill-link-contributions: %d contributions to carry", len(rows))

	var tally tally
	for _, row := range rows {
		if err := carry(ctx, q, row, *apply, &tally); err != nil {
			log.Printf("backfill-link-contributions: row %d (%s): %v", row.ID, row.Status, err)
			return 1
		}
	}

	verb := "would carry"
	if *apply {
		verb = "carried"
	}
	log.Printf("backfill-link-contributions: %s %d submissions and %d pending boards; "+
		"attributed %d existing boards; %d already carried, %d dropped refusals, %d skipped",
		verb, tally.submissions, tally.pending, tally.attributed, tally.noop, tally.dropped, tally.skipped)
	if !*apply {
		log.Print("backfill-link-contributions: dry run, nothing written. Re-run with --apply.")
	}
	return 0
}

// tally counts what a run did, so the report says which of the four paths each row took
// rather than one number that hides them.
type tally struct {
	submissions, pending, attributed int
	// dropped counts the refusals this worker deliberately does not carry, so the run
	// says so rather than leaving them to be inferred from a total that does not add up.
	dropped int
	// noop is a row whose destination already holds it — the re-run case.
	noop int
	// skipped is a row this worker cannot place: a recognized status with no
	// (source, board), which the schema allows only for 'review'.
	skipped int
}

func carry(ctx context.Context, q *db.Queries, row db.ListLinkContributionsForBackfillRow, apply bool, t *tally) error {
	// 'review' is the only status the schema lets carry a NULL source, and its
	// destination needs no board at all.
	if row.Status == "review" {
		return carrySubmission(ctx, q, row, apply, t)
	}
	if !row.Source.Valid || !row.Board.Valid || row.Source.String == "" || row.Board.String == "" {
		// A recognized status with no board names nothing that can be carried. Count it
		// rather than dropping it silently: a nonzero here means the schema's own
		// assumption is wrong and the row needs a human.
		t.skipped++
		return nil
	}

	switch row.Status {
	case "onboarded":
		return attribute(ctx, q, row, apply, t)
	case "pending":
		return carryBoard(ctx, q, row, apply, t)
	case "rejected":
		t.dropped++
		return nil
	}
	return fmt.Errorf("unknown status %q", row.Status)
}

func carrySubmission(ctx context.Context, q *db.Queries, row db.ListLinkContributionsForBackfillRow, apply bool, t *tally) error {
	if !apply {
		t.submissions++
		return nil
	}
	n, err := q.InsertBoardSubmissionAt(ctx, db.InsertBoardSubmissionAtParams{
		URL:         row.URL,
		SubmittedBy: row.SubmittedBy,
		Surface:     row.Surface,
		CreatedAt:   row.CreatedAt,
	})
	if err != nil {
		return err
	}
	countWritten(n, &t.submissions, t)
	return nil
}

func carryBoard(ctx context.Context, q *db.Queries, row db.ListLinkContributionsForBackfillRow, apply bool, t *tally) error {
	if !apply {
		t.pending++
		return nil
	}
	n, err := q.InsertPendingBoardAt(ctx, db.InsertPendingBoardAtParams{
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
	if err != nil {
		return err
	}
	countWritten(n, &t.pending, t)
	return nil
}

func attribute(ctx context.Context, q *db.Queries, row db.ListLinkContributionsForBackfillRow, apply bool, t *tally) error {
	if !apply {
		t.attributed++
		return nil
	}
	n, err := q.AttributeBoardToSubmitter(ctx, db.AttributeBoardToSubmitterParams{
		SubmittedBy: pgconv.Int8(&row.SubmittedBy),
		URL:         pgconv.Text(row.URL),
		Surface:     row.Surface,
		Provider:    row.Source.String,
		Board:       row.Board.String,
	})
	if err != nil {
		return err
	}
	countWritten(n, &t.attributed, t)
	return nil
}

// countWritten credits a statement that changed a row to its own counter, and one that
// changed nothing to noop. Zero rows is the ordinary re-run answer here, not a failure:
// every statement this worker issues is conflict- or NULL-guarded.
func countWritten(n int64, counter *int, t *tally) {
	if n > 0 {
		*counter++
		return
	}
	t.noop++
}
