package main

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// A batch dominated by one board is the signature of a broken board title rather than a
// real cluster, and it is only visible next to the other sources. The report is the
// only thing standing between that and an irreversible run.
func TestPlanReportShowsBreakdownAndSample(t *testing.T) {
	p := newPlan(20, rand.New(rand.NewPCG(1, 0)))
	p.add(db.PruneCandidatesRow{ID: 1, Title: "Line Cook", Source: "greenhouse"}, ruleTitle)
	p.add(db.PruneCandidatesRow{ID: 2, Title: "Registered Nurse", Source: "ukg"}, ruleTitle)
	p.add(db.PruneCandidatesRow{ID: 3, Title: "Account Manager", Source: "ukg"}, ruleBusiness)
	p.refuse("board still listed")

	var b strings.Builder
	if err := p.report(&b, false); err != nil {
		t.Fatalf("report: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"would delete 3 of 3", "BY RULE", ruleTitle, ruleBusiness,
		"BY SOURCE", "greenhouse", "ukg", "REFUSED", "board still listed",
		"SAMPLE", "Line Cook",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

// The cap bounds what a run removes, but the operator still has to see how much was
// matched — otherwise a capped run reads as a finished one.
func TestPlanReportDistinguishesMatchedFromDeleted(t *testing.T) {
	p := newPlan(20, rand.New(rand.NewPCG(1, 0)))
	for i := range 5 {
		p.add(db.PruneCandidatesRow{ID: int64(i), Title: "Line Cook", Source: "greenhouse"}, ruleTitle)
	}
	p.targets = p.targets[:2] // the cap trimmed the run
	p.deleted = 3             // and the delete dragged a duplicate along

	var b strings.Builder
	if err := p.report(&b, true); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(b.String(), "deleted 3 rows from 2 targets, of 5 matching") {
		t.Errorf("rows, targets and matches are three different numbers and all must show:\n%s", b.String())
	}
}

// An empty plan must still render — a run that found nothing is the campaign's goal,
// not a failure to report.
func TestPlanReportOnEmptyPlan(t *testing.T) {
	var b strings.Builder
	if err := newPlan(20, rand.New(rand.NewPCG(1, 0))).report(&b, false); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(b.String(), "would delete 0 of 0") {
		t.Errorf("empty plan must say so:\n%s", b.String())
	}
}

// The sample must be uniform over what matched, not the first N by id. Ids are
// chronological, so a first-N window shows the oldest rows of whichever board was
// ingested first — and keeps showing them while the dictionary changes underneath.
func TestPlanSampleIsUniformNotFirstN(t *testing.T) {
	p := newPlan(10, rand.New(rand.NewPCG(7, 0)))
	for i := range 1000 {
		p.add(db.PruneCandidatesRow{ID: int64(i), Title: "T" + strconv.Itoa(i), Source: "greenhouse"}, ruleTitle)
	}
	if len(p.samples) != 10 {
		t.Fatalf("samples = %d, want 10", len(p.samples))
	}
	late := 0
	for _, s := range p.samples {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(strings.Fields(s)[1], "T"), ""))
		if err != nil {
			t.Fatalf("unexpected sample %q", s)
		}
		if n >= 500 {
			late++
		}
	}
	if late == 0 {
		t.Errorf("no sample came from the second half of 1000 matches — the window is biased: %v", p.samples)
	}
}
