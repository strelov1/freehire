// Command report-skill-gaps is a hand-run, read-only report: it scans jobs.enrichment
// for the raw skill phrases LLM enrichment recorded, ranks the ones
// internal/dict/skilltag's alias table resolves to nothing, and prints the top-N by
// frequency. It writes nothing — no database row, no dictionary source file — and is
// not wired to any timer; a curator runs it, reads the report, and decides by hand
// what belongs in internal/dict/skilltag's dictionaries.go/labels.go/descriptions.tsv.
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

// defaultChunkSize is how many ids one chunk's id range spans. It reads only the
// enrichment column's `skills` key, not a wide description, so it can afford a wider
// span than cmd/backfill-requirements's 5k — sized the same as the other light-column
// chunked passes (cmd/backfill-slug-folded).
const defaultChunkSize = 50_000

// rowsPerChunk bounds how many (id, skill) pairs one statement returns: the LATERAL
// expansion means a dense id range can carry far more rows than ids, so this is what
// actually bounds one statement's memory, the same role it plays in
// cmd/backfill-requirements.
const rowsPerChunk = 2_000

// defaultTopN is how many ranked candidates the report prints.
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

	step, err := worker.EnvInt64("REPORT_SKILL_GAPS_CHUNK", defaultChunkSize)
	if err != nil {
		log.Printf("report-skill-gaps: %v", err)
		return 1
	}
	topN, err := worker.EnvInt64("REPORT_SKILL_GAPS_TOP", defaultTopN)
	if err != nil {
		log.Printf("report-skill-gaps: %v", err)
		return 1
	}
	resume, err := worker.EnvInt64("REPORT_SKILL_GAPS_FROM_ID", 0)
	if err != nil {
		log.Printf("report-skill-gaps: %v", err)
		return 1
	}

	q := db.New(pool)
	bounds, err := q.SkillGapReportBounds(ctx)
	if err != nil {
		log.Printf("report-skill-gaps: bounds: %v", err)
		return 1
	}
	if bounds.MaxID == 0 {
		log.Print("report-skill-gaps: nothing to do")
		return 0
	}

	from := bounds.MinID
	if resume > from {
		from = resume
	}
	log.Printf("report-skill-gaps: ids %d..%d from %d, chunk=%d", bounds.MinID, bounds.MaxID, from, step)

	counts := make(map[string]int)
	var examined int64
	lastLog := time.Now()
	for from <= bounds.MaxID {
		rows, err := q.ListJobSkillsForGapReport(ctx, db.ListJobSkillsForGapReportParams{
			FromID:   from,
			ToID:     from + step,
			RowLimit: rowsPerChunk,
		})
		if err != nil {
			log.Printf("report-skill-gaps: read chunk %d..%d after %d rows: %v", from, from+step, examined, err)
			return 1
		}
		examined += int64(len(rows))
		from = foldSkillCounts(counts, rows, rowsPerChunk, from+step)

		if time.Since(lastLog) >= time.Minute {
			log.Printf("report-skill-gaps: progress examined=%d distinct=%d at id=%d of %d",
				examined, len(counts), from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("report-skill-gaps: cancelled at id=%d after %d rows — resume with REPORT_SKILL_GAPS_FROM_ID=%d",
				from, examined, from)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}

	log.Printf("report-skill-gaps: done, examined=%d distinct phrases=%d", examined, len(counts))
	candidates := dictgap.SkillGapCandidates(counts)
	log.Printf("report-skill-gaps: %d unresolved candidates, printing top %d", len(candidates), topN)
	writeSkillGapReport(os.Stdout, candidates, int(topN))
	return 0
}

// foldSkillCounts tallies one chunk's (id, skill) rows into the running frequency
// map and returns where the next chunk should start: the last id seen plus one when
// the chunk came back at the row-limit ceiling (the LIMIT, not the id range, ended
// it), or the chunk's own upper bound (rangeEnd) otherwise.
func foldSkillCounts(counts map[string]int, rows []db.ListJobSkillsForGapReportRow, limit int32, rangeEnd int64) int64 {
	for _, r := range rows {
		counts[r.Skill]++
	}
	if len(rows) == int(limit) && len(rows) > 0 {
		return rows[len(rows)-1].ID + 1
	}
	return rangeEnd
}

// writeSkillGapReport prints the top n candidates as a tab-separated count/phrase
// table, one per line, ranked highest count first (candidates is assumed already
// sorted, as dictgap.SkillGapCandidates returns it).
func writeSkillGapReport(w io.Writer, candidates []dictgap.SkillGapCandidate, n int) {
	if n < len(candidates) {
		candidates = candidates[:n]
	}
	for _, c := range candidates {
		fmt.Fprintf(w, "%d\t%s\n", c.Count, c.Phrase)
	}
}
