package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// TestProvenanceForUpdate is the whole of the rule that lets a key correct a banked
// achievement at all.
//
// Correcting an atom stamps it `manual` — "the person asserted this" — and that stamp is
// honest only because the candidate is the one pressing the button. A key is held by the
// tailoring agent, so the same stamp coming from a key would be a laundering step: the
// agent banks its own reading as `agent_inferred` (unpublishable), PUTs it, gets `manual`
// back, and the CV evidence gate then admits a claim nobody ever made.
//
// So the label follows the credential. A keyed correction may fix the words and must leave
// the standing of the claim exactly where it was.
func TestUpdateAuthor(t *testing.T) {
	if got := updateAuthor(false); got != experience.AuthorCandidate {
		t.Errorf("cookie PUT author = %q, want AuthorCandidate — the person is editing their own bank", got)
	}
	if got := updateAuthor(true); got != experience.AuthorRewrite {
		t.Errorf("keyed PUT author = %q, want AuthorRewrite — a keyed edit rewrites words and may not relabel", got)
	}
}

// What each authorship then MEANS is experience.provenanceFor's, tested next to the rule in
// TestProvenanceFor — including the laundering route this door exists to close.

// TestExperienceRegister_Gates pins each route's gate against the real register().
//
// The two corrections take a key: a caller that can ADD an achievement from a script but
// cannot fix a typo in it is left with a bank it can only make worse, since the duplicate
// check refuses the corrected retry.
//
// The deletes take a key too, but the cascade does not come with them: DeleteEmployment
// refuses a keyed caller while achievements still hang off the row (migration 0047 deletes
// them with it). See TestKeyedEmploymentDeleteRefusesToCascade.
func TestExperienceRegister_Gates(t *testing.T) {
	const id = "/00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/me/experience", "key"},
		{http.MethodPost, "/me/experience/employments", "key"},
		{http.MethodPost, "/me/experience/atoms", "key"},
		{http.MethodPut, "/me/experience/employments" + id, "key"},
		{http.MethodPut, "/me/experience/atoms" + id, "key"},
		{http.MethodDelete, "/me/experience/employments" + id, "key"},
		{http.MethodDelete, "/me/experience/atoms" + id, "key"},
		// The merge stays cookie-only, and unlike the deletes it cannot simply be widened:
		// unionForMerge folds the discarded atom's metrics and skills into the kept one
		// while the kept one's provenance stands, so a `manual` atom can absorb numbers an
		// agent invented. Opening it needs that settled first, not a gate change.
		{http.MethodPost, "/me/experience/atoms/merge", "cookie"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			app := fiber.New()
			(&experienceHandlers{}).register(app.Group(""), middleware{
				key:    namedGate("key"),
				cookie: namedGate("cookie"),
			})
			resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil))
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if got := string(body); got != tc.want {
				t.Errorf("%s %s is gated by %q, want %q", tc.method, tc.path, got, tc.want)
			}
		})
	}
}

// stubKeys resolves any presented key to one owner, so the REAL RequireAuthOrKey can run in
// a unit test. Going through the real middleware is the point: it is what sets the
// via-API-key marker the provenance rule reads, and a test that set that marker itself
// would still pass if the middleware stopped setting it.
type stubKeys struct{ userID int64 }

func (s stubKeys) AuthenticateAPIKey(context.Context, string) (auth.APIKeyIdentity, error) {
	return auth.APIKeyIdentity{UserID: s.userID, Scope: auth.ScopeFull}, nil
}

// TestUpdateAtomEndToEndProvenance is the rule above, through the real route and the real
// auth middleware: the same correction, made with a key and with a cookie, must leave the
// atom's standing in two different places.
//
// If the keyed subtest ever comes back `manual`, an agent can write its own invention onto
// a real person's CV.
func TestUpdateAtomEndToEndProvenance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		viaKey bool
		want   experience.Provenance
	}{
		{"keyed correction leaves the agent's own reading unpublishable", true, experience.ProvenanceAgentInferred},
		{"the candidate's own correction is their assertion", false, experience.ProvenanceManual},
	} {
		t.Run(tc.name, func(t *testing.T) {
			iss := auth.NewIssuer("test-secret", time.Hour)
			token, err := iss.Issue(1, testTokenVersion)
			if err != nil {
				t.Fatalf("issue token: %v", err)
			}
			bank := newStubBank()
			atom := bank.add(1, experience.Atom{
				Claim:      "Rewrote the billing pipeline",
				Provenance: experience.ProvenanceAgentInferred,
			})

			guard := auth.RequireAuthOrKey(iss, testVersions, stubKeys{userID: 1})
			app := fiber.New(fiber.Config{ErrorHandler: RenderError})
			newExperienceHandlers(bank).register(app.Group(""), middleware{cookie: guard, key: guard})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPut,
				"/me/experience/atoms/"+atom.ID.String(),
				strings.NewReader(`{"claim":"Rewrote the billing pipeline end to end"}`))
			req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
			if tc.viaKey {
				req.Header.Set("Authorization", "Bearer fhk_test")
			} else {
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			}

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("put: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("put status = %d, body %s", resp.StatusCode, body)
			}
			var out struct {
				Data experience.Atom `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out.Data.Provenance != tc.want {
				t.Errorf("provenance after update = %q, want %q", out.Data.Provenance, tc.want)
			}
			if tc.viaKey && out.Data.Provenance.Publishable() {
				t.Error("a keyed correction made an agent-inferred claim publishable — the CV evidence gate would admit it")
			}
			if out.Data.Claim != "Rewrote the billing pipeline end to end" {
				t.Errorf("the correction itself did not land: claim = %q", out.Data.Claim)
			}
		})
	}
}
