package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// fakeProfileRepo is a userprofile.Repository that returns canned rows and records the
// upsert params — enough to exercise the handlers' request parsing and the
// specialization/skills validation (which reject before Upsert is reached) without a
// database. Shared by the profile, verdict, and ATS handler tests. The DB-backed contract
// is covered by the service's own tests.
type profileUpsert struct {
	UserID              int64
	Specializations     []string
	Skills              []string
	Seniorities         []string
	ExcludedSkills      []string
	ExcludedSources     []string
	ExcludedCompanies   []string
	LocationPreferences json.RawMessage
}

type fakeProfileRepo struct {
	getRet    userprofile.Profile
	getErr    error
	upsertRet userprofile.Profile
	upserted  profileUpsert
	delErr    error
}

func (f *fakeProfileRepo) Get(context.Context, int64) (userprofile.Profile, error) {
	return f.getRet, f.getErr
}
func (f *fakeProfileRepo) Upsert(_ context.Context, userID int64, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies []string, locationPreferences json.RawMessage) (userprofile.Profile, error) {
	f.upserted = profileUpsert{UserID: userID, Specializations: specializations, Skills: skills, Seniorities: seniorities, ExcludedSkills: excludedSkills, ExcludedSources: excludedSources, ExcludedCompanies: excludedCompanies, LocationPreferences: locationPreferences}
	return f.upsertRet, nil
}

// UpsertIfUnchanged is exercised by userprofile's own tests (MergeSkills' retry loop);
// no handler here calls MergeSkills, so this fake just satisfies the interface.
func (f *fakeProfileRepo) UpsertIfUnchanged(_ context.Context, userID int64, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies []string, locationPreferences json.RawMessage, _ time.Time) (userprofile.Profile, error) {
	f.upserted = profileUpsert{UserID: userID, Specializations: specializations, Skills: skills, Seniorities: seniorities, ExcludedSkills: excludedSkills, ExcludedSources: excludedSources, ExcludedCompanies: excludedCompanies, LocationPreferences: locationPreferences}
	return f.upsertRet, nil
}
func (f *fakeProfileRepo) Delete(context.Context, int64) error { return f.delErr }

// profileApp mounts the singleton profile endpoints behind RequireAuth on a handler whose
// user-profile service is backed by the given in-memory fake repo.
func profileApp(t *testing.T, repo *fakeProfileRepo) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &profileHandlers{userProfile: userprofile.New(repo)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Get("/me/profile", g, h.GetProfile)
	app.Put("/me/profile", g, h.PutProfile)
	app.Delete("/me/profile", g, h.DeleteProfile)
	return app, token
}

func doProfile(t *testing.T, app *fiber.App, method, body, token string) *http.Response {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequestWithContext(context.Background(), method, "/me/profile", nil)
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, "/me/profile", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestPutProfile_Unauthenticated(t *testing.T) {
	app, _ := profileApp(t, &fakeProfileRepo{})
	resp := doProfile(t, app, fiber.MethodPut, `{"specializations":["backend"],"skills":["go"]}`, "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPutProfile_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"missing lists", `{}`, fiber.StatusBadRequest},
		{"missing skills", `{"specializations":[]}`, fiber.StatusBadRequest},
		{"missing specializations", `{"skills":[]}`, fiber.StatusBadRequest},
		{"null skills", `{"specializations":[],"skills":null}`, fiber.StatusBadRequest},
		{"null specializations", `{"specializations":null,"skills":[]}`, fiber.StatusBadRequest},
		{"unknown specialization", `{"specializations":["wizardry"],"skills":["go"]}`, fiber.StatusBadRequest},
		// One past userprofile.MaxSpecializations. The set below the cap is exercised by
		// TestPutProfile_AcceptsSpecializationsUpToTheCap — a cap only holds if both sides
		// of it are checked.
		{"too many specializations", `{"specializations":["backend","frontend","fullstack","mobile","devops","sre","qa","security","hardware","embedded","blockchain"],"skills":["go"]}`, fiber.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeProfileRepo{}
			app, token := profileApp(t, repo)
			resp := doProfile(t, app, fiber.MethodPut, tc.body, token)
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			if repo.upserted.UserID != 0 {
				t.Error("repo.Upsert should not be called on invalid input")
			}
		})
	}
}

