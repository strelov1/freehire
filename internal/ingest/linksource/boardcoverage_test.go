package linksource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/externalid"
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
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": board}, nil)

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
	if want := externalid.Namespace("acme", "222"); job.ExternalID != want {
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
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": recruiteeBoard()}, nil)

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
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": recruiteeBoard()}, nil)

	job, ok, err := bc.Resolve(context.Background(), "https://acme.recruitee.com/o/long-gone")
	if err != nil {
		t.Fatalf("Resolve err = %v, want nil — a missing posting is not a failure", err)
	}
	if ok {
		t.Errorf("Resolve ok = true (%+v), want false", job)
	}
}

// An Unreadable marker is a posting's identity WITHOUT its detail, yielded so the run can
// withhold that employer's close scope. It answers both of pickPosting's predicates — and
// unlike a SeenRefresh, which a link resolver can never meet because its seen-predicate
// hydrates exactly the posting being asked about, this one fires on precisely the posting
// somebody pasted. Returning it hands the caller a titleless job, which job.New refuses as an
// invalid draft, which reaches POST /jobs/find as a 500 on a URL whose honest answer is "we
// could not read a vacancy from that url".
func TestBoardCoverageDoesNotResolveAPostingItCouldNotRead(t *testing.T) {
	board := &fakeBoard{
		provider: "recruitee",
		jobs: []sources.Job{
			{ExternalID: "111", URL: "https://acme.recruitee.com/o/junior-go", Title: "Junior Go", Company: "Acme"},
			// The pasted link's own posting: seen in the listing, unreadable in detail.
			{ExternalID: "222", URL: "https://acme.recruitee.com/o/senior-go", Company: "Acme", Unreadable: true},
		},
	}
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": board}, nil)

	for _, raw := range []string{
		"https://acme.recruitee.com/o/senior-go", // matched by URL
		"https://acme.recruitee.com/o/222",       // matched by the id in the last segment
	} {
		job, ok, err := bc.Resolve(context.Background(), raw)
		if err != nil {
			t.Errorf("Resolve(%q) err = %v, want nil — an unreadable page is not a board failure", raw, err)
		}
		if ok {
			t.Errorf("Resolve(%q) ok = true (title %q), want false — a marker carries no vacancy, and "+
				"the caller turns one into a 500", raw, job.Title)
		}
	}
}

func TestBoardCoverageReportsAFetchFailureAsAnError(t *testing.T) {
	// An unreachable board must NOT look like "no such vacancy": the caller would record the
	// link as unimportable and never retry.
	boom := errors.New("board unreachable")
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": &fakeBoard{provider: "recruitee", err: boom}}, nil)

	if _, ok, err := bc.Resolve(context.Background(), "https://acme.recruitee.com/o/senior-go"); ok || !errors.Is(err, boom) {
		t.Errorf("Resolve = (ok %v, err %v), want (false, the fetch error)", ok, err)
	}
}

// fakeCatalog answers one board's crawl entry, or an error.
type fakeCatalog struct {
	entry sources.CompanyEntry
	err   error
}

func (c fakeCatalog) Entry(_ context.Context, source, board string) (sources.CompanyEntry, bool, error) {
	if c.err != nil {
		return sources.CompanyEntry{}, false, c.err
	}
	return c.entry, c.entry.Provider == source && c.entry.Board != "", nil
}

func TestBoardCoverageReadsTheBoardThroughItsCatalogueEntry(t *testing.T) {
	// The adapter must be handed what a crawl hands it — the employer above all, which most
	// adapters copy onto every posting — and the id namespaced with the board as stored.
	board := recruiteeBoard()
	stored := sources.CompanyEntry{Provider: "recruitee", Board: "Acme", Company: "Acme Robotics"}
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": board}, fakeCatalog{entry: stored})

	job, ok, err := bc.Resolve(context.Background(), "https://acme.recruitee.com/o/senior-go")
	if err != nil || !ok {
		t.Fatalf("Resolve = (ok %v, err %v), want the linked vacancy", ok, err)
	}
	if len(board.asked) != 1 || board.asked[0].Company != stored.Company || board.asked[0].Board != stored.Board {
		t.Errorf("adapter asked for %+v, want the catalogue entry %+v", board.asked, stored)
	}
	if want := externalid.Namespace("Acme", "222"); job.ExternalID != want {
		t.Errorf("ExternalID = %q, want %q (the stored board spelling a crawl namespaces with)", job.ExternalID, want)
	}
}

