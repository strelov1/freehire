package handler

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

func runUpdateProfile(t *testing.T, repo *fakeProfileRepo, args string) (any, error) {
	t.Helper()
	h := assistantWithProfile(repo, nil)
	return toolByName(t, h.profileMCPTools(), "update_profile").
		Run(context.Background(), 1, json.RawMessage(args))
}

func TestUpdateProfile_OmittedFieldsStayAsTheyWere(t *testing.T) {
	stamp := time.Now() // a stored row takes the guarded write
	repo := &fakeProfileRepo{getRet: userprofile.Profile{
		UserID: 1, UpdatedAt: &stamp, Specializations: []string{"backend"}, Skills: []string{"go"},
		Seniorities: []string{"senior"}, ExcludedSkills: []string{"php"},
		ExcludedSources: []string{"hh"}, ExcludedCompanies: []string{"acme"},
		JobLanguages:        []string{"en"},
		LocationPreferences: json.RawMessage(`{"work_modes":["remote"],"remote":{},"base":{},"relocation":{"open":false}}`),
	}}

	if _, err := runUpdateProfile(t, repo, `{"add_skills":["Python"]}`); err != nil {
		t.Fatalf("update_profile: %v", err)
	}

	got := repo.upserted
	if !slices.Equal(got.Skills, []string{"go", "python"}) {
		t.Errorf("skills = %v, want go and python", got.Skills)
	}
	if !slices.Equal(got.Specializations, []string{"backend"}) || !slices.Equal(got.Seniorities, []string{"senior"}) ||
		!slices.Equal(got.ExcludedSkills, []string{"php"}) || !slices.Equal(got.JobLanguages, []string{"en"}) ||
		!slices.Equal(got.ExcludedSources, []string{"hh"}) || !slices.Equal(got.ExcludedCompanies, []string{"acme"}) {
		t.Errorf("untouched fields changed: %+v", got)
	}
	if !strings.Contains(string(got.LocationPreferences), `"remote"`) {
		t.Errorf("location preferences were lost: %s", got.LocationPreferences)
	}
}

func TestUpdateProfile_UnknownSkillIsReportedNotSaved(t *testing.T) {
	repo := savedProfile()

	out, err := runUpdateProfile(t, repo, `{"add_skills":["python","zzz-not-a-skill"]}`)
	if err != nil {
		t.Fatalf("update_profile: %v", err)
	}

	if slices.Contains(repo.upserted.Skills, "zzz-not-a-skill") {
		t.Errorf("an unresolved skill was saved: %v", repo.upserted.Skills)
	}
	res := out.(profileUpdateResult)
	if !slices.Equal(res.NotInDictionary, []string{"zzz-not-a-skill"}) || res.Next == "" {
		t.Errorf("unresolved skill not reported: %+v", res)
	}
}

func TestUpdateProfile_ReplaceWithEmptyListClears(t *testing.T) {
	repo := savedProfile()
	if _, err := runUpdateProfile(t, repo, `{"skills":[]}`); err != nil {
		t.Fatalf("update_profile: %v", err)
	}
	if len(repo.upserted.Skills) != 0 || !slices.Equal(repo.upserted.Specializations, []string{"backend"}) {
		t.Errorf("want skills cleared and specializations kept, got %+v", repo.upserted)
	}
}

func TestUpdateProfile_RefusesAnEmptyOrContradictoryCall(t *testing.T) {
	for _, args := range []string{`{}`, `{"skills":["go"],"add_skills":["rust"]}`, `{"nope":1}`} {
		_, err := runUpdateProfile(t, savedProfile(), args)
		// The MCP server shows the agent only invalid-argument and fiber errors.
		if !errors.Is(err, assistant.ErrInvalidArguments) {
			t.Errorf("%s: err = %v, want ErrInvalidArguments", args, err)
		}
	}
}

