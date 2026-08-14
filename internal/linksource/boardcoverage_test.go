package linksource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/sources"
)

// fakeBoard is a stand-in ingest adapter: it serves one tenant board's postings, and records
// which board it was asked for.
type fakeBoard struct {
	provider string
	jobs     []sources.Job
	err      error
	asked    []sources.CompanyEntry
}

func (f *fakeBoard) Provider() string { return f.provider }

func (f *fakeBoard) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	f.asked = append(f.asked, e)
	if f.err != nil {
		return nil, f.err
	}
	return f.jobs, nil
}

// recruiteeBoard is a board on an ATS that has an ingest adapter but no single-page
// link-source adapter — the case board coverage exists for.
func recruiteeBoard() *fakeBoard {
	return &fakeBoard{
		provider: "recruitee",
		jobs: []sources.Job{
			{ExternalID: "111", URL: "https://acme.recruitee.com/o/junior-go", Title: "Junior Go", Company: "Acme"},
			{ExternalID: "222", URL: "https://acme.recruitee.com/o/senior-go", Title: "Senior Go", Company: "Acme"},
		},
	}
}

func TestBoardCoverageResolvesAVacancyWithNoDedicatedAdapter(t *testing.T) {
	board := recruiteeBoard()
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": board})

	raw := "https://acme.recruitee.com/o/senior-go?utm_source=telegram"
	u, _ := url.Parse(raw)
	if !bc.Match(u) {
		t.Fatal("Match = false for a recognised board with an ingest adapter, want true")
	}

	job, ok, err := bc.Resolve(context.Background(), raw)
	if err != nil || !ok {
		t.Fatalf("Resolve = (ok %v, err %v), want the linked vacancy", ok, err)
	}
	if job.Title != "Senior Go" {
		t.Errorf("resolved %q, want the vacancy the link points at (Senior Go)", job.Title)
	}
	// The identity must be the one the ingest crawl of this board would write, so a later
	// crawl dedups onto this row instead of creating a second posting.
	if want := sources.NamespaceExternalID("acme", "222"); job.ExternalID != want {
		t.Errorf("ExternalID = %q, want %q — the ingest namespacing", job.ExternalID, want)
	}
	// One adapter serves many platforms, so the stored identity comes from the link, not from
	// the adapter — otherwise every board-covered import would land under one bogus source.
	per, isPerLink := bc.(PerLinkSource)
	if !isPerLink {
		t.Fatal("board coverage does not implement PerLinkSource, so jobs.source would be wrong")
	}
	if got := per.SourceFor(u); got != "recruitee" {
		t.Errorf("SourceFor = %q, want recruitee", got)
	}
	if len(board.asked) != 1 || board.asked[0].Board != "acme" {
		t.Errorf("fetched %+v, want exactly the acme board", board.asked)
	}
}

func TestBoardCoverageDeclinesWhatItCannotServe(t *testing.T) {
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": recruiteeBoard()})

	cases := []struct {
		name string
		raw  string
	}{
		{"unrecognised host", "https://example.com/careers/1"},
		{"recognised ATS with no ingest adapter registered", "https://acme.bamboohr.com/careers/42"},
		{"recognised host carrying no board", "https://recruitee.com/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(c.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if bc.Match(u) {
				t.Errorf("Match(%q) = true, want false — nothing here can be served", c.raw)
			}
		})
	}
}

func TestBoardCoverageResolvesNothingWhenTheBoardLacksTheVacancy(t *testing.T) {
	// A closed or delisted posting: the board fetches fine but does not carry the link. That
	// is "nothing resolved", not an error — the caller falls back to recording the link.
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": recruiteeBoard()})

	job, ok, err := bc.Resolve(context.Background(), "https://acme.recruitee.com/o/long-gone")
	if err != nil {
		t.Fatalf("Resolve err = %v, want nil — a missing posting is not a failure", err)
	}
	if ok {
		t.Errorf("Resolve ok = true (%+v), want false", job)
	}
}

func TestBoardCoverageReportsAFetchFailureAsAnError(t *testing.T) {
	// An unreachable board must NOT look like "no such vacancy": the caller would record the
	// link as unimportable and never retry.
	boom := errors.New("board unreachable")
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": &fakeBoard{provider: "recruitee", err: boom}})

	if _, ok, err := bc.Resolve(context.Background(), "https://acme.recruitee.com/o/senior-go"); ok || !errors.Is(err, boom) {
		t.Errorf("Resolve = (ok %v, err %v), want (false, the fetch error)", ok, err)
	}
}

func TestBoardCoverageMatchesAVacancyByItsIDInThePath(t *testing.T) {
	// Boards often serve a posting under a URL that differs from the one in the feed (a
	// locale prefix, a slug variant). The posting id in the last path segment still
	// identifies it.
	board := &fakeBoard{
		provider: "recruitee",
		jobs: []sources.Job{
			{ExternalID: "98765", URL: "https://acme.recruitee.com/o/senior-go/98765", Title: "Senior Go"},
		},
	}
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": board})

	job, ok, err := bc.Resolve(context.Background(), "https://acme.recruitee.com/o/some-other-slug/98765")
	if err != nil || !ok {
		t.Fatalf("Resolve = (ok %v, err %v), want the vacancy matched by its id", ok, err)
	}
	if job.Title != "Senior Go" {
		t.Errorf("resolved %q, want Senior Go", job.Title)
	}
}

