package contribution

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is an in-memory Repository for the service branch tests. Each behaviour is a
// tunable field so a test sets only what it exercises.
type fakeRepo struct {
	boardTracked  bool
	recordErr     error
	recorded      RecordInput
	recordCalls   int
	listByUserRet []Contribution
	companyName   string
	companySlug   string
}

func (f *fakeRepo) BoardTracked(_ context.Context, _, _ string) (bool, error) {
	return f.boardTracked, nil
}

func (f *fakeRepo) CompanyForBoard(_ context.Context, _, _ string) (string, string, bool, error) {
	return f.companyName, f.companySlug, f.companyName != "" || f.companySlug != "", nil
}

func (f *fakeRepo) Record(_ context.Context, in RecordInput) (Contribution, error) {
	f.recordCalls++
	f.recorded = in
	if f.recordErr != nil {
		return Contribution{}, f.recordErr
	}
	return Contribution{
		ID: 1, SubmittedBy: in.SubmittedBy, URL: in.URL,
		Source: in.Source, Board: in.Board, Status: "pending",
	}, nil
}

func (f *fakeRepo) ListByUser(_ context.Context, _ int64) ([]Contribution, error) {
	return f.listByUserRet, nil
}

// fakeResolver stands in for the network fallback.
type fakeResolver struct {
	source, board, canonical string
	ok                       bool
	calls                    int
}

func (r *fakeResolver) Resolve(_ context.Context, _ string) (string, string, string, bool) {
	r.calls++
	return r.source, r.board, r.canonical, r.ok
}

// newService wires the service with a network-free recognizer only (nil resolver).
func newService(repo Repository) *Service {
	return New(repo, nil)
}

func TestSubmitRejectsUnsupportedATS(t *testing.T) {
	repo := &fakeRepo{}
	_, _, _, err := newService(repo).Submit(context.Background(), 7, "https://example.com/careers/123")
	if !errors.Is(err, ErrUnsupportedATS) {
		t.Fatalf("err = %v, want ErrUnsupportedATS", err)
	}
	if repo.recordCalls != 0 {
		t.Errorf("recorded %d times, want 0 — nothing should be written", repo.recordCalls)
	}
}

func TestSubmitRejectsSingleTenantSource(t *testing.T) {
	// geekjob is a single-tenant aggregator — not a per-company board.
	_, _, _, err := newService(&fakeRepo{}).Submit(context.Background(), 7, "https://geekjob.ru/vacancy/6a1ebb85")
	if !errors.Is(err, ErrUnsupportedATS) {
		t.Fatalf("err = %v, want ErrUnsupportedATS", err)
	}
}

func TestSubmitRejectsNonURL(t *testing.T) {
	_, _, _, err := newService(&fakeRepo{}).Submit(context.Background(), 7, "not a url")
	if !errors.Is(err, ErrUnsupportedATS) {
		t.Fatalf("err = %v, want ErrUnsupportedATS", err)
	}
}

func TestSubmitRejectsWhenBoardAlreadyTracked(t *testing.T) {
	repo := &fakeRepo{boardTracked: true}
	_, source, board, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7")
	if !errors.Is(err, ErrBoardAlreadyTracked) {
		t.Fatalf("err = %v, want ErrBoardAlreadyTracked", err)
	}
	// Submit returns the resolved identity even on the tracked path, so the caller can look up
	// the company without re-recognizing.
	if source != "ashby" || board != "blitzy" {
		t.Errorf("returned (%q,%q), want (ashby, blitzy)", source, board)
	}
	if repo.recordCalls != 0 {
		t.Errorf("recorded %d times, want 0", repo.recordCalls)
	}
}

func TestSubmitRejectsDuplicateBoard(t *testing.T) {
	repo := &fakeRepo{recordErr: ErrBoardAlreadyContributed}
	_, _, _, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy")
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("err = %v, want ErrBoardAlreadyContributed", err)
	}
}

func TestSubmitRecordsBoardFromVacancyURL(t *testing.T) {
	repo := &fakeRepo{}
	got, source, board, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7?utm=x")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if source != "ashby" || board != "blitzy" || repo.recorded.Source != "ashby" || repo.recorded.Board != "blitzy" {
		t.Errorf("recorded = (%q,%q), want (ashby, blitzy)", repo.recorded.Source, repo.recorded.Board)
	}
	if repo.recorded.URL != "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7" {
		t.Errorf("stored URL = %q, want canonicalized", repo.recorded.URL)
	}
	if got.SubmittedBy != 7 {
		t.Errorf("SubmittedBy = %d, want 7", got.SubmittedBy)
	}
}

func TestSubmitUsesResolverForUnknownHost(t *testing.T) {
	// A vanity careers page (unknown host): recognizeBoard fails, the resolver detects the
	// embedded board.
	repo := &fakeRepo{}
	res := &fakeResolver{source: "greenhouse", board: "talkspace", canonical: "https://www.talkspace.com/careers/job", ok: true}
	_, source, board, err := New(repo, res).Submit(context.Background(), 7, "https://www.talkspace.com/careers/job?gh_jid=123")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.calls != 1 {
		t.Errorf("resolver called %d times, want 1", res.calls)
	}
	if source != "greenhouse" || board != "talkspace" || repo.recorded.Board != "talkspace" {
		t.Errorf("recorded = (%q,%q), want (greenhouse, talkspace)", repo.recorded.Source, repo.recorded.Board)
	}
	if repo.recorded.URL != "https://www.talkspace.com/careers/job" {
		t.Errorf("stored URL = %q, want the resolver's canonical", repo.recorded.URL)
	}
}

func TestSubmitSkipsResolverForRecognizedHost(t *testing.T) {
	// A known ATS host resolves network-free; the resolver must not be called.
	repo := &fakeRepo{}
	res := &fakeResolver{ok: true, source: "x", board: "y"}
	_, _, _, err := New(repo, res).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.calls != 0 {
		t.Errorf("resolver called %d times for a known host, want 0", res.calls)
	}
	if repo.recorded.Source != "ashby" {
		t.Errorf("recorded source = %q, want ashby (network-free)", repo.recorded.Source)
	}
}

func TestCompanyForBoard(t *testing.T) {
	repo := &fakeRepo{companyName: "Acme Corp", companySlug: "acme"}
	name, slug, ok := newService(repo).CompanyForBoard(context.Background(), "greenhouse", "acme")
	if !ok || name != "Acme Corp" || slug != "acme" {
		t.Errorf("CompanyForBoard = (%q,%q,%v), want (Acme Corp, acme, true)", name, slug, ok)
	}
	// No company on the board → not ok.
	if _, _, ok := newService(&fakeRepo{}).CompanyForBoard(context.Background(), "greenhouse", "empty"); ok {
		t.Error("CompanyForBoard(no company) ok = true, want false")
	}
}

func TestListMineReturnsRepoRows(t *testing.T) {
	repo := &fakeRepo{listByUserRet: []Contribution{{ID: 3}, {ID: 2}}}
	got, err := newService(repo).ListMine(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if len(got) != 2 || got[0].ID != 3 {
		t.Errorf("ListMine = %+v, want the repo rows in order", got)
	}
}
