package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeAsk records what the adapter asked the database and replays a canned answer, so the
// fold-and-credit mapping either side of the query is testable without Postgres. The query
// itself is covered by cmd/ingest's integration test and internal/platform/db's.
type fakeAsk struct {
	answer     []string
	err        error
	askedSlugs [][]string
	askedAfter time.Time
}

func (f *fakeAsk) ask(_ context.Context, folded, _ []string, seenAfter time.Time) ([]string, error) {
	f.askedSlugs = append(f.askedSlugs, folded)
	f.askedAfter = seenAfter
	return f.answer, f.err
}

func TestCoverageCreditsEverySpellingOfAFoldedAnswer(t *testing.T) {
	// The database answers in folded slugs; the caller asks — and must be answered — in the
	// slugs it will store. One folded answer therefore owns every spelling that folds to it.
	fake := &fakeAsk{answer: []string{"cfoinsights"}}
	c := &coverage{ask: fake.ask}

	got, err := c.NonAggregatorCompanies(context.Background(),
		[]string{"cfo-insights", "cfoinsights", "acme"}, []string{"himalayas"})
	if err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}
	if !got["cfo-insights"] || !got["cfoinsights"] {
		t.Errorf("= %v, want both spellings of cfoinsights credited", got)
	}
	if got["acme"] {
		t.Error("acme was not in the answer and must not be reported as covered")
	}
}

func TestCoverageAnswersOnlyAboutSlugsItWasAskedFor(t *testing.T) {
	// A key the caller never asked about would be a coverage claim about a company nobody
	// enquired about, and the port's contract forbids it.
	fake := &fakeAsk{answer: []string{"cfoinsights", "someoneelse"}}
	c := &coverage{ask: fake.ask}

	got, err := c.NonAggregatorCompanies(context.Background(), []string{"cfo-insights"}, nil)
	if err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}
	if len(got) != 1 || !got["cfo-insights"] {
		t.Errorf("= %v, want exactly {cfo-insights}", got)
	}
}

func TestCoverageAsksAboutFoldedSlugsWithinTheFreshnessWindow(t *testing.T) {
	fake := &fakeAsk{}
	c := &coverage{ask: fake.ask}
	before := time.Now()

	if _, err := c.NonAggregatorCompanies(context.Background(), []string{"cfo-insights"}, nil); err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}
	after := time.Now()

	if len(fake.askedSlugs) != 1 || len(fake.askedSlugs[0]) != 1 || fake.askedSlugs[0][0] != "cfoinsights" {
		t.Errorf("asked %v, want the folded spelling", fake.askedSlugs)
	}
	// The cutoff is one window back from the moment of the call, so it must land inside the
	// bracket the call itself spans. Asserting the bracket rather than an exact instant keeps
	// the test honest about a clock it does not control.
	if fake.askedAfter.Before(before.Add(-coverageFreshness)) || fake.askedAfter.After(after.Add(-coverageFreshness)) {
		t.Errorf("seen_after = %v, want within [%v, %v]",
			fake.askedAfter, before.Add(-coverageFreshness), after.Add(-coverageFreshness))
	}
}

func TestCoverageSkipsASlugThatFoldsToNothing(t *testing.T) {
	// A slug of nothing but hyphens folds to "", which as a filter value would match every
	// row whose own fold is empty — a coverage claim about an employer nobody named.
	fake := &fakeAsk{}
	c := &coverage{ask: fake.ask}

	got, err := c.NonAggregatorCompanies(context.Background(), []string{"--"}, nil)
	if err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("= %v, want no coverage", got)
	}
	if len(fake.askedSlugs) != 0 {
		t.Errorf("asked %v, want no query at all", fake.askedSlugs)
	}
}

func TestCoverageBatchesLargeRequests(t *testing.T) {
	slugs := make([]string, coverageBatchSize+1)
	for i := range slugs {
		slugs[i] = "co" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	fake := &fakeAsk{}
	c := &coverage{ask: fake.ask}

	if _, err := c.NonAggregatorCompanies(context.Background(), slugs, nil); err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}
	if len(fake.askedSlugs) != 2 {
		t.Fatalf("made %d queries, want 2 for %d slugs", len(fake.askedSlugs), len(slugs))
	}
	if len(fake.askedSlugs[0]) != coverageBatchSize {
		t.Errorf("first batch held %d slugs, want %d", len(fake.askedSlugs[0]), coverageBatchSize)
	}
}

func TestCoverageReportsAQueryFailure(t *testing.T) {
	// The gate's caller turns an error into "nothing is covered" and logs it — the recoverable
	// direction. That decision belongs to the caller, so the adapter must surface the error
	// rather than swallow it into an empty answer that reads as a successful lookup.
	fake := &fakeAsk{err: errors.New("connection refused")}
	c := &coverage{ask: fake.ask}

	if _, err := c.NonAggregatorCompanies(context.Background(), []string{"acme"}, nil); err == nil {
		t.Fatal("want the query failure surfaced, got nil")
	}
}
