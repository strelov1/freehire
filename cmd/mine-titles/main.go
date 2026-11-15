// Command mine-titles reports the word groups occurring most often in the titles of
// jobs that still carry no is_tech signal. It names the next cluster worth a
// dictionary term, and, run again after the dictionary grows, shows whether the
// unclassified group shrank.
//
// It is a run-once-and-exit worker and writes nothing. Grouping is by word group, not whole
// title, because boards append location, schedule and requisition detail; a word group is also
// the unit the non-tech dictionary accepts.
//
// --limit bounds the output, not the work: the whole group-by runs before the top-N sort, and
// overlapping groups multiply the rows, so expect minutes and temp sort space. Operator tool
// only, never on a request path.
//
// The report is a shortlist for a human, not a verdict: it includes technical phrases that
// must NOT reach the non-tech dictionary ("systems engineer", "team lead") and fragments of
// one verbose employer's titles.
//
// Usage: go run ./cmd/mine-titles [--limit=100]   (needs DATABASE_URL)
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	limit := flag.Int("limit", 100, "how many title clusters to report, busiest first")
	flag.Parse()
	if *limit <= 0 || *limit > math.MaxInt32 {
		log.Printf("--limit must be between 1 and %d, got %d", math.MaxInt32, *limit)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	rows, err := db.New(pool).ResidualTitleGroups(ctx, db.ResidualTitleGroupsParams{
		StopWords:  stopWords,
		Connectors: connectors,
		RowLimit:   int32(*limit),
	})
	if err != nil {
		log.Printf("mine-titles: %v", err)
		return 1
	}
	if err := report(os.Stdout, rows); err != nil {
		log.Printf("mine-titles: write report: %v", err)
		return 1
	}
	return 0
}

// shownSources is how many source names a row prints before the rest are counted off.
const shownSources = 3

// report writes the clusters as an aligned table in the order given (busiest first): job
// count, source breadth, the group, then a sample of sources. Breadth tells a catalogue-wide
// role from one employer's shredded titles. Sources are sorted here so an unchanged catalogue
// renders byte-identically between runs.
func report(w io.Writer, rows []db.ResidualTitleGroupsRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no unclassified title groups left")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "JOBS\tSRCS\tGROUP\tSOURCES"); err != nil {
		return err
	}
	for _, r := range rows {
		sources := slices.Clone(r.Sources)
		slices.Sort(sources)
		sample := strings.Join(sources[:min(len(sources), shownSources)], ", ")
		if n := len(sources) - shownSources; n > 0 {
			sample += fmt.Sprintf(" +%d", n)
		}
		if _, err := fmt.Fprintf(tw, "%d\t%d\t%s\t%s\n", r.Jobs, len(sources), r.Grp, sample); err != nil {
			return err
		}
	}
	return tw.Flush()
}
