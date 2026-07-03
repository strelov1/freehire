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
	"github.com/strelov1/freehire/internal/userprofile"
)

// fakeProfileRepo is a userprofile.Repository that returns canned rows and records the
// upsert params — enough to exercise the handlers' request parsing and the
// specialization/skills validation (which reject before Upsert is reached) without a
// database. Shared by the profile, verdict, and ATS handler tests. The DB-backed contract
// is covered by the service's own tests.
type fakeProfileRepo struct {
	getRet    db.UserProfile
	getErr    error
	upsertRet db.UserProfile
	upserted  db.UpsertUserProfileParams
	delErr    error
}

func (f *fakeProfileRepo) Get(context.Context, int64) (db.UserProfile, error) {
	return f.getRet, f.getErr
}
func (f *fakeProfileRepo) Upsert(_ context.Context, p db.UpsertUserProfileParams) (db.UserProfile, error) {
	f.upserted = p
	return f.upsertRet, nil
}
func (f *fakeProfileRepo) Delete(context.Context, int64) error { return f.delErr }

// profileApp mounts the singleton profile endpoints behind RequireAuth on a handler whose
// user-profile service is backed by the given in-memory fake repo.
func profileApp(t *testing.T, repo *fakeProfileRepo) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss, userProfile: userprofile.New(repo)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss)
	app.Get("/me/profile", g, h.GetProfile)
	app.Put("/me/profile", g, h.PutProfile)
	app.Delete("/me/profile", g, h.DeleteProfile)
	return app, token
}

func doProfile(t *testing.T, app *fiber.App, method, body, token string) *http.Response {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/me/profile", nil)
	} else {
		r = httptest.NewRequest(method, "/me/profile", strings.NewReader(body))
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
		{"empty specializations", `{"specializations":[],"skills":["go"]}`, fiber.StatusBadRequest},
		{"unknown specialization", `{"specializations":["wizardry"],"skills":["go"]}`, fiber.StatusBadRequest},
		{"too many specializations", `{"specializations":["backend","frontend","fullstack","mobile","devops","sre"],"skills":["go"]}`, fiber.StatusBadRequest},
		{"empty skills", `{"specializations":["backend"],"skills":[]}`, fiber.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeProfileRepo{}
			app, token := profileApp(t, repo)
			resp := doProfile(t, app, fiber.MethodPut, tc.body, token)
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
	ret := db.UserProfile{UserID: 1, Specializations: []string{"backend", "devops"}, Skills: []string{"go"}}
	app, token := profileApp(t, &fakeProfileRepo{upsertRet: ret})
	resp := doProfile(t, app, fiber.MethodPut, `{"specializations":["backend","devops"],"skills":["go"]}`, token)
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

func TestGetProfile_NullWhenNone(t *testing.T) {
	app, token := profileApp(t, &fakeProfileRepo{getErr: userprofile.ErrNotFound})
	resp := doProfile(t, app, fiber.MethodGet, "", token)
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
	ret := db.UserProfile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}}
	app, token := profileApp(t, &fakeProfileRepo{getRet: ret})
	resp := doProfile(t, app, fiber.MethodGet, "", token)
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

func TestDeleteProfile_Idempotent(t *testing.T) {
	// No stored profile: the fake's Delete returns nil, and the handler still answers 204.
	app, token := profileApp(t, &fakeProfileRepo{})
	resp := doProfile(t, app, fiber.MethodDelete, "", token)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}
