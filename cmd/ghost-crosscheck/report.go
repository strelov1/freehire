package main

import (
	"log"
	"sort"

	"github.com/strelov1/freehire/internal/ghost"
)

// report accumulates what a run did, or would do. It exists because the rollout
// gate is a person reading this output and deciding whether the signal may go live:
// the known failure mode is staffing and consulting agencies, which legitimately
// advertise clients' roles absent from their own board, and a bare total cannot
// show whether they dominate. So the report breaks the verdict down BY COMPANY and
// prints examples.
type report struct {
	apply bool

	stamped   int
	cleared   int
	skipped   int
	postings  int
	companies int
	// coverage-gated companies: no board of ours to check against.
	gated int

	perCompany map[string]int
	samples    []string
}

func (r *report) add(slug string, postings []ghost.Posting, res ghost.CrosscheckResult) {
	if r.perCompany == nil {
		r.perCompany = map[string]int{}
	}
	r.companies++
	r.postings += len(postings)
	r.stamped += len(res.Stamp)
	r.cleared += len(res.Clear)
	r.skipped += res.Skipped
	// A company whose every posting was skipped is one we cannot check at all.
	// Counting it separately keeps "we found nothing wrong" distinguishable from
	// "we could not look", which a single skipped total would blur.
	if res.Skipped == len(postings) && len(postings) > 0 {
		r.gated++
		return
	}
	if len(res.Stamp) == 0 {
		return
	}
	r.perCompany[slug] += len(res.Stamp)

	stampedIDs := map[int64]struct{}{}
	for _, id := range res.Stamp {
		stampedIDs[id] = struct{}{}
	}
	for _, p := range postings {
		if _, ok := stampedIDs[p.ID]; ok && len(r.samples) < sampleSize {
			r.samples = append(r.samples, slug+" — "+p.Title)
		}
	}
}

// print writes the calibration report.
func (r *report) print() {
	mode := "DRY RUN (no writes; pass --apply to stamp)"
	if r.apply {
		mode = "APPLIED"
	}
	log.Printf("ghost-crosscheck %s: %d postings across %d companies", mode, r.postings, r.companies)
	log.Printf("  stamped absent: %d", r.stamped)
	log.Printf("  cleared:        %d", r.cleared)
	log.Printf("  skipped:        %d (of which %d companies had no board of ours to check against)",
		r.skipped, r.gated)

	if len(r.perCompany) == 0 {
		log.Print("  no company reached the absent verdict")
		return
	}

	type row struct {
		slug string
		n    int
	}
	rows := make([]row, 0, len(r.perCompany))
	for slug, n := range r.perCompany {
		rows = append(rows, row{slug, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].slug < rows[j].slug
	})

	// The top of this list IS the gate. A batch dominated by one company, or by
	// staffing agencies, is a broken signal rather than a discovery — the same read
	// cmd/prune asks of its own dry run.
	log.Printf("  top companies by stamped postings (read this before opening the gate):")
	for i, rw := range rows {
		if i == 20 {
			log.Printf("    … and %d more companies", len(rows)-20)
			break
		}
		log.Printf("    %6d  %s", rw.n, rw.slug)
	}
	log.Printf("  sample of stamped roles:")
	for _, s := range r.samples {
		log.Printf("    %s", s)
	}
}
