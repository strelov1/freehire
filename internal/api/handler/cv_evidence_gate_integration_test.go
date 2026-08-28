//go:build integration

// The evidence gate must be a property of the editor the CV surface is built with, not
// of whether some other feature's constructor happened to run. These tests assemble the
// CV handlers through the production constructor and NOTHING else — no assistant — and
// then edit as the agent.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/credits"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// newCVAPIWithoutAssistant builds the CV handlers the way handler.Register does — through
// newCVHandlers — and mounts only the CV routes. The assistant is deliberately absent:
// PATCH /me/cvs/:id edits as the agent for any API-key caller and has nothing to do with
// the assistant, so it must not depend on the assistant having been built.
func newCVAPIWithoutAssistant(t *testing.T) (*cvHandlers, *auth.Issuer, *fiber.App, int64) {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE cvs, users, jobs, user_job_analysis, api_keys, assistant_sessions, experience_atoms RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	creditsStore := credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	bank := experience.NewStore(experience.NewQueriesRepository(queries))

	h := newCVHandlers(pool, queries, cv.NewStore(cv.NewQueriesRepository(queries)),
		assistant.NewStore(queries),
		// No renderer: this fixture exercises the edit path, and the PDF endpoint's 501
		// gate is what a nil one means.
		nil, "test-salt", "https://freehire.test",
		[]string{"freehire.test"},
		resume.New(nil, resume.NewQueriesRepository(queries)),
		headshot.New(nil, headshot.NewQueriesRepository(queries)),
		creditsStore,
		&matchHandlers{fit: fitanalysis.New(queries, creditsStore, matchanalysis.NewAnalyzer(nil))},
		bankGate{bank: bank},
		nil, // tracking save unused — this fixture never bootstraps tailor
		true,
	)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrScopedKey(iss, testVersions, apiKeys{h.queries}, auth.ScopeCV)
	app.Patch("/api/v1/me/cvs/:id", keyAuth, h.PatchCV)

	owner := seedAccount(t, pool, "owner@example.test", true)
	return h, iss, app, owner
}

// The honest wall: an API-key caller edits as ActorAgent, so a bullet — a claim about what
// the candidate did — cannot reach the page without citing banked evidence. Before this
// was a constructor argument, the same request succeeded whenever the assembly did not
// also build the assistant, because the editor was created with a nil gate and the gate
// treated its own absence as permission.
func TestPatchCVAsAgentIsGatedWithoutTheAssistant(t *testing.T) {
	h, _, app, owner := newCVAPIWithoutAssistant(t)
	ctx := context.Background()

	base, err := h.cvStore.Create(ctx, owner, "General", cv.DefaultTemplateID, cv.Document{
		Header:     cv.Header{FullName: "Ada Lovelace", Email: "ada@x.com"},
		Experience: []cv.ExperienceItem{{Role: "Eng", Bullets: []string{"Shipped API"}}},
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}
	path := "/api/v1/me/cvs/" + base.ID.String()
	key := cvScopedKey(t, h, owner)

	resp := doBearer(t, app, fiber.MethodPatch, path, key, map[string]any{
		"ops": []map[string]any{
			{"kind": "insert", "path": "experience[0].bullets[1]", "value": "Cut latency by 40%"},
		},
	})
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("uncited agent patch = %d, want 403 — the editor admitted a claim with no evidence", resp.StatusCode)
	}

	// And it really was refused, not merely reported as refused.
	rec, err := h.cvStore.Get(ctx, base.ID, owner)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got := rec.Document.Experience[0].Bullets; len(got) != 1 {
		t.Errorf("bullets = %v, want the original one only", got)
	}
}

// The rule exempts the candidate: they are the source the bank exists to record, so their
// own edit needs no citation. The same editor serves both actors, which is why the gate
// being present cannot be allowed to break candidate editing.
func TestPatchCVAsCandidateNeedsNoEvidence(t *testing.T) {
	h, iss, app, owner := newCVAPIWithoutAssistant(t)
	ctx := context.Background()

	base, err := h.cvStore.Create(ctx, owner, "General", cv.DefaultTemplateID, cv.Document{
		Experience: []cv.ExperienceItem{{Role: "Eng", Bullets: []string{"Shipped API"}}},
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}
	token, _ := iss.Issue(owner, testTokenVersion)
	body, _ := json.Marshal(map[string]any{
		"ops": []map[string]any{
			{"kind": "insert", "path": "experience[0].bullets[1]", "value": "Cut latency by 40%"},
		},
	})
	req := httptest.NewRequest(fiber.MethodPatch, "/api/v1/me/cvs/"+base.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("candidate patch = %d, want 200", resp.StatusCode)
	}
}