func TestBoardCoverageReportsACatalogueFailureAsAnError(t *testing.T) {
	// Reading the board without its entry is exactly the write that blanks a crawled row's
	// employer, so an unreadable catalogue stops the resolve rather than falling back to it.
	boom := errors.New("catalogue unreachable")
	board := recruiteeBoard()
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": board}, fakeCatalog{err: boom})

	if _, ok, err := bc.Resolve(context.Background(), "https://acme.recruitee.com/o/senior-go"); ok || !errors.Is(err, boom) {
		t.Errorf("Resolve = (ok %v, err %v), want (false, the catalogue error)", ok, err)
	}
	if len(board.asked) != 0 {
		t.Errorf("adapter fetched %+v without its catalogue entry", board.asked)
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
	bc := NewBoardCoverage(map[string]sources.Source{"recruitee": board}, nil)

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
	job, ok, err := ResolveOnBoard(context.Background(), reg, nil, "recruitee", "acme",
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
	job, ok, err := ResolveOnBoard(context.Background(), reg, nil, "workday", "acme", raw)
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

// SmartRecruiters writes a posting's id at the FRONT of its last path segment
// (…/<company>/<id>-<title-slug>), so the id is never a whole segment. Once that adapter
// hydrates, a seen predicate matching whole segments only would call the target seen, hand back
// a URL-less refresh, and lose a link that resolved while the adapter hydrated everything.
func TestResolveOnBoardHydratesAnIDPrefixedSlugSegment(t *testing.T) {
	board := &fakeHydratingBoard{
		provider: "smartrecruiters",
		jobs: []sources.Job{
			{ExternalID: "744000011111111", URL: "https://jobs.smartrecruiters.com/Acme/744000011111111-junior-go", Title: "Junior Go"},
			{ExternalID: "744000022222222", URL: "https://jobs.smartrecruiters.com/Acme/744000022222222-senior-go", Title: "Senior Go"},
		},
	}
	reg := map[string]sources.Source{"smartrecruiters": board}

	raw := "https://jobs.smartrecruiters.com/Acme/744000022222222-senior-go"
	job, ok, err := ResolveOnBoard(context.Background(), reg, nil, "smartrecruiters", "Acme", raw)
	if err != nil || !ok {
		t.Fatalf("ResolveOnBoard = (ok %v, err %v), want the linked vacancy", ok, err)
	}
	if job.Title != "Senior Go" || job.SeenRefresh {
		t.Errorf("resolved %+v, want the hydrated Senior Go posting", job)
	}
	if want := []string{"744000022222222"}; !slices.Equal(board.hydrated, want) {
		t.Errorf("hydrated postings = %v, want only %v", board.hydrated, want)
	}
}

// A SmartRecruiters slug follows the title, so a link pasted before the title was edited
// carries an old slug after the right id. The id still names the posting.
func TestResolveOnBoardMatchesAnIDPrefixedSegmentWithAStaleSlug(t *testing.T) {
	board := &fakeHydratingBoard{
		provider: "smartrecruiters",
		jobs: []sources.Job{
			{ExternalID: "744000022222222", URL: "https://jobs.smartrecruiters.com/Acme/744000022222222-staff-go", Title: "Staff Go"},
		},
	}
	reg := map[string]sources.Source{"smartrecruiters": board}

	raw := "https://jobs.smartrecruiters.com/Acme/744000022222222-senior-go"
	job, ok, err := ResolveOnBoard(context.Background(), reg, nil, "smartrecruiters", "Acme", raw)
	if err != nil || !ok {
		t.Fatalf("ResolveOnBoard = (ok %v, err %v), want the vacancy matched by the id in front of the stale slug", ok, err)
	}
	if job.Title != "Staff Go" {
		t.Errorf("resolved %q, want Staff Go", job.Title)
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
	reg := ImportRegistry(nil, ingest, nil)

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

// hrmdirectBoard is an HRM Direct board as the adapter yields it since the platform moved to
// ClearCompany: the external id is the posting's GUID lower-cased, and the URL is the career page
// with ?jobId=<that id> — every posting on the board shares the one path. Ids are real postings
// on the bkbehavior board, 2026-10-30.
func hrmdirectBoard() *fakeBoard {
	const page = "https://apply.clearcompany.com/bkbehavior?jobId="
	return &fakeBoard{
		provider: "hrmdirect",
		jobs: []sources.Job{
			{ExternalID: "3522713d-0ff6-5e08-72c3-617411eaccb5", URL: page + "3522713d-0ff6-5e08-72c3-617411eaccb5", Title: "Behavior Technician", Company: "BK Behavior"},
			{ExternalID: "7864d3e4-a031-ab73-23d1-4daa731c9ca4", URL: page + "7864d3e4-a031-ab73-23d1-4daa731c9ca4", Title: "Clinical Supervisor", Company: "BK Behavior"},
		},
	}
}

// A ClearCompany posting link resolves to the posting its jobId names, under the identity the
// hrmdirect crawl of that board writes.
func TestBoardCoverageResolvesAClearCompanyPostingByItsJobID(t *testing.T) {
	cases := []struct{ name, raw string }{
		{"canonical link", "https://apply.clearcompany.com/bkbehavior?jobId=7864d3e4-a031-ab73-23d1-4daa731c9ca4"},
		// The widget works with upper-case ids and only rewrites the address bar to lower case
		// once the page loads, so a link copied from elsewhere may carry either spelling.
		{"upper-case id", "https://apply.clearcompany.com/bkbehavior?jobId=7864D3E4-A031-AB73-23D1-4DAA731C9CA4"},
		{"upper-case slug and tracking", "https://apply.clearcompany.com/BKBehavior?jobId=7864d3e4-a031-ab73-23d1-4daa731c9ca4&utm_source=telegram"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bc := NewBoardCoverage(map[string]sources.Source{"hrmdirect": hrmdirectBoard()}, nil)
			u, _ := url.Parse(c.raw)
			if !bc.Match(u) {
				t.Fatal("Match = false, want true")
			}
			job, ok, err := bc.Resolve(context.Background(), c.raw)
			if err != nil || !ok {
				t.Fatalf("Resolve = (ok %v, err %v), want the linked posting", ok, err)
			}
			if want := externalid.Namespace("bkbehavior", "7864d3e4-a031-ab73-23d1-4daa731c9ca4"); job.ExternalID != want {
				t.Errorf("ExternalID = %q, want %q", job.ExternalID, want)
			}
			if got := bc.(PerLinkSource).SourceFor(u); got != "hrmdirect" {
				t.Errorf("SourceFor = %q, want hrmdirect", got)
			}
		})
	}
}

// The career page shares its path with every posting on the board, so a link to the page itself
// names no posting — it must not resolve to whichever posting the board lists first, even when
// the board holds only one.
func TestBoardCoverageDoesNotReadAClearCompanyCareerPageAsAPosting(t *testing.T) {
	single := hrmdirectBoard()
	single.jobs = single.jobs[:1]
	for name, board := range map[string]*fakeBoard{"two postings": hrmdirectBoard(), "one posting": single} {
		t.Run(name, func(t *testing.T) {
			bc := NewBoardCoverage(map[string]sources.Source{"hrmdirect": board}, nil)
			job, ok, err := bc.Resolve(context.Background(), "https://apply.clearcompany.com/bkbehavior")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if ok {
				t.Errorf("resolved %s, want nothing: the career page names no posting", job.ExternalID)
			}
		})
	}
}
