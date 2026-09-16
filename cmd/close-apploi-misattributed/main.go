// Command close-apploi-misattributed is a one-off: closes every open apploi posting and
// retires the provider's boards, because what the catalogue holds about them is wrong and
// cannot be repaired in place.
//
// Measured on production 2026-09-15. apploi's upstream API stopped honouring its `employer`
// parameter — `employer=39092`, `employer=999999999`, `employer=52601` and no employer
// parameter at all return byte-identical pages from api.apploi.com/v1/jobs — so every one of
// its 5833 boards fetched the same GLOBAL catalogue. The adapter attributes a posting with
// `Company: firstNonEmpty(e.Company, j.BrandName)`, and all 5833 board rows carry a company,
// so the real brand_name was never once stored.
//
// The result: 1,565,701 rows standing for 3,024 real jobs — 518 copies each, under 518
// different and mostly wrong employers — of which 264,022 reach live search, where one title
// repeats down the page under a different company each time. That was 12.5% of everything a
// visitor could find, and 30% of the open catalogue.
//
// WHY CLOSE RATHER THAN REPAIR. The correct employer is not in the stored row in any form:
// company_slug came from the board, and brand_name was discarded by firstNonEmpty. Re-deriving
// it means re-fetching, and re-fetching means the boardless adapter rewrite this worker does
// NOT do. Keeping one copy per real job is not an option either — there is no way to tell
// which of the 518 companies to keep, because none of them was read from the posting.
//
// WHY NOT REWRITE THE ADAPTER FIRST. Measured before deciding: 5,074 of the 1,473,738 open
// apploi rows are is_tech, 0.34%, and at 518 copies each that is on the order of ten real
// technical jobs. apploi is a healthcare ATS; for an IT catalogue the rewrite would buy
// almost nothing. The seam is noted in gen-ingest-timers.sh if that ever changes.
//
// Soft close only, with its own closed_reason ('source_misattributed', migration 0165), which
// is also the rollback: nothing is deleted, and that label is the only thing that separates
// these rows from the ordinary closed ones afterwards.
//
// FOLLOW IT WITH A REINDEX — or simply wait for the scheduled one. The close deliberately
// does not push to search_delete_outbox (see CloseMisattributedSourceJobs for the argument):
// 1.47M entries against a queue whose ordinary depth is ~7k would sit in front of every other
// Meilisearch task, and the rebuild reads open rows from Postgres anyway, so a closed row is
// simply absent from the next index.
//
// Chunked, paced and idempotent — the close is `closed_at IS NULL` guarded, so a re-run writes
// nothing and stopping mid-way is free. CLOSE_APPLOI_CHUNK sets the id span per statement and
// CLOSE_APPLOI_FROM_ID resumes at the cursor a previous run logged.
//
// THE CHUNK IS AN ID SPAN, NOT A ROW COUNT, and on this table the two are nowhere near each
// other: jobs.id has reached 1.6 BILLION while the table holds ~11M rows, so an id range is
// about 145x wider than the number of rows in it. The first run of this worker was launched
// with the 50k default every other backfill here uses, and spent four minutes closing nothing
// — apploi's rows start at id 485,750,838, which is 9,715 empty chunks and half an hour of
// pacing away from a walk that starts at zero. It finished in 18 minutes at
// CLOSE_APPLOI_CHUNK=5000000 with CLOSE_APPLOI_FROM_ID set to that minimum.
//
// So the default is 5M, and the rule for the next id-range walk over this table is: measure
// `SELECT min(id), max(id) FROM jobs WHERE <predicate>` FIRST and set both knobs from it. The
// rows are not spread evenly either — one 5M chunk near the top closed 43,599 rows and the
// last one closed 1.4M, because ids are handed out by a sequence and a source's bulk re-ingest
// lands them together.
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// source is hard-coded rather than taken from the environment. This worker's whole
// justification is a measurement of ONE provider's data, and every paragraph above is about
// apploi; a source argument would invite pointing it at a provider nobody has measured, where
// closing the whole source is exactly the wrong thing.
const source = "apploi"

// pause keeps a multi-million-row walk from becoming the load event it is cleaning up after.
// The host this runs on fell over twice on the day this was written, both times for reasons
// unrelated to this walk and both times because something assumed it had headroom.
const pause = 200 * time.Millisecond

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	chunk, err := worker.EnvInt64("CLOSE_APPLOI_CHUNK", 5_000_000)
	if err != nil {
		log.Printf("close-apploi-misattributed: %v", err)
		return 1
	}
	fromID, err := worker.EnvInt64("CLOSE_APPLOI_FROM_ID", 0)
	if err != nil {
		log.Printf("close-apploi-misattributed: %v", err)
		return 1
	}

	q := db.New(pool)
	maxID, err := q.MaxJobIDForSource(ctx, source)
	if err != nil {
		log.Printf("close-apploi-misattributed: read max id: %v", err)
		return 1
	}
	if maxID == 0 {
		log.Printf("close-apploi-misattributed: %s holds no rows — nothing to close", source)
		return 0
	}

	var closed int64
	for id := fromID; id <= maxID; id += chunk {
		n, err := q.CloseMisattributedSourceJobs(ctx, db.CloseMisattributedSourceJobsParams{
			Source: source,
			FromID: id,
			ToID:   id + chunk,
		})
		if err != nil {
			// The cursor is the point of logging here: an interrupted walk resumes from it
			// rather than re-reading the millions of ids it already passed.
			log.Printf("close-apploi-misattributed: chunk from id %d: %v (resume with CLOSE_APPLOI_FROM_ID=%d)", id, err, id)
			return 1
		}
		closed += n
		if n > 0 {
			log.Printf("close-apploi-misattributed: closed %d (running total %d, at id %d of %d)", n, closed, id, maxID)
		}
		select {
		case <-ctx.Done():
			log.Printf("close-apploi-misattributed: interrupted after %d rows (resume with CLOSE_APPLOI_FROM_ID=%d)", closed, id)
			return 1
		case <-time.After(pause):
		}
	}

	// Retiring the boards comes AFTER the close, not before. The order does not matter to
	// either statement, but it matters to an interrupted run: boards still listed as live
	// beside half-closed jobs reads as a run that was stopped, while retired boards beside
	// open jobs reads as a provider that was withdrawn and then forgotten.
	boards, err := q.RetireProviderBoards(ctx, source)
	if err != nil {
		log.Printf("close-apploi-misattributed: retire boards: %v (jobs are closed; re-run to finish)", err)
		return 1
	}

	log.Printf("close-apploi-misattributed: closed %d postings and retired %d boards — run a reindex, or wait for the scheduled one", closed, boards)
	return 0
}
