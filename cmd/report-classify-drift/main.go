// Command report-classify-drift is a hand-run, read-only report: it recomputes
// internal/dict/classify's seniority and category for every distinct enriched job
// title and ranks the titles where that dictionary answer disagrees with what LLM
// enrichment recorded, by frequency. It writes nothing — no database row, no
// dictionary source file — and is not wired to any timer; a curator runs it, reads
// the report, and decides by hand what belongs in internal/dict/classify's title
// tables (e.g. gradeBlindPhrases, the category ordering).
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

// defaultChunkSize is how many ids one chunk's id range spans. This query GROUPs BY
// title with no row LIMIT (see ListTitlesForClassifyDrift's comment) — the range
// width alone bounds one statement's cost — so it stays as wide as the other
// light-column chunked passes (cmd/backfill-slug-folded).
const defaultChunkSize = 50_000

// defaultTopN is how many ranked candidates each of the two report sections prints.
const defaultTopN = 200

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

	step, err := worker.EnvInt64("REPORT_CLASSIFY_DRIFT_CHUNK", defaultChunkSize)
	if err != nil {
		log.Printf("report-classify-drift: %v", err)
		return 1
	}
	topN, err := worker.EnvInt32("REPORT_CLASSIFY_DRIFT_TOP", defaultTopN)
	if err != nil {
		log.Printf("report-classify-drift: %v", err)
		return 1
	}
	resume, err := worker.EnvInt64("REPORT_CLASSIFY_DRIFT_FROM_ID", 0)
	if err != nil {
		log.Printf("report-classify-drift: %v", err)
		return 1
	}

	q := db.New(pool)
	bounds, err := q.ClassifyDriftReportBounds(ctx)
	if err != nil {
		log.Printf("report-classify-drift: bounds: %v", err)
		return 1
	}
	if bounds.MaxID == 0 {
		log.Print("report-classify-drift: nothing to do")
		return 0
	}

	from := bounds.MinID
	if resume > from {
		from = resume
	}
	log.Printf("report-classify-drift: ids %d..%d from %d, chunk=%d", bounds.MinID, bounds.MaxID, from, step)

	titles := make(map[string]*dictgap.TitleClassification)
	var examined int64
	lastLog := time.Now()
	for from <= bounds.MaxID {
		to := from + step
		rows, err := q.ListTitlesForClassifyDrift(ctx, db.ListTitlesForClassifyDriftParams{
			FromID: from,
			ToID:   to,
		})
		if err != nil {
			log.Printf("report-classify-drift: read chunk %d..%d after %d titles: %v", from, to, examined, err)
			return 1
		}
		examined += int64(len(rows))
		mergeTitleClassifications(titles, rows)
		from = to

		if time.Since(lastLog) >= time.Minute {
			log.Printf("report-classify-drift: progress chunks-seen=%d distinct-titles=%d at id=%d of %d",
				examined, len(titles), from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("report-classify-drift: cancelled at id=%d after %d rows — resume with REPORT_CLASSIFY_DRIFT_FROM_ID=%d",
				from, examined, from)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}

	log.Printf("report-classify-drift: done, distinct titles=%d", len(titles))
	rows := make([]dictgap.TitleClassification, 0, len(titles))
	for _, t := range titles {
		rows = append(rows, *t)
	}
	report := dictgap.ClassifyDriftCandidates(rows)
	log.Printf("report-classify-drift: %d seniority candidates, %d category candidates, printing top %d each",
		len(report.Seniority), len(report.Category), topN)
	if err := writeDriftReport(os.Stdout, report, int(topN)); err != nil {
		log.Printf("report-classify-drift: write report: %v", err)
		return 1
	}
	return 0
}

// mergeTitleClassifications folds one chunk's per-title rows into the running
// accumulator, keyed by title. A title already present has its count summed across
// chunks — the same title can appear in more than one id range, since grouping
// happens per chunk, not across the whole table — and keeps whichever
// enrichment seniority/category pair it already has: both chunks describe the same
// title, so the first one seen is as representative as any other.
func mergeTitleClassifications(acc map[string]*dictgap.TitleClassification, rows []db.ListTitlesForClassifyDriftRow) {
	for _, r := range rows {
		if existing, ok := acc[r.Title]; ok {
			existing.Count += int(r.JobCount)
			continue
		}
		acc[r.Title] = &dictgap.TitleClassification{
			Title:               r.Title,
			Count:               int(r.JobCount),
			EnrichmentSeniority: r.EnrichmentSeniority,
			EnrichmentCategory:  r.EnrichmentCategory,
		}
	}
}

// writeDriftReport prints the top n candidates of each facet as a tab-separated
// count/title/dictionary_value/enrichment_value table, seniority first then
// category, each ranked highest count first (as dictgap.ClassifyDriftCandidates
// already sorts them).
func writeDriftReport(w io.Writer, report dictgap.DriftReport, n int) error {
	if _, err := fmt.Fprintln(w, "# seniority"); err != nil {
		return err
	}
	if err := writeDriftCandidates(w, report.Seniority, n); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# category"); err != nil {
		return err
	}
	return writeDriftCandidates(w, report.Category, n)
}

func writeDriftCandidates(w io.Writer, candidates []dictgap.DriftCandidate, n int) error {
	if n < len(candidates) {
		candidates = candidates[:n]
	}
	for _, c := range candidates {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", c.Count, c.Title, c.DictionaryValue, c.EnrichmentValue); err != nil {
			return err
		}
	}
	return nil
}
