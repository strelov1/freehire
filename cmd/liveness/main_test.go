package main

import (
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestMatchingProvidersMatchesBareAndDashedMembers(t *testing.T) {
	providers := []string{"whatjobs", "whatjobs-de", "whatjobs-uk", "himalayas", "whatjobsomething"}
	got := matchingProviders(providers, []string{"whatjobs"})
	want := []string{"whatjobs", "whatjobs-de", "whatjobs-uk"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("matchingProviders() = %v, want %v", got, want)
	}
}

// The two candidate queries return row types that differ by one field, and ExternalID is
// that field: it is what echojobs' evidence path is keyed on, since this source's stored
// jobs.url is the employer's own ATS link rather than a page echojobs.io serves. A
// conversion that dropped it would probe echojobs.io/job/ for every candidate and read a
// 404 — "this posting is gone" — for a catalogue that is entirely still live.
func TestStaleCandidateCarriesEveryFieldAnEvidencePathReads(t *testing.T) {
	got := staleCandidate(db.SelectStaleRegisteredCandidatesRow{
		ID: 42, PublicSlug: "acme-go-dev-42", Source: "echojobs",
		URL: "https://boards.greenhouse.io/acme/jobs/1", ExternalID: ":acme-go-dev",
		LivenessStrikes: 1,
	})
	want := candidate{
		id: 42, publicSlug: "acme-go-dev-42", source: "echojobs",
		url: "https://boards.greenhouse.io/acme/jobs/1", externalID: ":acme-go-dev", strikes: 1,
	}
	if got != want {
		t.Errorf("staleCandidate() = %+v, want %+v", got, want)
	}
}

// An orphan row has no external id to carry — its evidence is its own URL — but everything
// the verdict is written against (the id it updates, the slug it logs, the strike count that
// decides whether a reset is even issued) must survive the conversion.
func TestOrphanCandidatesCarryEveryFieldAVerdictIsWrittenAgainst(t *testing.T) {
	got := orphanCandidates([]db.SelectOrphanLivenessCandidatesRow{
		{ID: 7, PublicSlug: "one-7", Source: "telegram", URL: "https://t.me/jobs/7", LivenessStrikes: 0},
		{ID: 8, PublicSlug: "two-8", Source: "manual", URL: "https://acme.test/jobs/8", LivenessStrikes: 1},
	})
	want := []candidate{
		{id: 7, publicSlug: "one-7", source: "telegram", url: "https://t.me/jobs/7"},
		{id: 8, publicSlug: "two-8", source: "manual", url: "https://acme.test/jobs/8", strikes: 1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("orphanCandidates() = %+v, want %+v", got, want)
	}
}

// The age rule may only close a posting the crawl has ALSO stopped listing, and how long
// "stopped listing" takes is the provider's own sweep window — not a number restated here.
// whatjobs declares 14 days because a posting that drifts past its crawl budget reads as
// unseen for days at a time; on the 48-hour default the age rule would close postings the
// next crawl immediately reopens, which is the flap it was closing them into.
func TestUnseenWindowTakesTheProvidersOwnSweepWindow(t *testing.T) {
	declared := map[string]time.Duration{
		"whatjobs":    14 * 24 * time.Hour,
		"whatjobs-de": 14 * 24 * time.Hour,
		"gem":         24 * time.Hour, // narrower than the default, and not in this list
	}
	got := unseenWindow([]string{"whatjobs", "whatjobs-de"}, declared)
	if want := 14 * 24 * time.Hour; got != want {
		t.Errorf("unseenWindow() = %v, want %v — the default would close postings the crawl still lists", got, want)
	}
}

// A provider that declares nothing is swept on the default, and a provider that declares
// something NARROWER than the default does not drag the guess below it: this close has no
// evidence to appeal to, so it takes the most patient window in play.
func TestUnseenWindowNeverNarrowsBelowTheDefault(t *testing.T) {
	declared := map[string]time.Duration{"impatient": 24 * time.Hour}
	for _, srcs := range [][]string{{"undeclared"}, {"impatient"}, {"impatient", "undeclared"}} {
		if got := unseenWindow(srcs, declared); got != sources.DefaultSweepGrace {
			t.Errorf("unseenWindow(%v) = %v, want the %v default", srcs, got, sources.DefaultSweepGrace)
		}
	}
}

func TestMatchingProvidersEmptyOnNoMatch(t *testing.T) {
	got := matchingProviders([]string{"himalayas", "jobicy"}, []string{"whatjobs"})
	if len(got) != 0 {
		t.Fatalf("matchingProviders() = %v, want empty", got)
	}
}
