//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
)

func TestPatchCVViaKey(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	owner := seedSubscriber(t, pool, "owner@example.test", true)
	other := seedSubscriber(t, pool, "other@example.test", true)

	base, err := h.cvStore.Create(ctx, owner, "General", cv.DefaultTemplateID, cv.Document{
		Header:     cv.Header{FullName: "Ada Lovelace", Email: "ada@x.com", Phone: "+1 415 555 0000"},
		Experience: []cv.ExperienceItem{{Role: "Eng", Bullets: []string{"Shipped API"}}},
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}
	path := "/api/v1/me/cvs/" + base.ID.String()

	// The agent (API key) must never see the contact block; the owner's cookie read does.
	decodeHeader := func(resp *http.Response) cv.Header {
		var w struct {
			Data struct {
				Document cv.Document `json:"document"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&w)
		return w.Data.Document.Header
	}

	ownerKey := cvScopedKey(t, h, owner)
	otherKey := cvScopedKey(t, h, other)

	// The key edits as the AGENT, so a bullet stating something about the candidate has to
	// cite banked evidence. Uncited is refused — this is the rule the tailoring capability
	// exists to enforce, and it must hold for an API-key caller with no assistant in sight.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{"kind": "insert", "path": "experience[0].bullets[1]", "value": "Cut latency"}},
	}); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("uncited patch = %d, want 403", resp.StatusCode)
	}

	// Cited by an atom the candidate asserted, the same batch applies — so the gate is a
	// wall with a door, not a blanket refusal.
	bank := experience.NewStore(experience.NewQueriesRepository(h.queries))
	atom, err := bank.AddAtom(ctx, owner, experience.Atom{
		Claim: "Cut latency", Provenance: experience.ProvenanceStatedInChat,
	},
		experience.AuthorQuoted,
	)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{
			"kind": "insert", "path": "experience[0].bullets[1]",
			"value": "Cut latency", "evidence_id": atom.ID.String(),
		}},
	}); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("cited patch = %d, want 200", resp.StatusCode)
	}
	rec, _ := h.cvStore.Get(ctx, base.ID, owner)
	if got := rec.Document.Experience[0].Bullets; len(got) != 2 || got[1] != "Cut latency" {
		t.Errorf("bullets after patch = %v", got)
	}

	// The minted key can also READ the CV back (GET /me/cvs/:id is keyAuth) — the agent
	// needs this to see the current document before patching — but the contact block is stripped.
	if resp := doBearer(t, app, fiber.MethodGet, path, ownerKey, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("get via key = %d, want 200", resp.StatusCode)
	} else if h := decodeHeader(resp); h.FullName != "" || h.Email != "" || h.Phone != "" {
		t.Errorf("agent read leaked the contact block: %+v", h)
	}

	// The owner's own cookie read sees the full contact block.
	ownerCookie, _ := iss.Issue(owner, testTokenVersion)
	greq := httptest.NewRequest(fiber.MethodGet, path, nil)
	greq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: ownerCookie})
	if resp, err := app.Test(greq); err != nil {
		t.Fatalf("owner get: %v", err)
	} else if h := decodeHeader(resp); h.FullName != "Ada Lovelace" || h.Email != "ada@x.com" {
		t.Errorf("owner read should keep contacts, got %+v", h)
	}

	// The agent cannot WRITE a contact field either.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{"kind": "set", "path": "header.full_name", "value": "Fake Name"}},
	}); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("patch full_name via key = %d, want 403", resp.StatusCode)
	}

	// Bad addressing is a 422 — but only once the evidence gate has let the op through. The
	// gate is the OUTER check: an uncited claim is 403 whatever its path addresses, which the
	// old nil-gate fixture could never show. Citing the banked atom is what makes this case
	// about addressing again.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{
			"kind": "set", "path": "experience[0].bullets[9]",
			"value": "x", "evidence_id": atom.ID.String(),
		}},
	}); resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("bad-patch = %d, want 422", resp.StatusCode)
	}

	// And the ordering itself, pinned: the same malformed op WITHOUT evidence is refused by
	// the gate rather than reaching the addressing check.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{"kind": "set", "path": "experience[0].bullets[9]", "value": "x"}},
	}); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("uncited bad-patch = %d, want 403 — the gate runs before addressing", resp.StatusCode)
	}

	// Another user's key cannot touch this CV (owner isolation → 404, not 403). The op is a
	// REMOVE because the evidence gate runs first and only gates claim-bearing set/insert:
	// with a claim-bearing op the foreign caller is refused at the gate (403) and never
	// reaches the ownership check. Isolation is intact either way — 403 is what an uncited
	// claim gets whether or not the CV exists, so neither answer discloses one — but only a
	// non-claiming op actually exercises the 404.
	if resp := doBearer(t, app, fiber.MethodPatch, path, otherKey, map[string]any{
		"ops": []map[string]any{{"kind": "remove", "path": "experience[0].bullets[0]"}},
	}); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("foreign patch = %d, want 404", resp.StatusCode)
	}
}
