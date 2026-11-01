package main

import (
	"log"
	"math/rand/v2"
	"sort"

	"github.com/strelov1/freehire/internal/job/ghost"
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
	skips     ghost.Skips
	postings  int
	companies int
	// coverage-gated companies: no board of ours to check against.
	gated int

	perCompany map[string]int
	perSource  map[string]int
	samples    []string
	sampled    int // stamped postings offered to the sample so far
}

func (r *report) add(source, slug string, postings []ghost.Posting, res ghost.CrosscheckResult) {
	if r.perCompany == nil {
		r.perCompany = map[string]int{}
		r.perSource = map[string]int{}
	}
	r.companies++
	r.postings += len(postings)
	r.stamped += len(res.Stamp)
	r.cleared += len(res.Clear)
	r.skips.Add(res.Skips)
	// A company we have no board for is one we cannot check at all. Counting it
	// separately keeps "we found nothing wrong" distinguishable from "we could not
	// look", which a single skipped total would blur.
	if res.Skips.NoBoard > 0 {
		r.gated++
	}
	if len(res.Stamp) == 0 {
		return
	}
	r.perCompany[slug] += len(res.Stamp)
	r.perSource[source] += len(res.Stamp)

	stampedIDs := map[int64]struct{}{}
	for _, id := range res.Stamp {
		stampedIDs[id] = struct{}{}
	}
	// A reservoir sample, so the examples stand for the whole run rather than for
	// the first sources it happened to walk.
	for _, p := range postings {
		if _, ok := stampedIDs[p.ID]; !ok {
			continue
		}
		r.sampled++
		line := slug + " [" + source + "] — " + p.Title
		if len(r.samples) < sampleSize {
			r.samples = append(r.samples, line)
		} else if i := rand.IntN(r.sampled); i < sampleSize {
			r.samples[i] = line
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
	log.Printf("  skipped:        %d", r.skips.Total())
	log.Printf("    no company:    %d", r.skips.NoCompany)
	log.Printf("    via recruiter: %d", r.skips.ViaRecruiter)
	log.Printf("    recruiter company (states it posts for clients elsewhere): %d", r.skips.RecruiterCompany)
	log.Printf("    no board:      %d (%d companies we crawl no board of)", r.skips.NoBoard, r.gated)
	log.Printf("    no title key:  %d", r.skips.NoTitle)
	log.Printf("    uncorroborated (board carries none of the batch): %d", r.skips.Uncorroborated)
	log.Printf("    mostly absent (absence is the batch's rule):      %d", r.skips.MostlyAbsent)

	if len(r.perCompany) == 0 {
		log.Print("  no company reached the absent verdict")
		return
	}

	// The top of these lists IS the gate. A batch dominated by one company, by
	// staffing agencies, or by one source is a broken signal rather than a discovery.
	logTop("top companies by stamped postings (read this before opening the gate)", r.perCompany, 20)
	logTop("top sources by stamped postings", r.perSource, 10)
	log.Printf("  random sample of stamped roles:")
	for _, s := range r.samples {
		log.Printf("    %s", s)
	}
}

// logTop prints the n largest counts of m, largest first.
func logTop(heading string, m map[string]int, n int) {
	type row struct {
		name string
		n    int
	}
	rows := make([]row, 0, len(m))
	for name, c := range m {
		rows = append(rows, row{name, c})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].name < rows[j].name
	})
	log.Printf("  %s:", heading)
	for i, rw := range rows {
		if i == n {
			log.Printf("    … and %d more", len(rows)-n)
			break
		}
		log.Printf("    %6d  %s", rw.n, rw.name)
	}
}
