// Command report-geo-gaps is a hand-run, read-only report: it recomputes
// internal/dict/location's country answer for every distinct location string among
// publishable postings and compares it with the countries LLM enrichment recorded,
// ranking where the two disagree by frequency. It writes nothing — no database row,
// no dictionary source file — and is not wired to any timer; a curator runs it,
// reads the report, and decides by hand what belongs in internal/dict/location's
// name tables.
//
// Two sections, because they call for opposite fixes: GAPS are locations the
// dictionary places nowhere (a missing name), CONFLICTS are locations it places
// somewhere the model mostly disagrees with (a name it may be resolving wrongly).
// The share column is the curator's confidence signal — the model is asked about
// "Remote"-only postings too, and answers those with a guess.
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/dictgap"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// defaultChunkSize is how many ids one chunk's id range spans. The query GROUPs BY with no row
// LIMIT and reads no description, so the range width alone bounds one statement's cost.
const defaultChunkSize = 50_000

// defaultTopN is how many ranked candidates each of the two sections prints.
const defaultTopN = 200

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

	step, err := worker.EnvInt64("REPORT_GEO_GAPS_CHUNK", defaultChunkSize)
	if err != nil {
		log.Printf("report-geo-gaps: %v", err)
		return 1
	}
	topN, err := worker.EnvInt32("REPORT_GEO_GAPS_TOP", defaultTopN)
	if err != nil {
		log.Printf("report-geo-gaps: %v", err)
		return 1
	}
	resume, err := worker.EnvInt64("REPORT_GEO_GAPS_FROM_ID", 0)
	if err != nil {
		log.Printf("report-geo-gaps: %v", err)
		return 1
	}

	q := db.New(pool)
	bounds, err := q.GeoGapReportBounds(ctx)
	if err != nil {
		log.Printf("report-geo-gaps: bounds: %v", err)
		return 1
	}
	if bounds.MaxID == 0 {
		log.Print("report-geo-gaps: nothing to do")
		return 0
	}

	from := bounds.MinID
	if resume > from {
		from = resume
	}
	log.Printf("report-geo-gaps: ids %d..%d from %d, chunk=%d", bounds.MinID, bounds.MaxID, from, step)

	var groups []dictgap.LocationCountries
	lastLog := time.Now()
	for from <= bounds.MaxID {
		to := from + step
		rows, err := q.ListLocationsForGeoGap(ctx, db.ListLocationsForGeoGapParams{FromID: from, ToID: to})
		if err != nil {
			log.Printf("report-geo-gaps: read chunk %d..%d after %d groups: %v", from, to, len(groups), err)
			return 1
		}
		for _, r := range rows {
			groups = append(groups, dictgap.LocationCountries{
				Location:     r.Location,
				LLMCountries: r.LlmCountries,
				Count:        int(r.JobCount),
			})
		}
		from = to

		if time.Since(lastLog) >= time.Minute {
			log.Printf("report-geo-gaps: progress groups=%d at id=%d of %d", len(groups), from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("report-geo-gaps: cancelled at id=%d after %d groups — resume with REPORT_GEO_GAPS_FROM_ID=%d",
				from, len(groups), from)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}

	report := dictgap.GeoGapCandidates(groups)
	log.Printf("report-geo-gaps: done, groups=%d gaps=%d conflicts=%d, printing top %d each",
		len(groups), len(report.Gaps), len(report.Conflicts), topN)
	if err := writeGeoGapReport(os.Stdout, report, int(topN)); err != nil {
		log.Printf("report-geo-gaps: write report: %v", err)
		return 1
	}
	return 0
}

// writeGeoGapReport prints the top n of each section as a tab-separated table,
// gaps first: count, the model's share for its top country, that country, the
// dictionary's countries (conflicts only) and the location string.
func writeGeoGapReport(w io.Writer, report dictgap.GeoGapReport, n int) error {
	if _, err := fmt.Fprintln(w, "# gaps\tcount\tshare\tllm\tlocation"); err != nil {
		return err
	}
	for _, c := range head(report.Gaps, n) {
		if _, err := fmt.Fprintf(w, "%d\t%.0f%%\t%s\t%s\n", c.Count, c.LLMShare*100, c.LLMCountry, c.Location); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# conflicts\tcount\tshare\tllm\tdictionary\tlocation"); err != nil {
		return err
	}
	for _, c := range head(report.Conflicts, n) {
		if _, err := fmt.Fprintf(w, "%d\t%.0f%%\t%s\t%s\t%s\n",
			c.Count, c.LLMShare*100, c.LLMCountry, strings.Join(c.DictionaryCountries, ","), c.Location); err != nil {
			return err
		}
	}
	return nil
}

func head(c []dictgap.GeoGapCandidate, n int) []dictgap.GeoGapCandidate {
	if n < len(c) {
		return c[:n]
	}
	return c
}
