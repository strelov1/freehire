package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"sort"
	"text/tabwriter"

	"github.com/strelov1/freehire/internal/db"
)

// target is one row the rule matched, and the rule that matched it.
type target struct {
	id   int64
	rule string
}

// plan is what a scan found: the rows to delete, why, and everything an operator needs
// to decide whether the run is sane before any of it happens.
type plan struct {
	targets  []target
	byRule   map[string]int
	bySource map[string]int
	samples  []string
	// refused counts rows a company-scoped rule matched but the guard turned down,
	// per reason. They are not deletions and not errors: they are the guard working.
	refused map[string]int
	// matched is every row the rule matched, including those past the cap, so the
	// report can say how much of the work this run is doing.
	matched int
	// deleted is what the delete statement actually removed, which exceeds the number
	// of targets: each one drags its duplicate chain along.
	deleted    int
	sampleSize int
	rnd        *rand.Rand
}

func newPlan(sampleSize int, rnd *rand.Rand) *plan {
	return &plan{
		byRule: map[string]int{}, bySource: map[string]int{}, refused: map[string]int{},
		sampleSize: sampleSize, rnd: rnd,
	}
}

func (p *plan) add(row db.PruneCandidatesRow, rule string) {
	p.matched++
	p.targets = append(p.targets, target{id: row.ID, rule: rule})
	p.byRule[rule]++
	p.bySource[row.Source]++
	p.sample(fmt.Sprintf("[%s] %s — %s", rule, row.Title, row.Source))
}

// sample keeps a uniformly random selection of the matched titles, by reservoir.
//
// Taking the first N instead would be worse than useless here. Ids are chronological
// and clustered by ingest batch, so the first N matches are the oldest rows of whichever
// board happened to be ingested first — systematically unrepresentative, and stable
// across iterations, so it would keep showing the same rows while the dictionary changed
// underneath. The sample's job is to catch an over-broad term before an irreversible
// run; a biased window cannot do it.
func (p *plan) sample(line string) {
	if len(p.samples) < p.sampleSize {
		p.samples = append(p.samples, line)
		return
	}
	if k := p.rnd.IntN(p.matched); k < p.sampleSize {
		p.samples[k] = line
	}
}

func (p *plan) refuse(reason string) { p.refused[reason]++ }

// report prints what the run would do, or did. The breakdown by source is the tripwire
// the design asks for: a batch dominated by one board is the signature of a broken
// board title, not a real cluster, and it is only visible next to the other sources.
func (p *plan) report(w io.Writer, applied bool) error {
	if applied {
		// Targets and rows differ: PruneJobs extends every batch to its duplicate
		// chain, so reporting only the target count would understate what was removed.
		if _, err := fmt.Fprintf(w, "deleted %d rows from %d targets, of %d matching\n",
			p.deleted, len(p.targets), p.matched); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "would delete %d of %d matching rows\n", len(p.targets), p.matched); err != nil {
		return err
	}
	if err := section(w, "BY RULE", p.byRule); err != nil {
		return err
	}
	if err := section(w, "BY SOURCE", p.bySource); err != nil {
		return err
	}
	if len(p.refused) > 0 {
		if err := section(w, "REFUSED (guard)", p.refused); err != nil {
			return err
		}
	}
	if len(p.samples) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nSAMPLE"); err != nil {
		return err
	}
	for _, s := range p.samples {
		if _, err := fmt.Fprintf(w, "  %s\n", s); err != nil {
			return err
		}
	}
	return nil
}

func section(w io.Writer, title string, counts map[string]int) error {
	if len(counts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})

	if _, err := fmt.Fprintf(w, "\n%s\n", title); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, k := range keys {
		if _, err := fmt.Fprintf(tw, "  %s\t%d\n", k, counts[k]); err != nil {
			return err
		}
	}
	return tw.Flush()
}
