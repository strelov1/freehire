// Command report-unclassified-titles is a hand-run, read-only report: it ranks the
// distinct job titles that NEITHER dictionary places — no tech-title signal from
// internal/dict/classify.IsTech and no category from classify.Parse — by how many
// publishable postings carry them.
//
// It is the third report beside cmd/report-skill-gaps and cmd/report-classify-drift,
// and it sees what neither of those can. Both of them mine LLM enrichment output, and
// enrichment is gated on is_tech IS TRUE — so a title no dictionary places is never
// enriched, has no model opinion recorded against it, and can never appear in a drift
// report (which compares against that opinion) or a skill-gap report (which mines it).
// Measured on prod 2026-09-23, that blind spot held 2,232,773 open canonical postings,
// 71,314 of them on titles reading as software or IT outright; acting on one hand-made
// sample of it returned ~110 terms to the tech-title dictionary.
//
// It writes nothing — no database row, no dictionary source file — and is on no timer.
// A curator runs it, reads the ranked list, and decides by hand what belongs in
// internal/dict/classify. It does NOT suggest terms: a generated suggestion would be
// read as an answer, and the whole argument for a curated dictionary is that every
// entry was looked at.
//
// Expect the top of the list to be non-technical and to stay that way — seamstresses,
// chambermaids, retail apprenticeships. That is the catalogue being right about them.
// The report's value is the software titles sitting in volume further down.
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/strelov1/freehire/internal/job/dictgap"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// defaultChunkSize is how many ids one chunk's id range spans. The query GROUPs BY
// title with no row LIMIT (see ListTitlesForUnclassifiedReport), so the range width
// alone bounds one statement's cost.
//
// NOT the 50,000 cmd/report-classify-drift uses, and the first run of this report is
// what settled that. The id sequence is far sparser than the row count: measured on
// prod 2026-09-23, 12.7M rows spread over a max id of 1,620,699,741, i.e. about 8
// rows per thousand ids. At 50,000 the walk is 32,414 chunks, most of them empty,
// and the run projected to ~3.5 hours — nearly all of it the 200ms courtesy pause
// between statements rather than any work. At 2,000,000 the same report finished in
// 25 minutes, and each chunk still averages only ~15k rows, so the statement stays
// cheap. The knob a run can move is the chunk; what it cannot move is a default
// that makes the first run look broken.
const defaultChunkSize = 2_000_000

// defaultTopN is how many ranked candidates the report prints. The catalogue holds
// over 1.5M distinct unrecognised titles, so the cap is what makes the output
// readable at all; raise it through the env knob when curating in depth.
const defaultTopN = 500

// pauseBetweenChunks lets the host breathe between statements, the same courtesy
// every other chunked one-off tool in this repo pays.
const pauseBetweenChunks = 200 * time.Millisecond

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	step, err := worker.EnvInt64("REPORT_UNCLASSIFIED_CHUNK", defaultChunkSize)
	if err != nil {
		log.Printf("report-unclassified-titles: %v", err)
		return 1
	}
	topN, err := worker.EnvInt32("REPORT_UNCLASSIFIED_TOP", defaultTopN)
	if err != nil {
		log.Printf("report-unclassified-titles: %v", err)
		return 1
	}
	q := db.New(pool)
	bounds, err := q.UnclassifiedTitleReportBounds(ctx)
	if err != nil {
		log.Printf("report-unclassified-titles: bounds: %v", err)
		return 1
	}
	if bounds.MaxID == 0 {
		log.Print("report-unclassified-titles: nothing to do")
		return 0
	}

	// No resume knob, deliberately, and this is the one place where copying
	// cmd/report-classify-drift would have been wrong. The ranking is over counts
	// accumulated across the WHOLE span: starting at a later id leaves the
	// accumulator empty for everything before it, so a title carried by postings on
	// both sides of the cursor is undercounted and the top-N describes a suffix of
	// the catalogue while looking exactly like a report on all of it. A read-only
	// report can simply be run again; a plausible wrong number cannot be spotted.
	from := bounds.MinID
	log.Printf("report-unclassified-titles: ids %d..%d, chunk=%d", bounds.MinID, bounds.MaxID, step)

	counts := map[string]int{}
	var chunkRows int64
	lastLog := time.Now()
	for from <= bounds.MaxID {
		to := from + step
		rows, err := q.ListTitlesForUnclassifiedReport(ctx, db.ListTitlesForUnclassifiedReportParams{
			FromID: from,
			ToID:   to,
		})
		if err != nil {
			log.Printf("report-unclassified-titles: read chunk %d..%d after %d rows: %v", from, to, chunkRows, err)
			return 1
		}
		chunkRows += int64(len(rows))
		mergeTitleCounts(counts, rows)
		from = to

		if time.Since(lastLog) >= time.Minute {
			log.Printf("report-unclassified-titles: progress chunk-rows=%d distinct-titles=%d at id=%d of %d",
				chunkRows, len(counts), from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("report-unclassified-titles: cancelled at id=%d after %d rows — no partial report is printed; run it again from the start",
				from, chunkRows)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}

	rows := make([]dictgap.TitleCount, 0, len(counts))
	for title, count := range counts {
		rows = append(rows, dictgap.TitleCount{Title: title, Count: count})
	}
	candidates := dictgap.UnclassifiedTitles(rows)
	log.Printf("report-unclassified-titles: done, distinct titles=%d, unplaced=%d, printing top %d",
		len(counts), len(candidates), topN)
	if err := writeReport(os.Stdout, candidates, int(topN)); err != nil {
		log.Printf("report-unclassified-titles: write report: %v", err)
		return 1
	}
	return 0
}

// mergeTitleCounts folds one chunk's per-title rows into the running accumulator.
// Grouping happens per chunk, not across the whole table, so the same title arrives
// once per id range it appears in and the counts are summed — a report ranking a
// title by its largest chunk would rank the wrong things first.
func mergeTitleCounts(acc map[string]int, rows []db.ListTitlesForUnclassifiedReportRow) {
	for _, r := range rows {
		acc[r.Title] += int(r.JobCount)
	}
}

// writeReport prints the top n candidates as a tab-separated count/title table,
// highest count first (dictgap.UnclassifiedTitles already sorts them). Count first
// so the output pipes into sort, cut and grep — the tools a curator actually reads a
// list this long with.
func writeReport(w io.Writer, candidates []dictgap.TitleCount, n int) error {
	if n < len(candidates) {
		candidates = candidates[:n]
	}
	for _, c := range candidates {
		if _, err := fmt.Fprintf(w, "%d\t%s\n", c.Count, c.Title); err != nil {
			return err
		}
	}
	return nil
}