func TestUpdateProfile_CreatesAProfileWhenNoneExists(t *testing.T) {
	repo := &fakeProfileRepo{getErr: userprofile.ErrNotFound}
	if _, err := runUpdateProfile(t, repo, `{"specializations":["backend"],"add_skills":["go"]}`); err != nil {
		t.Fatalf("update_profile: %v", err)
	}
	if !slices.Equal(repo.upserted.Skills, []string{"go"}) {
		t.Errorf("skills = %v", repo.upserted.Skills)
	}
}

func TestUpdateProfile_ConcurrentInitialSavePreservesOmittedFields(t *testing.T) {
	stamp := time.Now()
	concurrent := userprofile.Profile{
		UserID: 1, UpdatedAt: &stamp, Skills: []string{"go"},
		Specializations: []string{"backend"}, Seniorities: []string{"senior"},
		ExcludedSkills: []string{"php"}, ExcludedSources: []string{"hh"},
		ExcludedCompanies: []string{"acme"}, JobLanguages: []string{"en"},
		LocationPreferences: json.RawMessage(`{"work_modes":["remote"]}`),
	}
	repo := &fakeProfileRepo{createErr: userprofile.ErrConflict}
	reads := 0
	repo.getFn = func() (userprofile.Profile, error) {
		reads++
		if reads == 1 {
			// A website save creates the row after this missing-row snapshot.
			return userprofile.Profile{}, userprofile.ErrNotFound
		}
		return concurrent, nil
	}
	if _, err := runUpdateProfile(t, repo, `{"add_skills":["Python"]}`); err != nil {
		t.Fatalf("update_profile: %v", err)
	}
	got := repo.upserted
	if reads != 2 || repo.creates != 1 {
		t.Errorf("reads=%d creates=%d, want one conflicted create and a reload", reads, repo.creates)
	}
	if !slices.Equal(got.Skills, []string{"go", "python"}) ||
		!slices.Equal(got.Specializations, concurrent.Specializations) ||
		!slices.Equal(got.Seniorities, concurrent.Seniorities) ||
		!slices.Equal(got.ExcludedSkills, concurrent.ExcludedSkills) ||
		!slices.Equal(got.ExcludedSources, concurrent.ExcludedSources) ||
		!slices.Equal(got.ExcludedCompanies, concurrent.ExcludedCompanies) ||
		!slices.Equal(got.JobLanguages, concurrent.JobLanguages) ||
		!strings.Contains(string(got.LocationPreferences), `"remote"`) {
		t.Errorf("concurrent preferences were lost: %+v", got)
	}
}

func TestUpdateProfile_InitialCreateConflictIsBounded(t *testing.T) {
	repo := &fakeProfileRepo{getErr: userprofile.ErrNotFound, createErr: userprofile.ErrConflict}
	_, err := runUpdateProfile(t, repo, `{"add_skills":["go"]}`)
	if !errors.Is(err, userprofile.ErrConflict) || repo.creates != updateProfileAttempts {
		t.Fatalf("err=%v creates=%d, want ErrConflict after %d attempts", err, repo.creates, updateProfileAttempts)
	}
}

func TestUpdateProfile_InitialCreateFaultIsNotRetried(t *testing.T) {
	repo := &fakeProfileRepo{getErr: userprofile.ErrNotFound, createErr: errors.New("database unavailable")}
	_, err := runUpdateProfile(t, repo, `{"add_skills":["go"]}`)
	if err == nil || repo.creates != 1 {
		t.Fatalf("err=%v creates=%d, want failure after one attempt", err, repo.creates)
	}
}

func TestUpdateProfile_IsAccountOnlyAndDeclaresItsEffect(t *testing.T) {
	h := assistantWithProfile(savedProfile(), nil)
	tool := toolByName(t, mcpAccountTools(h, "https://x"), "update_profile")
	if tool.Title == "" || tool.Description == "" {
		t.Errorf("title and description are required: %+v", tool)
	}
	for _, k := range mcpKeyTools(h, "https://x") {
		if k.Name == "update_profile" {
			t.Error("update_profile must not be offered to an API key")
		}
	}
}