func TestPutProfile_ReturnsSpecializationsArray(t *testing.T) {
	ret := userprofile.Profile{UserID: 1, Specializations: []string{"backend", "devops"}, Skills: []string{"go"}}
	app, token := profileApp(t, &fakeProfileRepo{upsertRet: ret})
	resp := doProfile(t, app, fiber.MethodPut, `{"specializations":["backend","devops"],"skills":["go"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data profileResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Join(got.Data.Specializations, ",") != "backend,devops" {
		t.Errorf("specializations = %v, want [backend devops]", got.Data.Specializations)
	}
}

// A profile filled from a CV routinely resolves more than a handful of categories, and the
// cap that used to sit at 5 rejected the whole save when it did. Exactly MaxSpecializations
// must be accepted, or the bound is really one lower than it says.
func TestPutProfile_AcceptsSpecializationsUpToTheCap(t *testing.T) {
	specs := []string{"backend", "frontend", "fullstack", "mobile", "devops", "sre", "qa", "security", "hardware", "embedded"}
	if len(specs) != userprofile.MaxSpecializations {
		t.Fatalf("test fixture holds %d specializations, want MaxSpecializations = %d", len(specs), userprofile.MaxSpecializations)
	}
	repo := &fakeProfileRepo{}
	app, token := profileApp(t, repo)
	body, err := json.Marshal(map[string]any{"specializations": specs, "skills": []string{"go"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := doProfile(t, app, fiber.MethodPut, string(body), token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(repo.upserted.Specializations) != len(specs) {
		t.Errorf("specializations upserted = %v, want all %d", repo.upserted.Specializations, len(specs))
	}
}

func TestPutProfile_NormalizesExcludedSkillsAndDropsOverlap(t *testing.T) {
	repo := &fakeProfileRepo{}
	app, token := profileApp(t, repo)
	// "php" is wanted and also listed as excluded → the service drops it from the
	// excluded set; "WordPress" is normalized to "wordpress".
	resp := doProfile(t, app, fiber.MethodPut,
		`{"specializations":["backend"],"skills":["go","php"],"excluded_skills":["php","WordPress"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if strings.Join(repo.upserted.ExcludedSkills, ",") != "wordpress" {
		t.Errorf("excluded_skills upserted = %v, want [wordpress] (overlap dropped, normalized)", repo.upserted.ExcludedSkills)
	}
}

func TestPutProfile_EchoesExcludedSkills(t *testing.T) {
	ret := userprofile.Profile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}, ExcludedSkills: []string{"wordpress"}}
	app, token := profileApp(t, &fakeProfileRepo{upsertRet: ret})
	resp := doProfile(t, app, fiber.MethodPut,
		`{"specializations":["backend"],"skills":["go"],"excluded_skills":["wordpress"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data profileResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Join(got.Data.ExcludedSkills, ",") != "wordpress" {
		t.Errorf("excluded_skills = %v, want [wordpress]", got.Data.ExcludedSkills)
	}
}

func TestPutProfile_SavesAndEchoesExcludedSourcesAndCompanies(t *testing.T) {
	ret := userprofile.Profile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}, ExcludedSources: []string{"greenhouse"}, ExcludedCompanies: []string{"acme"}}
	repo := &fakeProfileRepo{upsertRet: ret}
	app, token := profileApp(t, repo)
	resp := doProfile(t, app, fiber.MethodPut,
		`{"specializations":["backend"],"skills":["go"],"excluded_sources":["Greenhouse"],"excluded_companies":["Acme"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if strings.Join(repo.upserted.ExcludedSources, ",") != "greenhouse" {
		t.Errorf("excluded_sources upserted = %v, want [greenhouse] (normalized)", repo.upserted.ExcludedSources)
	}
	if strings.Join(repo.upserted.ExcludedCompanies, ",") != "acme" {
		t.Errorf("excluded_companies upserted = %v, want [acme] (normalized)", repo.upserted.ExcludedCompanies)
	}
	var got struct {
		Data profileResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Join(got.Data.ExcludedSources, ",") != "greenhouse" {
		t.Errorf("excluded_sources in response = %v, want [greenhouse]", got.Data.ExcludedSources)
	}
	if strings.Join(got.Data.ExcludedCompanies, ",") != "acme" {
		t.Errorf("excluded_companies in response = %v, want [acme]", got.Data.ExcludedCompanies)
	}
}

func TestPutProfile_RejectsTooManyExcludedSources(t *testing.T) {
	repo := &fakeProfileRepo{}
	app, token := profileApp(t, repo)
	sources := make([]string, 201)
	for i := range sources {
		sources[i] = "source" + strconv.Itoa(i)
	}
	body, err := json.Marshal(map[string]any{"specializations": []string{"backend"}, "skills": []string{"go"}, "excluded_sources": sources})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := doProfile(t, app, fiber.MethodPut, string(body), token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.upserted.UserID != 0 {
		t.Error("repo.Upsert should not be called past the excluded-sources cap")
	}
}

func TestPutProfile_RejectsTooManyExcludedCompanies(t *testing.T) {
	repo := &fakeProfileRepo{}
	app, token := profileApp(t, repo)
	companies := make([]string, 201)
	for i := range companies {
		companies[i] = "company" + strconv.Itoa(i)
	}
	body, err := json.Marshal(map[string]any{"specializations": []string{"backend"}, "skills": []string{"go"}, "excluded_companies": companies})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := doProfile(t, app, fiber.MethodPut, string(body), token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.upserted.UserID != 0 {
		t.Error("repo.Upsert should not be called past the excluded-companies cap")
	}
}

func TestPutProfile_ParsesAndEchoesLocationPreferences(t *testing.T) {
	loc := `{"work_modes":["remote","onsite"],"remote":{"regions":["latam"]},"base":{"country":"BR","city":"Florianópolis"},"relocation":{"open":true,"cities":["Berlin"]}}`
	repo := &fakeProfileRepo{}
	// Echo back whatever the service upserts, so the response reflects the stored block.
	app, token := profileApp(t, repo)
	resp := doProfile(t, app, fiber.MethodPut,
		`{"specializations":["backend"],"skills":["go"],"location_preferences":`+loc+`}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	// The nested block reached the service and was normalized into the upsert params.
	if repo.upserted.LocationPreferences == nil {
		t.Fatal("location_preferences did not reach the upsert")
	}
	var stored userprofile.LocationPreferences
	if err := json.Unmarshal(repo.upserted.LocationPreferences, &stored); err != nil {
		t.Fatalf("unmarshal upserted location: %v", err)
	}
	if stored.Base.Country != "br" || stored.Base.City != "Florianópolis" {
		t.Errorf("base = %+v, want {br Florianópolis}", stored.Base)
	}
	if !stored.Relocation.Open || len(stored.Relocation.Cities) != 1 {
		t.Errorf("relocation = %+v", stored.Relocation)
	}
}

func TestPutProfile_RejectsInvalidLocationBlock(t *testing.T) {
	repo := &fakeProfileRepo{}
	app, token := profileApp(t, repo)
	resp := doProfile(t, app, fiber.MethodPut,
		`{"specializations":["backend"],"skills":["go"],"location_preferences":{"work_modes":["freelance"]}}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.upserted.UserID != 0 {
		t.Error("repo.Upsert should not be called on an invalid location block")
	}
}

func TestGetProfile_EchoesStoredLocationPreferences(t *testing.T) {
	stored := json.RawMessage(`{"work_modes":["remote"],"remote":{"regions":["latam"]}}`)
	ret := userprofile.Profile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}, LocationPreferences: stored}
	app, token := profileApp(t, &fakeProfileRepo{getRet: ret})
	resp := doProfile(t, app, fiber.MethodGet, "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data profileResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var loc userprofile.LocationPreferences
	if err := json.Unmarshal(got.Data.LocationPreferences, &loc); err != nil {
		t.Fatalf("unmarshal response location: %v", err)
	}
	if len(loc.WorkModes) != 1 || loc.WorkModes[0] != "remote" {
		t.Errorf("work_modes = %v, want [remote]", loc.WorkModes)
	}
}

func TestGetProfile_NullWhenNone(t *testing.T) {
	app, token := profileApp(t, &fakeProfileRepo{getErr: userprofile.ErrNotFound})
	resp := doProfile(t, app, fiber.MethodGet, "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, ok := got["data"]; !ok || v != nil {
		t.Errorf("data = %v, want null", got["data"])
	}
}

func TestGetProfile_ReturnsProfile(t *testing.T) {
	ret := userprofile.Profile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}}
	app, token := profileApp(t, &fakeProfileRepo{getRet: ret})
	resp := doProfile(t, app, fiber.MethodGet, "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data profileResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Join(got.Data.Skills, ",") != "go" {
		t.Errorf("skills = %v, want [go]", got.Data.Skills)
	}
}

func TestPutProfile_SavesAndEchoesSeniorities(t *testing.T) {
	ret := userprofile.Profile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}, Seniorities: []string{"middle", "senior"}}
	repo := &fakeProfileRepo{upsertRet: ret}
	app, token := profileApp(t, repo)
	resp := doProfile(t, app, fiber.MethodPut,
		`{"specializations":["backend"],"skills":["go"],"seniorities":["middle","senior"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if strings.Join(repo.upserted.Seniorities, ",") != "middle,senior" {
		t.Errorf("seniorities upserted = %v, want [middle senior]", repo.upserted.Seniorities)
	}
	var got struct {
		Data profileResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Join(got.Data.Seniorities, ",") != "middle,senior" {
		t.Errorf("seniorities in response = %v, want [middle senior]", got.Data.Seniorities)
	}
}

func TestPutProfile_RejectsUnknownSeniority(t *testing.T) {
	repo := &fakeProfileRepo{}
	app, token := profileApp(t, repo)
	resp := doProfile(t, app, fiber.MethodPut,
		`{"specializations":["backend"],"skills":["go"],"seniorities":["wizard"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.upserted.UserID != 0 {
		t.Error("repo.Upsert should not be called on an unknown seniority")
	}
}

func TestPutProfile_OmittedSeniorityDefaultsToEmpty(t *testing.T) {
	repo := &fakeProfileRepo{upsertRet: userprofile.Profile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}}}
	app, token := profileApp(t, repo)
	resp := doProfile(t, app, fiber.MethodPut, `{"specializations":["backend"],"skills":["go"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if repo.upserted.Seniorities == nil {
		t.Error("seniorities upserted = nil, want non-nil empty set")
	}
	if len(repo.upserted.Seniorities) != 0 {
		t.Errorf("seniorities upserted = %v, want empty", repo.upserted.Seniorities)
	}
}

func TestDeleteProfile_Idempotent(t *testing.T) {
	// No stored profile: the fake's Delete returns nil, and the handler still answers 204.
	app, token := profileApp(t, &fakeProfileRepo{})
	resp := doProfile(t, app, fiber.MethodDelete, "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestPutProfile_AcceptsIncompleteProfile(t *testing.T) {
	for _, body := range []string{
		`{"specializations":[],"skills":[]}`,
		`{"specializations":["backend"],"skills":[]}`,
		`{"specializations":[],"skills":["go"]}`,
		`{"specializations":[],"skills":[],"location_preferences":{"work_modes":["remote"]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &fakeProfileRepo{}
			app, token := profileApp(t, repo)
			resp := doProfile(t, app, fiber.MethodPut, body, token)
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200 for incomplete profile", resp.StatusCode)
			}
			if repo.upserted.UserID == 0 {
				t.Fatal("incomplete profile was not persisted")
			}
		})
	}
}