func TestBoardCoverageMatchesAVacancyByAnIDBeforeTheSlug(t *testing.T) {
	// A storefront over the board puts a human-readable slug AFTER the id
	// (…/jobs/<id>/<title>/), so reading only the last path segment finds a title where an id
	// was expected and the posting looks absent from its own board.
	board := &fakeBoard{
		provider: "recruitee",
		jobs: []sources.Job{
			{ExternalID: "7862086", URL: "https://acme.recruitee.com/o/senior-go", Title: "Senior Go"},
		},
	}
	// Reached through ResolveOnBoard, since the board came from elsewhere (the intake resolved
	// it) rather than from this unrecognisable host.
	reg := map[string]sources.Source{"recruitee": board}
	job, ok, err := ResolveOnBoard(context.Background(), reg, "recruitee", "acme",
		"https://careers.acme.test/en/jobs/7862086/senior-go/")
	if err != nil || !ok {
		t.Fatalf("ResolveOnBoard = (ok %v, err %v), want the vacancy matched by the id before the slug", ok, err)
	}
	if job.Title != "Senior Go" {
		t.Errorf("resolved %q, want Senior Go", job.Title)
	}
}

// fakeHydratingBoard is a stand-in for a HydratingSource ingest adapter (e.g. workday): FetchNew
// hydrates full detail only for the postings its seen predicate reports as new, recording which
// ones so a test can pin exactly what got hydrated. Fetch is the list-only fallback and records
// whether it was called at all — ResolveOnBoard must prefer FetchNew.
type fakeHydratingBoard struct {
	provider string
	jobs     []sources.Job
	fetched  bool
	hydrated []string
}

func (f *fakeHydratingBoard) Provider() string { return f.provider }

func (f *fakeHydratingBoard) Fetch(_ context.Context, _ sources.CompanyEntry) ([]sources.Job, error) {
	f.fetched = true
	return f.jobs, nil
}

func (f *fakeHydratingBoard) FetchNew(_ context.Context, _ sources.CompanyEntry, seen func(externalID string) bool) ([]sources.Job, error) {
	out := make([]sources.Job, 0, len(f.jobs))
	for _, j := range f.jobs {
		if seen(j.ExternalID) {
			out = append(out, sources.Job{ExternalID: j.ExternalID, URL: j.URL, Title: j.Title, SeenRefresh: true})
			continue
		}
		f.hydrated = append(f.hydrated, j.ExternalID)
		out = append(out, j)
	}
	return out, nil
}

func TestResolveOnBoardHydratesOnlyTheTargetPostingOnAHydratingSource(t *testing.T) {
	board := &fakeHydratingBoard{
		provider: "workday",
		jobs: []sources.Job{
			{ExternalID: "111", URL: "https://acme.wd1.myworkdayjobs.com/en-US/careers/job/111", Title: "Junior Go", Description: "full detail 111"},
			{ExternalID: "222", URL: "https://acme.wd1.myworkdayjobs.com/en-US/careers/job/222", Title: "Senior Go", Description: "full detail 222"},
			{ExternalID: "333", URL: "https://acme.wd1.myworkdayjobs.com/en-US/careers/job/333", Title: "Staff Go", Description: "full detail 333"},
		},
	}
	reg := map[string]sources.Source{"workday": board}

	raw := "https://acme.wd1.myworkdayjobs.com/en-US/careers/job/222"
	job, ok, err := ResolveOnBoard(context.Background(), reg, "workday", "acme", raw)
	if err != nil || !ok {
		t.Fatalf("ResolveOnBoard = (ok %v, err %v), want the linked vacancy", ok, err)
	}
	if job.Title != "Senior Go" || job.Description != "full detail 222" {
		t.Errorf("resolved %+v, want the fully hydrated Senior Go posting", job)
	}
	if board.fetched {
		t.Error("Fetch was called — ResolveOnBoard must prefer FetchNew for a HydratingSource, to avoid hydrating the whole board")
	}
	if want := []string{"222"}; !slices.Equal(board.hydrated, want) {
		t.Errorf("hydrated postings = %v, want only %v — resolving one link must not hydrate the rest of the board", board.hydrated, want)
	}
}

// TestImportRegistryOrder pins the resolver order the import path depends on. Getting it
// wrong is silent and expensive: board coverage ahead of the host-scoped adapters would fetch
// a whole board where a per-job API call would do, and anything after generic would never be
// reached, since generic matches every page and Find picks exactly one adapter.
func TestImportRegistryOrder(t *testing.T) {
	ingest := map[string]sources.Source{
		"greenhouse": &fakeBoard{provider: "greenhouse"},
		"recruitee":  recruiteeBoard(),
	}
	reg := ImportRegistry(nil, ingest)

	cases := []struct {
		name     string
		raw      string
		wantType any
	}{
		// Greenhouse has both a dedicated adapter and an ingest adapter — the dedicated one wins.
		{"platform with a dedicated adapter", "https://job-boards.greenhouse.io/acme/jobs/123", greenhouse{}},
		// Recruitee has only an ingest adapter — board coverage takes it.
		{"platform with only an ingest adapter", "https://acme.recruitee.com/o/senior-go", boardCoverage{}},
		// Anything else falls to the last-resort resolver.
		{"unrecognised page", "https://example.com/careers/1", generic{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(c.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := Find(reg, u)
			if got == nil {
				t.Fatalf("no adapter matched %q", c.raw)
			}
			if fmt.Sprintf("%T", got) != fmt.Sprintf("%T", c.wantType) {
				t.Errorf("%q resolved by %T, want %T", c.raw, got, c.wantType)
			}
		})
	}
}
