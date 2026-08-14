package pipeline

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/sources"
)

// A crawled board pours a company's whole hiring into the catalogue, so the postings
// the non-tech dictionary recognises are rejected before the write path. Rejection is
// safe here precisely because the board is re-crawled: if a dictionary term turns out
// to be too broad, removing it re-admits the postings on the next pass.
func TestRunRejectsNonTechPostings(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Registered Nurse", Company: "Acme"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().Title != "Backend Engineer" {
		t.Fatalf("saved = %+v, want only the technical posting", store.saved)
	}
	if got := stats.Total(); got.Ingested != 1 || got.Rejected != 1 {
		t.Errorf("stats = %+v, want Ingested=1 Rejected=1", got)
	}
}

// A liveness refresh is subject to the same filter as a write. Catalogue pruning depends on it:
// once the dictionary recognises a board's non-technical postings, the stored ones must stop
// being seen so the unseen sweep closes them. A hydrating adapter re-lists them every crawl, so
// refreshing without the filter would keep exactly the rows the campaign is retiring alive
// forever — on one Workday board that is 25k of its 25.6k stored postings.
func TestRunDoesNotRefreshARejectedPosting(t *testing.T) {
	src := &fakeHydratingSource{provider: "workday", jobs: []sources.Job{
		{ExternalID: "1", Title: "Registered Nurse", Company: "Acme", URL: "u"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme", URL: "u"},
	}}
	// Both are already ingested, so both come back as liveness refreshes. Neither row carries
	// tech evidence, so the dictionary alone decides.
	store := &fakeStore{seenByBrd: map[string]map[string]bool{
		"acme": {"acme:1": false, "acme:2": false},
	}}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "workday", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.touched) != 1 || store.touched[0] != [2]string{"workday", "acme:2"} {
		t.Errorf("touched = %v, want only the technical posting (workday, acme:2)", store.touched)
	}
	if got := stats.Total(); got.Rejected != 1 || got.Skipped != 0 {
		t.Errorf("stats = %+v, want Rejected=1 Skipped=0 — a rejection is not a malfunction", got)
	}
}

// The refresh filter reads the tech evidence STORED with the row, not what the listing alone
// implies. A hydrating crawl carries no description, and the dictionary matches its terms
// anywhere in a title — measured against prod, 1,601 of 96,218 titles the catalogue holds as
// technical (1.7%) are flagged when the title is judged alone. Judging a refresh on the listing
// would close them: real hardware and electrical engineering roles whose evidence lives in the
// description. So the row's own is_tech overrides the dictionary here exactly as it does on the
// write path.
func TestRunRefreshesAFlaggedTitleThatTheRowProvesTechnical(t *testing.T) {
	src := &fakeHydratingSource{provider: "workday", jobs: []sources.Job{
		{ExternalID: "1", Title: "Associate Hardware Mechanical Engineer", Company: "Acme", URL: "u"},
	}}
	store := &fakeStore{seenByBrd: map[string]map[string]bool{
		"acme": {"acme:1": true}, // stored as is_tech = true, evidence the listing cannot show
	}}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "workday", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.touched) != 1 || store.touched[0] != [2]string{"workday", "acme:1"} {
		t.Errorf("touched = %v, want the posting refreshed — its row proves it technical", store.touched)
	}
	if got := stats.Total(); got.Rejected != 0 {
		t.Errorf("stats = %+v, want Rejected=0", got)
	}
}

// The ingest filter reads the non-tech TITLE dictionary, not the tri-state is_tech.
// A business role at an IT company resolves is_tech=false through its category, and
// whether it belongs in the catalogue depends on the company — a judgement the crawl
// cannot make and the prune worker makes later. Rejecting it here would quietly
// delete every sales and recruiting job on the board.
func TestRunKeepsBusinessRolesTheCompanyRuleOwns(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Sales Manager", Company: "Acme"},
		{ExternalID: "2", Title: "Technical Recruiter", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 2 {
		t.Fatalf("len(saved) = %d, want 2 — a non-technical category is not a title-dictionary match", len(store.saved))
	}
	if got := stats.Total(); got.Rejected != 0 {
		t.Errorf("stats = %+v, want Rejected=0", got)
	}
}

// A streaming board goes through a separate save loop, so the filter has to be wired
// twice; a rejection reaching only one path would leak on half the catalogue.
func TestRunRejectsNonTechInStreamPath(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Line Cook", Company: "Acme"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "jobtech", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().Title != "Backend Engineer" {
		t.Fatalf("saved = %+v, want only the technical posting", store.saved)
	}
	if got := stats.Total(); got.Ingested != 1 || got.Rejected != 1 {
		t.Errorf("stats = %+v, want Ingested=1 Rejected=1", got)
	}
}

