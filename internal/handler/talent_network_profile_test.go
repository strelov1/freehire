package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// fakeTalentNetworkPublicStore is a talentNetworkPublicStore backed by an in-memory row,
// enough to exercise the public handler's request parsing and branching without a
// database.
type fakeTalentNetworkPublicStore struct {
	row      db.GetTalentNetworkProfileByPublicIDRow
	err      error
	gotID    uuid.UUID
	getCalls int
}

func (f *fakeTalentNetworkPublicStore) GetTalentNetworkProfileByPublicID(_ context.Context, id uuid.UUID) (db.GetTalentNetworkProfileByPublicIDRow, error) {
	f.getCalls++
	f.gotID = id
	if f.err != nil {
		return db.GetTalentNetworkProfileByPublicIDRow{}, f.err
	}
	return f.row, nil
}

func talentNetworkProfileApp(store *fakeTalentNetworkPublicStore) *fiber.App {
	h := newTalentNetworkProfileHandlers(store)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/talent-network/:publicID", h.GetProfile)
	return app
}

func doTalentNetworkProfile(t *testing.T, app *fiber.App, publicID string) *http.Response {
	t.Helper()
	r := httptest.NewRequest(fiber.MethodGet, "/talent-network/"+publicID, nil)
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func talentNetworkReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

func forbidSubstrings(t *testing.T, body string, forbidden ...string) {
	t.Helper()
	for _, s := range forbidden {
		if strings.Contains(body, s) {
			t.Errorf("body must not contain %q: %s", s, body)
		}
	}
}

func assertNotFoundBody(t *testing.T, resp *http.Response) {
	t.Helper()
	var got struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Error != "not found" {
		t.Errorf("error = %q, want %q", got.Error, "not found")
	}
}

func TestTalentNetworkProfile_PublicMode(t *testing.T) {
	id := uuid.New()
	structured := resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+1-555-0100",
		Links:    []string{"https://linkedin.com/in/ada"},
		Skills:   []string{"go", "algorithms"},
		Experience: []resumeextract.Experience{
			{Company: "Analytical Engines Inc", Title: "Engineer", End: "2020-01"},
		},
	}
	store := &fakeTalentNetworkPublicStore{row: db.GetTalentNetworkProfileByPublicIDRow{
		TalentNetworkVisibility: "public",
		ResumeStructured:        mustMarshal(t, structured),
		Specializations:         []string{"backend"},
		Skills:                  []string{"go"},
	}}
	app := talentNetworkProfileApp(store)

	resp := doTalentNetworkProfile(t, app, id.String())
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if store.gotID != id {
		t.Errorf("store called with id %v, want %v", store.gotID, id)
	}

	body := talentNetworkReadBody(t, resp)
	if !strings.Contains(body, "Ada Lovelace") || !strings.Contains(body, "Analytical Engines Inc") {
		t.Errorf("body missing expected public-mode content: %s", body)
	}
	forbidSubstrings(t, body, "ada@example.com", "+1-555-0100", "linkedin.com/in/ada")
}

func TestTalentNetworkProfile_AnonymousMode(t *testing.T) {
	id := uuid.New()
	structured := resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+1-555-0100",
		Links:    []string{"https://linkedin.com/in/ada"},
		Experience: []resumeextract.Experience{
			{Company: "Current Corp", Title: "Staff Engineer", End: ""},
			{Company: "Past Co", Title: "Engineer", End: "2019-06"},
		},
	}
	store := &fakeTalentNetworkPublicStore{row: db.GetTalentNetworkProfileByPublicIDRow{
		TalentNetworkVisibility: "anonymous",
		ResumeStructured:        mustMarshal(t, structured),
	}}
	app := talentNetworkProfileApp(store)

	resp := doTalentNetworkProfile(t, app, id.String())
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body := talentNetworkReadBody(t, resp)
	if strings.Contains(body, "Ada Lovelace") {
		t.Errorf("anonymous-mode body must not contain the candidate's name: %s", body)
	}
	if strings.Contains(body, "Current Corp") {
		t.Errorf("anonymous-mode body must mask the current employer: %s", body)
	}
	if !strings.Contains(body, "Current employer") || !strings.Contains(body, "Past Co") {
		t.Errorf("anonymous-mode body must show the masking label and the older employer unmodified: %s", body)
	}
	forbidSubstrings(t, body, "ada@example.com", "+1-555-0100", "linkedin.com/in/ada")
}

func TestTalentNetworkProfile_VisibilityOff(t *testing.T) {
	store := &fakeTalentNetworkPublicStore{row: db.GetTalentNetworkProfileByPublicIDRow{TalentNetworkVisibility: "off"}}
	app := talentNetworkProfileApp(store)

	resp := doTalentNetworkProfile(t, app, uuid.New().String())
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	assertNotFoundBody(t, resp)
}

func TestTalentNetworkProfile_NonexistentID(t *testing.T) {
	store := &fakeTalentNetworkPublicStore{err: pgx.ErrNoRows}
	app := talentNetworkProfileApp(store)

	resp := doTalentNetworkProfile(t, app, uuid.New().String())
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	assertNotFoundBody(t, resp)
}

func TestTalentNetworkProfile_MalformedID(t *testing.T) {
	store := &fakeTalentNetworkPublicStore{row: db.GetTalentNetworkProfileByPublicIDRow{TalentNetworkVisibility: "public"}}
	app := talentNetworkProfileApp(store)

	resp := doTalentNetworkProfile(t, app, "not-a-uuid")
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	assertNotFoundBody(t, resp)
	if store.getCalls != 0 {
		t.Error("store should not be queried for a malformed id")
	}
}

func TestTalentNetworkProfile_OffAndNotFoundHaveIdenticalBody(t *testing.T) {
	offStore := &fakeTalentNetworkPublicStore{row: db.GetTalentNetworkProfileByPublicIDRow{TalentNetworkVisibility: "off"}}
	offResp := doTalentNetworkProfile(t, talentNetworkProfileApp(offStore), uuid.New().String())
	offBody := talentNetworkReadBody(t, offResp)

	missingStore := &fakeTalentNetworkPublicStore{err: pgx.ErrNoRows}
	missingResp := doTalentNetworkProfile(t, talentNetworkProfileApp(missingStore), uuid.New().String())
	missingBody := talentNetworkReadBody(t, missingResp)

	malformedStore := &fakeTalentNetworkPublicStore{}
	malformedResp := doTalentNetworkProfile(t, talentNetworkProfileApp(malformedStore), "not-a-uuid")
	malformedBody := talentNetworkReadBody(t, malformedResp)

	if offBody != missingBody || offBody != malformedBody {
		t.Errorf("expected identical bodies, got off=%q missing=%q malformed=%q", offBody, missingBody, malformedBody)
	}
}

func TestTalentNetworkProfile_EmptyResumeStructuredRendersEmptySections(t *testing.T) {
	for _, visibility := range []string{"public", "anonymous"} {
		t.Run(visibility, func(t *testing.T) {
			store := &fakeTalentNetworkPublicStore{row: db.GetTalentNetworkProfileByPublicIDRow{
				TalentNetworkVisibility: visibility,
				ResumeStructured:        nil,
			}}
			app := talentNetworkProfileApp(store)

			resp := doTalentNetworkProfile(t, app, uuid.New().String())
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			var got struct {
				Data talentNetworkProfileResponse `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got.Data.CV.Experience) != 0 {
				t.Errorf("experience = %v, want empty", got.Data.CV.Experience)
			}
			if len(got.Data.CV.Skills) != 0 {
				t.Errorf("skills = %v, want empty", got.Data.CV.Skills)
			}
		})
	}
}
