package userprofile_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/identity/userprofile"
)

func TestCreateIfAbsent_NormalizesAndReportsConflicts(t *testing.T) {
	repo := &fakeRepo{upsertErr: userprofile.ErrConflict}
	_, err := userprofile.New(repo).CreateIfAbsent(context.Background(), 7,
		[]string{" backend ", "backend"}, []string{" Go ", "go"}, nil,
		[]string{"go", "php"}, []string{" HH "}, []string{" ACME "}, nil, "EN")
	if !errors.Is(err, userprofile.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	got := repo.upserted
	if got.UserID != 7 || !slices.Equal(got.Specializations, []string{"backend"}) ||
		!slices.Equal(got.Skills, []string{"go"}) || !slices.Equal(got.ExcludedSkills, []string{"php"}) ||
		!slices.Equal(got.ExcludedSources, []string{"hh"}) || !slices.Equal(got.ExcludedCompanies, []string{"acme"}) ||
		!slices.Equal(got.JobLanguages, []string{"en"}) {
		t.Errorf("unnormalized creation: %+v", got)
	}
}

func TestCreateIfAbsent_RejectsInvalidProfileBeforeWriting(t *testing.T) {
	repo := &fakeRepo{}
	_, err := userprofile.New(repo).CreateIfAbsent(context.Background(), 7,
		[]string{"unknown-role"}, nil, nil, nil, nil, nil, nil)
	if err == nil || repo.upsertCalled {
		t.Fatalf("err=%v wrote=%v, want validation error without persistence", err, repo.upsertCalled)
	}
}