// Rejections and skips mean opposite things to an operator: a rejection is the filter
// working, a skip is something broken. Folding them together would make a board whose
// every save fails look like a board full of non-technical postings.
func TestRunCountsRejectionsSeparatelyFromSkips(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Registered Nurse", Company: "Acme"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{err: errors.New("write failed")}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := stats.Total()
	if got.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1 (the nurse posting)", got.Rejected)
	}
	if got.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (the engineer posting failed to save)", got.Skipped)
	}
}

// The non-tech dictionary matches anywhere in a title and was written on the contract
// that tech evidence is checked first. Used as a drop gate without that veto it rejects
// postings the pipeline itself derived as technical — every title here carries
// is_tech=true and also matches a dictionary term (hvac, teller, cook, patient care).
// Labelling them non-tech was harmless because a label is reversible; dropping them is
// not, and a rejected posting leaves no record anywhere.
func TestRunKeepsTechnicalTitlesThatMatchANonTechTerm(t *testing.T) {
	titles := []string{
		"DevOps Engineer (HVAC IoT Platform)",
		"Backend Engineer — Teller Systems",
		"Data Engineer, Patient Care Platform",
		"Senior Software Engineer - Cook County Health",
	}
	var jobs []sources.Job
	for i, title := range titles {
		jobs = append(jobs, sources.Job{ExternalID: string(rune('1' + i)), Title: title, Company: "Acme"})
	}
	store := &fakeStore{}
	r := Runner{Registry: registry(fakeSource{provider: "greenhouse", jobs: jobs}), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats.Total(); got.Rejected != 0 || got.Ingested != len(titles) {
		t.Errorf("stats = %+v, want Ingested=%d Rejected=0 — technical evidence vetoes the dictionary", got, len(titles))
	}
}

// A streaming board that emitted postings before failing made progress even if the
// filter turned every one of them away. Counting that as a board failure would cool a
// healthy board for hours after three occurrences — and an all-rejected partial crawl
// is the normal case on the non-tech-heavy national feeds, not the edge case.
func TestRunTreatsAllRejectedStreamProgressAsSuccess(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: 2, jobs: []sources.Job{
		{ExternalID: "1", Title: "Line Cook", Company: "Acme"},
		{ExternalID: "2", Title: "Registered Nurse", Company: "Acme"},
		{ExternalID: "3", Title: "Backend Engineer", Company: "Acme"},
	}}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "jobtech", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats.Total(); got.Rejected != 2 {
		t.Fatalf("stats = %+v, want Rejected=2", got)
	}
	if len(health.failures) != 0 {
		t.Errorf("board recorded as failed (%v) — rejected postings are progress, not an outage", health.failures)
	}
}

// A streaming aggregator board that emitted postings before failing made progress even if
// every one of them was skipped as already covered by a non-aggregator source — the exact
// same reasoning TestRunTreatsAllRejectedStreamProgressAsSuccess proves for a rejected
// prefix. Missing ATSCovered from the "no progress" check would cool a board down for hours
// precisely when the coverage gate is doing its job well (heavily-covered board), so this
// case gets more likely to fire, not less, as the feature succeeds.
func TestRunTreatsAllATSCoveredStreamProgressAsSuccess(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: 2, jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme"},
		{ExternalID: "3", Title: "Data Engineer", Company: "Acme"},
	}}
	health := &fakeHealth{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "jobtech", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats.Total(); got.ATSCovered != 2 {
		t.Fatalf("stats = %+v, want ATSCovered=2", got)
	}
	if len(health.failures) != 0 {
		t.Errorf("board recorded as failed (%v) — ATSCovered postings are progress, not an outage", health.failures)
	}
}

// The per-board line is the only operator-facing signal that a dictionary term is too
// broad, and it has to arrive within the crawl hour. Counts alone say how much was
// dropped but not what, so it carries sample titles too.
func TestRejectionLogCarriesShareAndSamples(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Registered Nurse", Company: "Acme"},
		{ExternalID: "2", Title: "Line Cook", Company: "Acme"},
		{ExternalID: "3", Title: "Backend Engineer", Company: "Acme"},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := buf.String()
	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "rejected") {
			if line != "" {
				t.Fatalf("more than one rejection line — it must be once per board, not per posting:\n%s", out)
			}
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no rejection line logged:\n%s", out)
	}
	for _, want := range []string{"2/3", "67%", "Registered Nurse", "Line Cook"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line missing %q: %s", want, line)
		}
	}
}

// A board with nothing to reject must stay silent, or the signal drowns in noise across
// twelve hundred boards.
func TestRejectionLogSilentWhenNothingRejected(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(buf.String(), "rejected") {
		t.Errorf("logged a rejection line with nothing rejected: %s", buf.String())
	}
}
