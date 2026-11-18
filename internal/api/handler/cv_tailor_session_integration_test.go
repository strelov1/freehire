//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestTailorContextSplit(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "ctx@example.test", true)
	jobID := seedJobSlug(t, pool, "backend-eng")
	seedAnalysis(t, h, user, jobID)

	// A tailored CV bound to the vacancy.
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	key := cvScopedKey(t, h, user)

	resp := doBearer(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String()+"/tailor-context", key, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("context = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data fitanalysis.TailoringContext `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if len(got.Data.MissingHave) != 1 || got.Data.MissingHave[0].Text != "Go" {
		t.Errorf("missing_have = %+v, want [Go]", got.Data.MissingHave)
	}
	if len(got.Data.MissingGap) != 1 || got.Data.MissingGap[0].Text != "Kubernetes" {
		t.Errorf("missing_gap = %+v, want [Kubernetes]", got.Data.MissingGap)
	}
	if got.Data.Verdict != "Good Fit" {
		t.Errorf("verdict = %q", got.Data.Verdict)
	}
	// The vacancy is bundled in so the agent sees the role it reframes toward.
	if got.Data.Job.Title != "Backend Engineer" || got.Data.Job.Slug != "backend-eng" {
		t.Errorf("job = %+v, want title=Backend Engineer slug=backend-eng", got.Data.Job)
	}

	// A base CV (no bound vacancy) is not tailorable-context → 409.
	base, _ := h.cvStore.Create(ctx, user, "Base", cv.DefaultTemplateID, cv.Document{})
	resp = doBearer(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+base.ID.String()+"/tailor-context", key, nil)
	_ = resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("base-cv context = %d, want 409", resp.StatusCode)
	}
}

// TestCVSessionAndTailoredList covers the CV↔session link over real Postgres: SetSession
// round-trips on Get, ListTailored returns the vacancy slug + session (and excludes base CVs),
// and SetSession is owner-scoped.
func TestCVSessionAndTailoredList(t *testing.T) {
	h, _, pool := newTailorAPI(t)
	ctx := context.Background()
	user := seedSubscriber(t, pool, "sess@example.test", true)
	jobID := seedJobSlug(t, pool, "backend-eng")

	// A base CV (job_id NULL) must NOT appear in the tailored list.
	if _, err := h.cvStore.Create(ctx, user, "Base", cv.DefaultTemplateID, cv.Document{}); err != nil {
		t.Fatalf("create base: %v", err)
	}
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	if err := h.cvStore.SetSession(ctx, tailored.ID, user, "sess-xyz"); err != nil {
		t.Fatalf("set session: %v", err)
	}
	rec, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.AgentSessionID != "sess-xyz" {
		t.Errorf("session = %q, want sess-xyz", rec.AgentSessionID)
	}

	items, err := h.cvStore.ListTailored(ctx, user)
	if err != nil {
		t.Fatalf("list tailored: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("tailored list len = %d, want 1 (base excluded)", len(items))
	}
	if items[0].JobSlug != "backend-eng" || items[0].AgentSessionID != "sess-xyz" {
		t.Errorf("tailored item = %+v, want slug backend-eng + session sess-xyz", items[0])
	}

	other := seedSubscriber(t, pool, "other-sess@example.test", true)
	if err := h.cvStore.SetSession(ctx, tailored.ID, other, "hijack"); !errors.Is(err, cv.ErrNotFound) {
		t.Errorf("foreign set-session err = %v, want ErrNotFound", err)
	}
}

// cvScopedKey issues a user-created `cv`-scoped API key — the credential the public
// CLI authenticates its CV commands with. The in-app agent no longer uses one (it
// runs as the caller), but the scope and its owner checks still guard this surface.
func cvScopedKey(t *testing.T, h *cvHandlers, userID int64) string {
	t.Helper()
	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := h.queries.CreateAPIKey(context.Background(), db.CreateAPIKeyParams{
		UserID:      userID,
		Name:        "cv scope",
		TokenHash:   hash,
		TokenPrefix: prefix,
		Scope:       auth.ScopeCV,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}
	return token
}
