package userprofile_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/identity/userprofile"
)

func TestSaveJobLanguages(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input, want []string
		invalid     bool
	}{
		{name: "any language", want: []string{}},
		{name: "normalize and deduplicate", input: []string{" EN ", "ru", "en", ""}, want: []string{"en", "ru"}},
		{name: "reject names", input: []string{"English"}, invalid: true},
		{name: "reject filter syntax", input: []string{`en OR true`}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := userprofile.New(repo).Save(context.Background(), 7, []string{}, []string{}, nil, nil, nil, nil, nil, tc.input...)
			if tc.invalid {
				if !errors.Is(err, userprofile.ErrInvalidJobLanguage) || repo.upsertCalled {
					t.Fatalf("invalid input persisted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(repo.upserted.JobLanguages, tc.want) {
				t.Fatalf("languages = %v, want %v", repo.upserted.JobLanguages, tc.want)
			}
		})
	}
}

func TestMergeSkillsPreservesJobLanguages(t *testing.T) {
	for _, guarded := range []bool{false, true} {
		profile := userprofile.Profile{UserID: 7, Skills: []string{"go"}, JobLanguages: []string{"en", "ru"}}
		if guarded {
			now := time.Now()
			profile.UpdatedAt = &now
		}
		repo := &fakeRepo{getRet: profile}
		if err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"rust"}); err != nil {
			t.Fatal(err)
		}
		got := repo.upserted.JobLanguages
		if guarded {
			got = repo.guardedUpserted.JobLanguages
		}
		if !reflect.DeepEqual(got, profile.JobLanguages) {
			t.Fatalf("guarded=%v: languages lost: %v", guarded, got)
		}
	}
}
