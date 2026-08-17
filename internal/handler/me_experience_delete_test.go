package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/experience"
)

// deleteApp wires the bank surface behind the real RequireAuthOrKey, and returns a cookie
// token so one fixture can exercise both credentials.
func deleteApp(t *testing.T, bank *stubBank) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	guard := auth.RequireAuthOrKey(iss, testVersions, stubKeys{userID: 1})
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	newExperienceHandlers(bank).register(app.Group(""), middleware{cookie: guard, key: guard})
	return app, token
}

// deleteReq issues a DELETE as either credential.
func deleteReq(t *testing.T, app *fiber.App, path, token string, viaKey bool) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, path, nil)
	if viaKey {
		req.Header.Set("Authorization", "Bearer fhk_test")
	} else {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("delete %s: %v", path, err)
	}
	return resp
}

// TestKeyedEmploymentDeleteRefusesToCascade is what makes deleting reachable by key at all.
//
// experience_atoms.employment_id is ON DELETE CASCADE (migration 0047), so removing a place
// removes every achievement recorded under it, and the bank has no undo. That is a coherent
// thing for a person to do in a browser, where the confirm dialog says how many go with it.
// It is not a coherent thing to reach through a credential sitting in a script's environment,
// where the same call is one wrong id away from erasing years of someone's record.
//
// So a keyed delete is confined to a place that is already empty. The route stays useful —
// move the achievements first, then remove the shell — and the destructive form stays where
// its consequence is visible. The rule lives here rather than in a CLI confirmation prompt,
// because a prompt is no obstacle at all to the agent it would be protecting against.
func TestKeyedEmploymentDeleteRefusesToCascade(t *testing.T) {
	bank := newStubBank()
	place := bank.addEmployment(1, experience.Employment{Kind: experience.KindJob, Company: "Informa"})
	bank.add(1, experience.Atom{
		Claim: "Led the migration to serverless", EmploymentID: &place.ID,
		Provenance: experience.ProvenanceManual,
	})
	bank.reindex()
	app, _ := deleteApp(t, bank)

	resp := deleteReq(t, app, "/me/experience/employments/"+place.ID.String(), "", true)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("keyed delete of a non-empty employment = %d, want 409", resp.StatusCode)
	}
	if _, err := bank.GetAtom(context.Background(), bank.list[0].ID, 1); err != nil {
		t.Errorf("the achievement was destroyed by a refused delete: %v", err)
	}
}

// The candidate's own delete keeps the cascade. They are looking at the confirmation the
// browser showed them; taking the capability away here would be removing the only way to
// delete a place at all.
func TestCookieEmploymentDeleteStillCascades(t *testing.T) {
	bank := newStubBank()
	place := bank.addEmployment(1, experience.Employment{Kind: experience.KindJob, Company: "Informa"})
	bank.add(1, experience.Atom{
		Claim: "Led the migration to serverless", EmploymentID: &place.ID,
		Provenance: experience.ProvenanceManual,
	})
	bank.reindex()
	app, token := deleteApp(t, bank)

	resp := deleteReq(t, app, "/me/experience/employments/"+place.ID.String(), token, false)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("cookie delete of a non-empty employment = %d, want 204", resp.StatusCode)
	}
	if len(bank.employments) != 0 {
		t.Errorf("the employment survived a cookie delete: %v", bank.employments)
	}
}

// An emptied place is the whole point of the rule above: move the achievements, then remove
// the shell. If this were refused too, the keyed delete would be decorative.
func TestKeyedEmploymentDeleteAllowsAnEmptyPlace(t *testing.T) {
	bank := newStubBank()
	place := bank.addEmployment(1, experience.Employment{Kind: experience.KindJob, Company: "TEST — delete me"})
	app, _ := deleteApp(t, bank)

	resp := deleteReq(t, app, "/me/experience/employments/"+place.ID.String(), "", true)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("keyed delete of an empty employment = %d, want 204", resp.StatusCode)
	}
}

// Deleting one achievement takes nothing with it, so it needs no equivalent of the rule
// above — the row named is the only row removed.
func TestKeyedAtomDelete(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(1, experience.Atom{Claim: "Duplicate of another entry", Provenance: experience.ProvenanceCVImport})
	bank.reindex()
	app, _ := deleteApp(t, bank)

	resp := deleteReq(t, app, "/me/experience/atoms/"+atom.ID.String(), "", true)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("keyed atom delete = %d, want 204", resp.StatusCode)
	}
	if _, err := bank.GetAtom(context.Background(), atom.ID, 1); err == nil {
		t.Error("the atom survived its own delete")
	}
}

// A foreign or absent id is a 404 before the emptiness check runs, so the refusal above
// never doubles as a way to probe whether an id exists.
func TestKeyedEmploymentDeleteUnknownID(t *testing.T) {
	app, _ := deleteApp(t, newStubBank())
	resp := deleteReq(t, app, "/me/experience/employments/"+uuid.NewString(), "", true)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("keyed delete of an unknown employment = %d, want 404", resp.StatusCode)
	}
}
