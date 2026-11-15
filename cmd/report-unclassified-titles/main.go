// Command report-unclassified-titles is a hand-run, read-only report: it ranks the
// distinct job titles that NEITHER dictionary places — no tech-title signal from
// internal/dict/classify.IsTech and no category from classify.Parse — by how many
// publishable postings carry them.
//
// It sees what cmd/report-skill-gaps and cmd/report-classify-drift cannot: both mine LLM enrichment,
// which is gated on is_tech IS TRUE, so a title no dictionary places is never enriched and never
// appears in either.
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
// Much wider than cmd/report-classify-drift's 50,000: the id sequence is far sparser than the row
// count (about 8 rows per thousand ids), so small chunks are mostly empty and the run is dominated
// by the courtesy pause between statements.
const defaultChunkSize = 2_000_000

// defaultTopN is how many ranked candidates the report prints. There are far too many distinct
// unrecognised titles to print, so the cap keeps output readable; raise it through the env knob.
const defaultTopN = 500

// pauseBetweenChunks lets the host breathe between statements.
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

	// No resume knob, deliberately: the ranking is over counts accumulated across the WHOLE span, so
	// starting at a later id undercounts titles on both sides of the cursor and the top-N describes a
	// suffix of the catalogue while looking like the whole. A read-only report can simply be rerun.
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
// once per id range it appears in and the counts are summed.
func mergeTitleCounts(acc map[string]int, rows []db.ListTitlesForUnclassifiedReportRow) {
	for _, r := range rows {
		acc[r.Title] += int(r.JobCount)
	}
}

// writeReport prints the top n candidates as a tab-separated count/title table,
// highest count first (dictgap.UnclassifiedTitles already sorts them). Count first so the output
// pipes into sort, cut and grep.
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
