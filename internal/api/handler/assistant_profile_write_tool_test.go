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
