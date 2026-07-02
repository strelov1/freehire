package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/searchprofile"
)

// fakeProfileRepo is a searchprofile.Repository that returns a canned created row and
// records nothing else — enough to exercise the handler's request parsing and the
// specialization/skills validation (which reject before Create/Count are reached) without
// a database. The DB-backed contract is covered by the service's own tests.
type fakeProfileRepo struct {
	createRet db.SearchProfile
	updateRet db.SearchProfile
	updated   db.UpdateSearchProfileParams

	getRet db.SearchProfile
	getErr error
}

func (f *fakeProfileRepo) List(context.Context, int64) ([]db.SearchProfile, error) {
	return nil, nil
}
func (f *fakeProfileRepo) Count(context.Context, int64) (int64, error) { return 0, nil }
func (f *fakeProfileRepo) Create(context.Context, db.CreateSearchProfileParams) (db.SearchProfile, error) {
	return f.createRet, nil
}
func (f *fakeProfileRepo) Update(_ context.Context, p db.UpdateSearchProfileParams) (db.SearchProfile, error) {
	f.updated = p
	return f.updateRet, nil
}
func (f *fakeProfileRepo) Delete(context.Context, db.DeleteSearchProfileParams) error { return nil }
func (f *fakeProfileRepo) Get(_ context.Context, _ db.GetSearchProfileParams) (db.SearchProfile, error) {
	return f.getRet, f.getErr
}

// profilesApp mounts the create/update endpoints behind RequireAuth on a handler whose
// search-profile service is backed by the given in-memory fake repo.
func profilesApp(t *testing.T, repo *fakeProfileRepo) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss, searchProfile: searchprofile.New(repo)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/me/profiles", auth.RequireAuth(iss), h.CreateSearchProfile)
	app.Patch("/me/profiles/:id", auth.RequireAuth(iss), h.UpdateSearchProfile)
	return app, token
}

func patchProfile(t *testing.T, app *fiber.App, id, body, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPatch, "/me/profiles/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func postProfile(t *testing.T, app *fiber.App, body, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, "/me/profiles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestCreateSearchProfile_Unauthenticated(t *testing.T) {
	app, _ := profilesApp(t, &fakeProfileRepo{})
	resp := postProfile(t, app, `{"name":"X","specializations":["backend"],"skills":["go"]}`, "")
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestCreateSearchProfile_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty specializations", `{"name":"X","specializations":[],"skills":["go"]}`, fiber.StatusBadRequest},
		{"unknown specialization", `{"name":"X","specializations":["wizardry"],"skills":["go"]}`, fiber.StatusBadRequest},
		{"too many specializations", `{"name":"X","specializations":["backend","frontend","fullstack","mobile","devops","sre"],"skills":["go"]}`, fiber.StatusBadRequest},
		{"empty skills", `{"name":"X","specializations":["backend"],"skills":[]}`, fiber.StatusBadRequest},
		{"blank name", `{"name":"  ","specializations":["backend"],"skills":["go"]}`, fiber.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, token := profilesApp(t, &fakeProfileRepo{})
			resp := postProfile(t, app, tc.body, token)
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestCreateSearchProfile_ReturnsSpecializationsArray(t *testing.T) {
	ret := db.SearchProfile{ID: 42, Name: "Go backend", Specializations: []string{"backend", "devops"}, Skills: []string{"go"}}
	app, token := profilesApp(t, &fakeProfileRepo{createRet: ret})
	resp := postProfile(t, app, `{"name":"Go backend","specializations":["backend","devops"],"skills":["go"]}`, token)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data searchProfileResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Join(got.Data.Specializations, ",") != "backend,devops" {
		t.Errorf("specializations = %v, want [backend devops]", got.Data.Specializations)
	}
}

func TestUpdateSearchProfile_RejectsEmptySpecializations(t *testing.T) {
	repo := &fakeProfileRepo{}
	app, token := profilesApp(t, repo)
	resp := patchProfile(t, app, "5", `{"specializations":[]}`, token)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.updated.ID != 0 {
		t.Error("repo.Update should not be called on an empty specializations array")
	}
}

func TestUpdateSearchProfile_RenameOnly_LeavesArraysUnchanged(t *testing.T) {
	repo := &fakeProfileRepo{updateRet: db.SearchProfile{ID: 5, Name: "Renamed", Specializations: []string{"backend"}, Skills: []string{"go"}}}
	app, token := profilesApp(t, repo)
	resp := patchProfile(t, app, "5", `{"name":"Renamed"}`, token)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	// A rename-only PATCH leaves specializations/skills as nil params (SQL NULL →
	// COALESCE keeps the stored value), mirroring the pre-existing skills behavior.
	if repo.updated.Specializations != nil {
		t.Errorf("Specializations param = %v, want nil (unchanged)", repo.updated.Specializations)
	}
	if repo.updated.Skills != nil {
		t.Errorf("Skills param = %v, want nil (unchanged)", repo.updated.Skills)
	}
}
