package userprofile_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/userprofile"
)

// decodeLoc unmarshals the location block the fake repo captured.
func decodeLoc(t *testing.T, raw json.RawMessage) userprofile.LocationPreferences {
	t.Helper()
	var got userprofile.LocationPreferences
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal location_preferences: %v", err)
	}
	return got
}

// The Florianópolis case: remote for LATAM, on-site at home in BR, willing to relocate to
// Berlin — all at once. The block round-trips normalized and whole.
func TestSave_StoresCombinedLocationPreferences(t *testing.T) {
	repo := &fakeRepo{}
	svc := userprofile.New(repo)

	loc := &userprofile.LocationPreferences{
		WorkModes:  []string{"remote", "onsite"},
		Remote:     userprofile.GeoSet{Regions: []string{"latam"}},
		Base:       userprofile.BaseLocation{Country: "BR", City: " Florianópolis "},
		Relocation: userprofile.Relocation{Open: true, Cities: []string{"Berlin"}},
	}
	if _, err := svc.Save(context.Background(), 7, []string{"backend"}, []string{"go"}, loc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("repo.Upsert was not called")
	}
	got := decodeLoc(t, repo.upserted.LocationPreferences)
	if len(got.WorkModes) != 2 || got.WorkModes[0] != "remote" || got.WorkModes[1] != "onsite" {
		t.Errorf("WorkModes = %v", got.WorkModes)
	}
	if len(got.Remote.Regions) != 1 || got.Remote.Regions[0] != "latam" {
		t.Errorf("Remote.Regions = %v", got.Remote.Regions)
	}
	if got.Base.Country != "br" { // lowercased
		t.Errorf("Base.Country = %q, want br", got.Base.Country)
	}
	if got.Base.City != "Florianópolis" { // trimmed, case preserved
		t.Errorf("Base.City = %q", got.Base.City)
	}
	if !got.Relocation.Open || len(got.Relocation.Cities) != 1 || got.Relocation.Cities[0] != "Berlin" {
		t.Errorf("Relocation = %+v", got.Relocation)
	}
}

// No location block → NULL stored (never-set semantics), profile still saved.
func TestSave_NilLocationStoresNull(t *testing.T) {
	repo := &fakeRepo{}
	if _, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend"}, []string{"go"}, nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.upserted.LocationPreferences != nil {
		t.Errorf("LocationPreferences = %s, want nil", repo.upserted.LocationPreferences)
	}
}

// An entirely-empty block collapses to NULL — no meaningless {} row.
func TestSave_EmptyLocationCollapsesToNull(t *testing.T) {
	repo := &fakeRepo{}
	empty := &userprofile.LocationPreferences{}
	if _, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend"}, []string{"go"}, empty); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.upserted.LocationPreferences != nil {
		t.Errorf("LocationPreferences = %s, want nil", repo.upserted.LocationPreferences)
	}
}

// Out-of-vocabulary values reject the whole save; nothing is persisted.
func TestSave_RejectsInvalidLocationValues(t *testing.T) {
	cases := []struct {
		name string
		loc  *userprofile.LocationPreferences
		want error
	}{
		{"bad work mode", &userprofile.LocationPreferences{WorkModes: []string{"freelance"}}, userprofile.ErrInvalidWorkMode},
		{"bad region", &userprofile.LocationPreferences{Remote: userprofile.GeoSet{Regions: []string{"antarctica"}}}, userprofile.ErrInvalidRegion},
		{"malformed remote country", &userprofile.LocationPreferences{Remote: userprofile.GeoSet{Countries: []string{"usa"}}}, userprofile.ErrInvalidCountry},
		{"malformed base country", &userprofile.LocationPreferences{Base: userprofile.BaseLocation{Country: "usa"}}, userprofile.ErrInvalidCountry},
		{"bad relocation region", &userprofile.LocationPreferences{Relocation: userprofile.Relocation{Regions: []string{"mars"}}}, userprofile.ErrInvalidRegion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend"}, []string{"go"}, tc.loc)
			if !errors.Is(err, tc.want) {
				t.Errorf("Save err = %v, want %v", err, tc.want)
			}
			if repo.upsertCalled {
				t.Error("repo.Upsert should not be called on invalid input")
			}
		})
	}
}

// Work modes and regions are matched case-insensitively (lowercased) and deduped, so a
// non-lowercase or mixed-case API client is accepted, not rejected.
func TestSave_NormalizesLocationEnumCase(t *testing.T) {
	repo := &fakeRepo{}
	loc := &userprofile.LocationPreferences{
		WorkModes: []string{"Remote", "remote"},
		Remote:    userprofile.GeoSet{Regions: []string{"LATAM", "latam"}},
	}
	if _, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend"}, []string{"go"}, loc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := decodeLoc(t, repo.upserted.LocationPreferences)
	if len(got.WorkModes) != 1 || got.WorkModes[0] != "remote" {
		t.Errorf("WorkModes = %v, want [remote]", got.WorkModes)
	}
	if len(got.Remote.Regions) != 1 || got.Remote.Regions[0] != "latam" {
		t.Errorf("Remote.Regions = %v, want [latam]", got.Remote.Regions)
	}
}

// Countries lowercased + deduped; cities trimmed, blanks dropped, deduped.
func TestSave_NormalizesLocationCountriesAndCities(t *testing.T) {
	repo := &fakeRepo{}
	loc := &userprofile.LocationPreferences{
		Remote: userprofile.GeoSet{Countries: []string{"BR", "br", "US"}},
		Relocation: userprofile.Relocation{
			Open:   true,
			Cities: []string{" Berlin ", "berlin ", "", "Lisbon"},
		},
	}
	if _, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend"}, []string{"go"}, loc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := decodeLoc(t, repo.upserted.LocationPreferences)
	if len(got.Remote.Countries) != 2 || got.Remote.Countries[0] != "br" || got.Remote.Countries[1] != "us" {
		t.Errorf("Remote.Countries = %v, want [br us]", got.Remote.Countries)
	}
	// "Berlin" and "berlin" differ only by case/space → deduped to one; blank dropped.
	if len(got.Relocation.Cities) != 2 || got.Relocation.Cities[0] != "Berlin" || got.Relocation.Cities[1] != "Lisbon" {
		t.Errorf("Relocation.Cities = %v, want [Berlin Lisbon]", got.Relocation.Cities)
	}
}
