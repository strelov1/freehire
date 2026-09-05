// Command backfill-requirements fills jobs.requirements_derived for the open postings
// that predate the column (migration 0135), then exits.
//
// Every write path fills the column going forward (internal/job/job's withDerived);
// this is the one-off pass for the catalogue already in the table. Once a row carries a
// list, SetJobEnrichment overlays it into the served enrichment.requirements whenever
// the model has stated none — which today is ~97% of open postings.
//
// It is NOT folded into cmd/backfill-derive, which re-derives every deterministic
// column in one ~15h pass over all ~11M rows. This walks the ~4.6M OPEN ones, writes
// one column, and so does not have to wait on that pass's schedule.
//
// Run it repeatedly and it costs nothing: the chunk UPDATE is guarded by
// IS DISTINCT FROM, so rows already carrying the derived value are not rewritten and
// produce no dead tuples. That is what makes it safe to stop and resume — including
// stopping it because the host is busy, which is the expected way to run it. A row
// whose description yields nothing keeps the '[]' it already had, so it is
// indistinguishable from an unvisited one; BACKFILL_REQUIREMENTS_FROM_ID is how a
// follow-up run skips the span a previous one already covered (the id is in its log).
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/strelov1/freehire/internal/job/reqextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// defaultChunkSize is how many ids one chunk spans, overridable with
// BACKFILL_REQUIREMENTS_CHUNK. It spans an ID RANGE, not a row count: the id sequence
// has run far ahead of the live row count through pruning, so most chunks cover empty
// stretches and a narrow width spends the pass on round trips rather than on rows.
//
// The ceiling on width is how much description text one chunk pulls into memory and
// de-TOASTs in a single statement. 5k ids is deliberately narrower than
// cmd/backfill-slug-folded's 50k for exactly that reason: this pass reads descriptions
// (tens of KB each), that one reads a slug.
const defaultChunkSize = 5_000

// defaultMaxPerRun bounds how many postings one run derives for. A `Type=oneshot`
// systemd unit that runs past its timer's next firing is a silently skipped run, so an
// unbounded pass over 4.6M rows is not something to schedule; take the backlog in
// bounded, observable runs instead.
const defaultMaxPerRun = 200_000

// pauseBetweenChunks lets the host breathe between statements. This pass competes with
// ingest and with whatever reindex is running, and it is never urgent: until it
// completes, a posting simply shows no requirements, which is what it shows today.
// Concurrency stays at one for the same reason BACKFILL_CONCURRENCY is held to 2-3
// elsewhere — a heavier setting has degraded prod before, and de-TOASTing descriptions
// is that same shape of load.
const pauseBetweenChunks = 200 * time.Millisecond

func envInt64(name string, fallback int64) int64 {
	if v, err := strconv.ParseInt(os.Getenv(name), 10, 64); err == nil && v > 0 {
		return v
	}
	return fallback
}

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	bounds, err := q.RequirementsDerivedBackfillBounds(ctx)
	if err != nil {
		log.Printf("backfill-requirements: bounds: %v", err)
		return 1
	}
	if bounds.MaxID == 0 {
		log.Print("backfill-requirements: nothing to do")
		return 0
	}

	step := envInt64("BACKFILL_REQUIREMENTS_CHUNK", defaultChunkSize)
	maxPerRun := envInt64("BACKFILL_REQUIREMENTS_MAX", defaultMaxPerRun)
	from := bounds.MinID
	// A run resumes where a previous one stopped rather than re-deriving the span it
	// already covered. Only ever forward: a start id past the table simply finds
	// nothing, which is a no-op, not a wrong answer.
	if resume := envInt64("BACKFILL_REQUIREMENTS_FROM_ID", 0); resume > from {
		from = resume
	}
	log.Printf("backfill-requirements: ids %d..%d from %d, chunk=%d, max=%d",
		bounds.MinID, bounds.MaxID, from, step, maxPerRun)

	var examined, filled int64
	lastLog := time.Now()
	for ; from <= bounds.MaxID; from += step {
		if examined >= maxPerRun {
			log.Printf("backfill-requirements: reached max=%d, stopping at id=%d — "+
				"re-run with BACKFILL_REQUIREMENTS_FROM_ID=%d", maxPerRun, from, from)
			break
		}

		rows, err := q.ListJobsForRequirementsBackfill(ctx, db.ListJobsForRequirementsBackfillParams{
			FromID: from,
			ToID:   from + step,
		})
		if err != nil {
			log.Printf("backfill-requirements: read chunk %d..%d after %d filled: %v", from, from+step, filled, err)
			return 1
		}
		examined += int64(len(rows))

		ids, payloads := derive(rows)
		if len(ids) > 0 {
			n, err := q.SetJobsRequirementsDerived(ctx, db.SetJobsRequirementsDerivedParams{
				Ids:      ids,
				Payloads: payloads,
			})
			if err != nil {
				// Report what was already committed: every chunk is its own transaction,
				// so the work done so far survives and a re-run resumes from it.
				log.Printf("backfill-requirements: write chunk %d..%d after %d filled: %v", from, from+step, filled, err)
				return 1
			}
			filled += n
		}

		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-requirements: progress examined=%d filled=%d at id=%d of %d",
				examined, filled, from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-requirements: cancelled at id=%d after %d filled — "+
				"resume with BACKFILL_REQUIREMENTS_FROM_ID=%d", from, filled, from)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}
	log.Printf("backfill-requirements: done, examined=%d filled=%d", examined, filled)
	return 0
}

// derive runs the extractor over a chunk and returns the rows worth writing: those
// whose description yields at least one requirement. A row that yields none is skipped
// rather than written as '[]' — that is already its value, and the chunk statement's
// IS DISTINCT FROM guard would discard the write anyway, so sending it would only make
// the batch bigger.
func derive(rows []db.ListJobsForRequirementsBackfillRow) ([]int64, [][]byte) {
	ids := make([]int64, 0, len(rows))
	payloads := make([][]byte, 0, len(rows))
	for _, r := range rows {
		reqs := reqextract.Derive(r.Description)
		if len(reqs) == 0 {
			continue
		}
		payload, err := json.Marshal(reqs)
		if err != nil {
			// Impossible for this shape (two strings per entry). Skip the row rather
			// than fail the pass: a posting without its list is what it has today.
			log.Printf("backfill-requirements: encode job %d: %v", r.ID, err)
			continue
		}
		ids = append(ids, r.ID)
		payloads = append(payloads, payload)
	}
	return ids, payloads
}
