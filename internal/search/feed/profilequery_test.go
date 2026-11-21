package feed

import (
	"encoding/json"
	"maps"
	"os"
	"testing"

	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/search/search"
)

// parityCase is one row of the fixture the web reads too (web/src/lib/profileFeedParity.test.ts):
// the params filtersToParams(filtersFromProfile(profile)) emits, which ProfileQuery must match.
type parityCase struct {
	Name    string `json:"name"`
	Profile struct {
		Specializations     []string        `json:"specializations"`
		Skills              []string        `json:"skills"`
		Seniorities         []string        `json:"seniorities"`
		ExcludedSkills      []string        `json:"excluded_skills"`
		ExcludedSources     []string        `json:"excluded_sources"`
		ExcludedCompanies   []string        `json:"excluded_companies"`
		JobLanguages        []string        `json:"job_languages"`
		LocationPreferences json.RawMessage `json:"location_preferences"`
	} `json:"profile"`
	Params map[string]string `json:"params"`
}

func (c parityCase) profile() userprofile.Profile {
	p := c.Profile
	return userprofile.Profile{
		Specializations: p.Specializations, Skills: p.Skills, Seniorities: p.Seniorities,
		ExcludedSkills: p.ExcludedSkills, ExcludedSources: p.ExcludedSources,
		ExcludedCompanies: p.ExcludedCompanies, JobLanguages: p.JobLanguages,
		LocationPreferences: p.LocationPreferences,
	}
}

func loadParityCases(t *testing.T) []parityCase {
	t.Helper()
	raw, err := os.ReadFile("../../../web/src/lib/fixtures/profileFeedParity.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []parityCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 12 {
		t.Fatalf("want at least 12 shared cases, got %d", len(cases))
	}
	return cases
}

func TestProfileQuerySharesCasesWithTheWeb(t *testing.T) {
	for _, c := range loadParityCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			got, reason := ProfileQuery(c.profile())
			flat := map[string]string{}
			for k, vs := range got {
				if len(vs) != 1 {
					t.Fatalf("param %s has %d values, want 1", k, len(vs))
				}
				flat[k] = vs[0]
			}
			if !maps.Equal(flat, c.Params) {
				t.Errorf("params\n got %v\nwant %v", flat, c.Params)
			}
			wantReason := ""
			if len(c.Params) == 0 {
				wantReason = ReasonNoUsableFields
			}
			if reason != wantReason {
				t.Errorf("reason = %q, want %q", reason, wantReason)
			}
		})
	}
}

// Every param the port emits must be one the search reads, or the feed would widen silently.
func TestProfileQueryNarrowsTheSearch(t *testing.T) {
	for _, c := range loadParityCases(t) {
		if len(c.Params) == 0 {
			continue
		}
		v, _ := ProfileQuery(c.profile())
		if search.FilterFromValues(v) == nil {
			t.Errorf("%s: FilterFromValues(%s) = nil", c.Name, v.Encode())
		}
		if unknown := search.UnknownParams(v, nil); len(unknown) > 0 {
			t.Errorf("%s: params the search does not read: %v", c.Name, unknown)
		}
	}
}
