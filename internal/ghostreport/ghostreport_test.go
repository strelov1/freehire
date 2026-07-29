package ghostreport_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ghostreport"
)

var now = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

type fakeRepo struct {
	created     int
	createdOn   time.Time
	createErr   error
	retractErr  error
	filedToday  int
	countErr    error
	retractions int
}

func (f *fakeRepo) Create(_ context.Context, _, _ int64, appliedOn time.Time) (ghostreport.Report, error) {
	if f.createErr != nil {
		return ghostreport.Report{}, f.createErr
	}
	f.created++
	f.createdOn = appliedOn
	return ghostreport.Report{JobID: 7, AppliedOn: appliedOn}, nil
}

func (f *fakeRepo) Retract(_ context.Context, _, _ int64) error {
	if f.retractErr != nil {
		return f.retractErr
	}
	f.retractions++
	return nil
}

func (f *fakeRepo) CountFiledSince(_ context.Context, _ int64, _ time.Time) (int, error) {
	return f.filedToday, f.countErr
}

func file(t *testing.T, repo *fakeRepo, appliedOn time.Time) error {
	t.Helper()
	_, err := ghostreport.New(repo).File(context.Background(), 1, 7, appliedOn, now)
	return err
}

func TestFile_StoresAClaimAboutAPastApplication(t *testing.T) {
	repo := &fakeRepo{}
	appliedOn := now.AddDate(0, 0, -30)

	if err := file(t, repo, appliedOn); err != nil {
		t.Fatalf("File: %v", err)
	}
	if repo.created != 1 {
		t.Errorf("created = %d, want 1", repo.created)
	}
	if !repo.createdOn.Equal(appliedOn) {
		t.Errorf("applied_on = %v, want %v", repo.createdOn, appliedOn)
	}
}

func TestFile_RejectsAFutureApplyDate(t *testing.T) {
	repo := &fakeRepo{}

	err := file(t, repo, now.AddDate(0, 0, 1))
	if !errors.Is(err, ghostreport.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
	if repo.created != 0 {
		t.Error("a future claim reached the repository")
	}
}

// A year-old application says nothing about whether a posting is live now.
func TestFile_RejectsAClaimOlderThanAYear(t *testing.T) {
	repo := &fakeRepo{}

	err := file(t, repo, now.AddDate(0, 0, -366))
	if !errors.Is(err, ghostreport.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
	if repo.created != 0 {
		t.Error("a stale claim reached the repository")
	}
}

// Maturity is not the service's business: a fresh claim is recorded, and the
// evidence aggregator decides when it starts counting. Refusing it here would
// lose the report of somebody who applied yesterday and came back later.
func TestFile_StoresAClaimThatIsNotYetEvidence(t *testing.T) {
	repo := &fakeRepo{}

	if err := file(t, repo, now.AddDate(0, 0, -3)); err != nil {
		t.Fatalf("File: %v", err)
	}
	if repo.created != 1 {
		t.Error("a fresh claim was refused; it should be stored and mature later")
	}
}

func TestFile_PassesThroughADuplicate(t *testing.T) {
	repo := &fakeRepo{createErr: ghostreport.ErrDuplicate}

	if err := file(t, repo, now.AddDate(0, 0, -30)); !errors.Is(err, ghostreport.ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

// A real job seeker does not have more than a handful of silent applications to
// report in one day; the cap bounds what one account can do to the catalogue.
func TestFile_RefusesPastTheDailyCap(t *testing.T) {
	repo := &fakeRepo{filedToday: ghostreport.DailyCap}

	err := file(t, repo, now.AddDate(0, 0, -30))
	if !errors.Is(err, ghostreport.ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
	if repo.created != 0 {
		t.Error("the capped request still reached the repository")
	}
}

func TestFile_AllowsTheLastFilingUnderTheCap(t *testing.T) {
	repo := &fakeRepo{filedToday: ghostreport.DailyCap - 1}

	if err := file(t, repo, now.AddDate(0, 0, -30)); err != nil {
		t.Errorf("File at cap-1: %v, want it allowed", err)
	}
}

func TestRetract_WithdrawsTheClaim(t *testing.T) {
	repo := &fakeRepo{}

	if err := ghostreport.New(repo).Retract(context.Background(), 1, 7); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if repo.retractions != 1 {
		t.Errorf("retractions = %d, want 1", repo.retractions)
	}
}

func TestRetract_PassesThroughAMissingClaim(t *testing.T) {
	repo := &fakeRepo{retractErr: ghostreport.ErrNotFound}

	err := ghostreport.New(repo).Retract(context.Background(), 1, 7)
	if !errors.Is(err, ghostreport.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
