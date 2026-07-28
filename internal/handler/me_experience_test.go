package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/experience"
)

// experienceApp wires the bank surface over an in-memory repository, so the routes are
// exercised end to end without a database.
func experienceApp(t *testing.T) (*fiber.App, string, *stubBank) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	bank := newStubBank()
	h := newExperienceHandlers(bank)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	guard := auth.RequireAuth(iss, testVersions)
	h.register(app.Group(""), middleware{cookie: guard, key: guard})
	return app, token, bank
}

func experienceReq(t *testing.T, app *fiber.App, method, path, body, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// The surface exists so a person can see what was recorded about them — including, and
// especially, what they never wrote down themselves.
func TestListExperienceGroupsAchievementsUnderTheirPlace(t *testing.T) {
	app, token, bank := experienceApp(t)

	role := bank.addEmployment(1, experience.Employment{
		Kind: experience.KindJob, Company: "RingCentral", Role: "SWE",
	})
	bank.add(1, experience.Atom{
		EmploymentID: &role.ID, Claim: "Cut latency 20s to 1s", Provenance: experience.ProvenanceCVImport,
	})
	bank.add(1, experience.Atom{
		Claim: "Probably led the migration", Provenance: experience.ProvenanceAgentInferred,
	})
	bank.reindex()

	resp := experienceReq(t, app, http.MethodGet, "/me/experience", "", token)
	var body struct {
		Data struct {
			Employments []struct {
				Company string `json:"company"`
				Atoms   []struct {
					Claim      string `json:"claim"`
					Provenance string `json:"provenance"`
				} `json:"atoms"`
			} `json:"employments"`
			Unplaced []struct {
				Claim      string `json:"claim"`
				Provenance string `json:"provenance"`
			} `json:"unplaced"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	if len(body.Data.Employments) != 1 || len(body.Data.Employments[0].Atoms) != 1 {
		t.Fatalf("employments = %+v, want one role carrying one achievement", body.Data.Employments)
	}
	// Provenance is served on every entry: the distinction the model cannot fake has to be
	// visible to the person it describes.
	if body.Data.Employments[0].Atoms[0].Provenance != "cv_import" {
		t.Errorf("provenance = %q, want it served", body.Data.Employments[0].Atoms[0].Provenance)
	}
	// Placeless evidence is shown, not hidden — it is usually what was volunteered in
	// conversation, which makes it the most important thing to be able to check.
	if len(body.Data.Unplaced) != 1 || body.Data.Unplaced[0].Provenance != "agent_inferred" {
		t.Errorf("unplaced = %+v, want the agent's own entry visible and flagged", body.Data.Unplaced)
	}
}

// Correcting what the agent inferred is how a person confirms it. Typing it makes it
// theirs, so it becomes publishable — that is the whole confirmation path.
func TestUpdateAtomMakesItTheOwnersOwnStatement(t *testing.T) {
	app, token, bank := experienceApp(t)
	ctx := t.Context()

	inferred := bank.add(1, experience.Atom{
		Claim: "Probably led the Kubernetes migration", Provenance: experience.ProvenanceAgentInferred,
	})

	resp := experienceReq(t, app, http.MethodPut, "/me/experience/atoms/"+inferred.ID.String(),
		`{"claim":"Led the Kubernetes migration for 21 services"}`, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	updated, err := bank.GetAtom(ctx, inferred.ID, 1)
	if err != nil {
		t.Fatalf("GetAtom: %v", err)
	}
	if !updated.Provenance.Publishable() {
		t.Errorf("provenance = %q, want the owner's edit to make it theirs", updated.Provenance)
	}
	if updated.Claim != "Led the Kubernetes migration for 21 services" {
		t.Errorf("claim = %q, want the correction", updated.Claim)
	}
}

// Deletion is the only path that takes evidence out of the bank, and it belongs to the
// owner alone.
func TestDeleteExperienceIsOwnerScoped(t *testing.T) {
	app, token, bank := experienceApp(t)
	ctx := t.Context()

	mine := bank.add(1, experience.Atom{Claim: "Mine", Provenance: experience.ProvenanceManual})
	theirs := bank.add(2, experience.Atom{Claim: "Theirs", Provenance: experience.ProvenanceManual})

	if resp := experienceReq(t, app, http.MethodDelete, "/me/experience/atoms/"+theirs.ID.String(), "", token); resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting another owner's achievement = %d, want 404 (never 403)", resp.StatusCode)
	}
	if _, err := bank.GetAtom(ctx, theirs.ID, 2); err != nil {
		t.Errorf("another owner's achievement was removed: %v", err)
	}

	if resp := experienceReq(t, app, http.MethodDelete, "/me/experience/atoms/"+mine.ID.String(), "", token); resp.StatusCode != http.StatusNoContent {
		t.Errorf("deleting my own achievement = %d, want 204", resp.StatusCode)
	}
	if _, err := bank.GetAtom(ctx, mine.ID, 1); err == nil {
		t.Error("the achievement survived its own deletion")
	}
}

func TestExperienceRoutesRefuseAMalformedID(t *testing.T) {
	app, token, _ := experienceApp(t)

	for _, path := range []string{
		"/me/experience/atoms/not-a-uuid",
		"/me/experience/employments/not-a-uuid",
	} {
		if resp := experienceReq(t, app, http.MethodDelete, path, "", token); resp.StatusCode != http.StatusNotFound {
			t.Errorf("DELETE %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestExperienceRoutesRequireAuth(t *testing.T) {
	app, _, _ := experienceApp(t)

	if resp := experienceReq(t, app, http.MethodGet, "/me/experience", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated read = %d, want 401", resp.StatusCode)
	}
	if resp := experienceReq(t, app, http.MethodDelete, "/me/experience/atoms/"+uuid.New().String(), "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated delete = %d, want 401", resp.StatusCode)
	}
}

// Nothing may be invisible on the page whose purpose is that nothing is invisible. An
// achievement whose place is unknown is shown as unplaced rather than filed under a
// heading that never renders — otherwise the owner loses something they cannot even delete.
func TestListExperienceShowsAnAchievementWithAnUnknownPlace(t *testing.T) {
	app, token, bank := experienceApp(t)

	ghost := uuid.New()
	bank.add(1, experience.Atom{
		EmploymentID: &ghost, Claim: "Ran the cluster", Provenance: experience.ProvenanceManual,
	})
	bank.reindex()

	resp := experienceReq(t, app, http.MethodGet, "/me/experience", "", token)
	var body struct {
		Data struct {
			Unplaced []struct {
				Claim string `json:"claim"`
			} `json:"unplaced"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	if len(body.Data.Unplaced) != 1 || body.Data.Unplaced[0].Claim != "Ran the cluster" {
		t.Errorf("unplaced = %+v, want the achievement visible despite its unknown place", body.Data.Unplaced)
	}
}
