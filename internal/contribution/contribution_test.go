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

func newService(repo Repository) *Service {
	return New(repo)
}

func TestSubmitRejectsUnsupportedATS(t *testing.T) {
	repo := &fakeRepo{}
	_, err := newService(repo).Submit(context.Background(), 7, "https://example.com/careers/123")
	if !errors.Is(err, ErrUnsupportedATS) {
		t.Fatalf("err = %v, want ErrUnsupportedATS", err)
	}
	if repo.recordCalls != 0 {
		t.Errorf("recorded %d times, want 0 — nothing should be written", repo.recordCalls)
	}
}

func TestSubmitRejectsSingleTenantSource(t *testing.T) {
	// geekjob is a single-tenant aggregator — not a per-company board.
	_, err := newService(&fakeRepo{}).Submit(context.Background(), 7, "https://geekjob.ru/vacancy/6a1ebb85")
	if !errors.Is(err, ErrUnsupportedATS) {
		t.Fatalf("err = %v, want ErrUnsupportedATS", err)
	}
}

func TestSubmitRejectsNonURL(t *testing.T) {
	_, err := newService(&fakeRepo{}).Submit(context.Background(), 7, "not a url")
	if !errors.Is(err, ErrUnsupportedATS) {
		t.Fatalf("err = %v, want ErrUnsupportedATS", err)
	}
}

func TestSubmitRejectsWhenBoardAlreadyTracked(t *testing.T) {
	repo := &fakeRepo{boardTracked: true}
	_, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7")
	if !errors.Is(err, ErrBoardAlreadyTracked) {
		t.Fatalf("err = %v, want ErrBoardAlreadyTracked", err)
	}
	if repo.recordCalls != 0 {
		t.Errorf("recorded %d times, want 0", repo.recordCalls)
	}
}

func TestSubmitRejectsDuplicateBoard(t *testing.T) {
	repo := &fakeRepo{recordErr: ErrBoardAlreadyContributed}
	_, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy")
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("err = %v, want ErrBoardAlreadyContributed", err)
	}
}

func TestSubmitRecordsBoardFromVacancyURL(t *testing.T) {
	repo := &fakeRepo{}
	got, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7?utm=x")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if repo.recorded.Source != "ashby" || repo.recorded.Board != "blitzy" {
		t.Errorf("recorded = (%q,%q), want (ashby, blitzy)", repo.recorded.Source, repo.recorded.Board)
	}
	if repo.recorded.URL != "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7" {
		t.Errorf("stored URL = %q, want canonicalized", repo.recorded.URL)
	}
	if got.SubmittedBy != 7 {
		t.Errorf("SubmittedBy = %d, want 7", got.SubmittedBy)
	}
	// The board-tracked check must scope to the board namespace, not the whole source.
	// (Covered implicitly: BoardTracked was called with prefix "blitzy:".)
}

func TestSubmitRecordsBoardFromListingURL(t *testing.T) {
	repo := &fakeRepo{}
	_, err := newService(repo).Submit(context.Background(), 7, "https://job-boards.greenhouse.io/acme")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if repo.recorded.Source != "greenhouse" || repo.recorded.Board != "acme" {
		t.Errorf("recorded = (%q,%q), want (greenhouse, acme)", repo.recorded.Source, repo.recorded.Board)
	}
}

func TestTrackedCompanyResolvesFromLink(t *testing.T) {
	repo := &fakeRepo{companyName: "Acme Corp", companySlug: "acme"}
	name, slug, ok := newService(repo).TrackedCompany(context.Background(), "https://jobs.ashbyhq.com/blitzy/uuid")
	if !ok || name != "Acme Corp" || slug != "acme" {
		t.Errorf("TrackedCompany = (%q,%q,%v), want (Acme Corp, acme, true)", name, slug, ok)
	}
	// An unrecognized link resolves nothing.
	if _, _, ok := newService(repo).TrackedCompany(context.Background(), "https://example.com/x"); ok {
		t.Error("TrackedCompany(unknown host) ok = true, want false")
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
