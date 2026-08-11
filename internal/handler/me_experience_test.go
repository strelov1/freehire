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

// The only way in for a caller with no chat session — a script, freehire-cli — is this
// route, and it must land under the SAME caller the token names, the same as every other
// write in this file.
func TestAddAtomCreatesUnderTheCaller(t *testing.T) {
	app, token, bank := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms",
		`{"claim":"Cut latency 20s to 1s"}`, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data struct {
			ID    uuid.UUID `json:"id"`
			Claim string    `json:"claim"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data.Claim != "Cut latency 20s to 1s" {
		t.Errorf("claim = %q, want it echoed back", body.Data.Claim)
	}

	stored, err := bank.GetAtom(t.Context(), body.Data.ID, 1)
	if err != nil {
		t.Fatalf("the atom was not persisted under caller 1: %v", err)
	}
	if stored.Claim != "Cut latency 20s to 1s" {
		t.Errorf("stored claim = %q", stored.Claim)
	}
}

// There is no chat transcript to check a stated_in_chat claim against outside a chat
// turn, so a plain authenticated POST can only ever honestly produce manual — the same
// provenance PUT already forces on a correction.
func TestAddAtomForcesManualProvenance(t *testing.T) {
	app, token, bank := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms",
		`{"claim":"Led the migration","provenance":"stated_in_chat"}`, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	stored, err := bank.GetAtom(t.Context(), body.Data.ID, 1)
	if err != nil {
		t.Fatalf("GetAtom: %v", err)
	}
	if stored.Provenance != experience.ProvenanceManual {
		t.Errorf("provenance = %q, want it forced to manual regardless of what the caller sent", stored.Provenance)
	}
}

func TestAddAtomRefusesAnEmptyClaim(t *testing.T) {
	app, token, bank := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms", `{"claim":""}`, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
	if len(bank.list) != 0 {
		t.Error("a refused atom was persisted anyway")
	}
}

// AddAtom shares the store's dedup with experience_add: a claim already banked, under any
// spelling, must not silently create a second row nor a generic 500 — the caller needs to
// know it already exists.
func TestAddAtomOnAnAlreadyBankedClaimIsAConflict(t *testing.T) {
	app, token, bank := experienceApp(t)
	bank.addErr = experience.ErrAlreadyBanked

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms", `{"claim":"Cut latency 20s to 1s"}`, token)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAddEmploymentCreatesUnderTheCaller(t *testing.T) {
	app, token, bank := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/employments",
		`{"kind":"job","company":"RingCentral","role":"SWE"}`, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data struct {
			ID      uuid.UUID `json:"id"`
			Company string    `json:"company"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data.Company != "RingCentral" {
		t.Errorf("company = %q, want it echoed back", body.Data.Company)
	}

	found := false
	for _, e := range bank.employments {
		if e.ID == body.Data.ID && bank.owner[e.ID] == 1 {
			found = true
		}
	}
	if !found {
		t.Error("the employment was not persisted under caller 1")
	}
}

func TestAddProjectEmploymentReturnsName(t *testing.T) {
	app, token, bank := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/employments",
		`{"kind":"project","name":"telagon.io","link":"https://telagon.io"}`, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data["name"] != "telagon.io" {
		t.Errorf("name = %v, want telagon.io", body.Data["name"])
	}
	if _, ok := body.Data["company"]; ok {
		t.Errorf("project response must not include company, got %v", body.Data["company"])
	}
	if body.Data["link"] != "https://telagon.io" {
		t.Errorf("link = %v, want URL echoed", body.Data["link"])
	}
	if len(bank.employments) != 1 || bank.employments[0].Company != "telagon.io" {
		t.Errorf("stored = %+v, want company column filled from name", bank.employments)
	}
}

func TestAddProjectEmploymentAcceptsLegacyCompany(t *testing.T) {
	app, token, _ := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/employments",
		`{"kind":"project","company":"opensched"}`, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data["name"] != "opensched" {
		t.Errorf("name = %v, want legacy company returned as name", body.Data["name"])
	}
}

func TestListProjectEmploymentEmitsName(t *testing.T) {
	app, token, bank := experienceApp(t)
	bank.addEmployment(1, experience.Employment{
		Kind: experience.KindProject, Company: "telagon.io", Link: "https://telagon.io",
	})

	resp := experienceReq(t, app, http.MethodGet, "/me/experience", "", token)
	var body struct {
		Data struct {
			Employments []map[string]any `json:"employments"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if len(body.Data.Employments) != 1 {
		t.Fatalf("employments = %d, want 1", len(body.Data.Employments))
	}
	e := body.Data.Employments[0]
	if e["name"] != "telagon.io" {
		t.Errorf("name = %v, want telagon.io", e["name"])
	}
	if _, ok := e["company"]; ok {
		t.Errorf("list must not emit company for a project, got %v", e["company"])
	}
	if _, ok := e["atoms"]; !ok {
		t.Error("list entry must still carry atoms alongside the employment wire shape")
	}
}

func TestAddEmploymentRefusesOneWithNeitherCompanyNorRole(t *testing.T) {
	app, token, bank := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/employments", `{"kind":"job"}`, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
	if len(bank.employments) != 0 {
		t.Error("a refused employment was persisted anyway")
	}
}

func TestAddEmploymentRefusesAKindOutsideTheVocabulary(t *testing.T) {
	app, token, bank := experienceApp(t)

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/employments",
		`{"kind":"hobby","company":"RingCentral"}`, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
	if len(bank.employments) != 0 {
		t.Error("an employment with an out-of-vocabulary kind was persisted anyway")
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
	if resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms", `{"claim":"x"}`, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated atom create = %d, want 401", resp.StatusCode)
	}
	if resp := experienceReq(t, app, http.MethodPost, "/me/experience/employments", `{"kind":"job","company":"x"}`, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated employment create = %d, want 401", resp.StatusCode)
	}
}

func TestMergeAtomsUnionsAndRemovesLoser(t *testing.T) {
	app, token, bank := experienceApp(t)

	a := bank.add(1, experience.Atom{
		Claim:  "Built a Chromium plugin with faster-whisper for live and batch",
		Skills: []string{"python"}, Provenance: experience.ProvenanceAgentInferred,
	})
	b := bank.add(1, experience.Atom{
		Claim:   "Built a Chromium plugin with VAD filtering on faster-whisper",
		Context: "model profiles", Metrics: []string{"VAD"},
		Skills: []string{"nlp"}, Provenance: experience.ProvenanceAgentInferred,
	})
	bank.reindex()

	body := `{"ids":["` + a.ID.String() + `","` + b.ID.String() + `"]}`
	resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms/merge", body, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data struct {
			ID      uuid.UUID `json:"id"`
			Context string    `json:"context"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if out.Data.Context == "" {
		t.Error("kept atom has no context from the richer sibling")
	}

	list := experienceReq(t, app, http.MethodGet, "/me/experience", "", token)
	var listed struct {
		Data struct {
			Unplaced []struct {
				ID string `json:"id"`
			} `json:"unplaced"`
		} `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	list.Body.Close()
	if len(listed.Data.Unplaced) != 1 || listed.Data.Unplaced[0].ID != out.Data.ID.String() {
		t.Errorf("after merge unplaced = %+v, want only the keep", listed.Data.Unplaced)
	}
}

func TestMergeAtomsIsCookieOnly(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	bank := newStubBank()
	h := newExperienceHandlers(bank)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookieReject := func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusUnauthorized, "cookie only")
	}
	keyAccept := auth.RequireAuth(iss, testVersions)
	h.register(app.Group(""), middleware{cookie: cookieReject, key: keyAccept})

	a := bank.add(1, experience.Atom{Claim: "One", Provenance: experience.ProvenanceManual})
	b := bank.add(1, experience.Atom{Claim: "Two", Provenance: experience.ProvenanceManual})
	bank.reindex()

	body := `{"ids":["` + a.ID.String() + `","` + b.ID.String() + `"]}`
	resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms/merge", body, token)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("merge through key-only path = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestListExperienceFlagsSoftDuplicatesAndThinAtoms(t *testing.T) {
	app, token, bank := experienceApp(t)

	bank.add(1, experience.Atom{
		Claim:      "Built a Chromium plugin with custom audio transcription pipeline using faster-whisper models, with configurable profiles for live and batch processing",
		Provenance: experience.ProvenanceAgentInferred,
	})
	bank.add(1, experience.Atom{
		Claim:      "Built a Chromium plugin that transcribes audio using faster-whisper models with configurable profiles and VAD-based filtering to reduce hallucination artifacts",
		Provenance: experience.ProvenanceAgentInferred,
	})
	bank.reindex()

	resp := experienceReq(t, app, http.MethodGet, "/me/experience", "", token)
	var body struct {
		Data struct {
			Unplaced []struct {
				NeedsContext bool   `json:"needs_context"`
				NeedsMetrics bool   `json:"needs_metrics"`
				ClusterID    string `json:"cluster_id"`
			} `json:"unplaced"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if len(body.Data.Unplaced) != 2 {
		t.Fatalf("unplaced = %d, want 2", len(body.Data.Unplaced))
	}
	if body.Data.Unplaced[0].ClusterID == "" || body.Data.Unplaced[0].ClusterID != body.Data.Unplaced[1].ClusterID {
		t.Errorf("cluster ids = %q / %q, want a shared soft-dup cluster",
			body.Data.Unplaced[0].ClusterID, body.Data.Unplaced[1].ClusterID)
	}
	if !body.Data.Unplaced[0].NeedsContext || !body.Data.Unplaced[0].NeedsMetrics {
		t.Errorf("thin flags missing on claim-only atom: %+v", body.Data.Unplaced[0])
	}
}

type stubRequireContext struct{ on bool }

func (s stubRequireContext) GetUserExperienceRequireContext(context.Context, int64) (bool, error) {
	return s.on, nil
}

func TestAddAtomRespectsContextGate(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	bank := newStubBank()
	h := newExperienceHandlers(bank).withRequireContext(stubRequireContext{on: true})
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	guard := auth.RequireAuth(iss, testVersions)
	h.register(app.Group(""), middleware{cookie: guard, key: guard})

	resp := experienceReq(t, app, http.MethodPost, "/me/experience/atoms",
		`{"claim":"Cut latency"}`, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("gated empty context = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
	if len(bank.list) != 0 {
		t.Error("gated create persisted anyway")
	}

	resp = experienceReq(t, app, http.MethodPost, "/me/experience/atoms",
		`{"claim":"Cut latency","context":"payments checkout"}`, token)
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("gated with context = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()
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
