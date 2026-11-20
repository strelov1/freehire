//go:build integration

// The experience-bank and tailored-CV tools of the signed-in MCP server run the REST route's
// own handler as a keyed caller: cv_tailor keeps the paid-AI gate its route sits behind, a CV
// read is redacted, an edit is the agent's (an uncited claim is refused), a banked
// achievement is the candidate's, and a place with achievements is not deleted.
package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

func TestMCPCVTools_ActAsAKeyedCallerBehindThePaidGate(t *testing.T) {
	h, _, pool := newTailorAPI(t)
	ctx := context.Background()
	user := seedAccount(t, pool, "mcp-cv@example.test", true)
	seedFreshResume(t, pool, user)
	seedJobSlug(t, pool, "backend-eng")
	bank := experience.NewStore(experience.NewQueriesRepository(h.queries))
	mcpc := newMCPToolCaller(t, &assistantHandlers{cv: h, experienceH: newExperienceHandlers(bank)}, user)
	callTool := func(name string, args map[string]any) (string, bool) {
		return mcpc.call("call_tool", map[string]any{"name": name, "arguments": args})
	}
	countCVs := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM cvs WHERE user_id = $1`, user).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// A free account is refused before anything is created: hosted MCP is no way around paying.
	if text, failed := callTool("cv_tailor", map[string]any{"job_slug": "backend-eng"}); !failed || !strings.Contains(text, "paid plan") {
		t.Fatalf("free cv_tailor = %q (failed %v), want the paid-plan refusal", text, failed)
	}
	if n := countCVs(); n != 0 {
		t.Fatalf("free cv_tailor created %d CVs, want 0", n)
	}

	makePro(t, pool, user)
	text, failed := callTool("cv_tailor", map[string]any{"job_slug": "backend-eng"})
	if failed || !strings.Contains(text, "tailor_cv_id") {
		t.Fatalf("pro cv_tailor = %q (failed %v)", text, failed)
	}
	var cvID string
	if err := pool.QueryRow(ctx, `SELECT id FROM cvs WHERE user_id = $1 AND job_id IS NOT NULL`, user).Scan(&cvID); err != nil {
		t.Fatal(err)
	}
	if text, failed := callTool("cv_list", nil); failed || !strings.Contains(text, cvID) {
		t.Fatalf("cv_list = %q (failed %v), want the tailored CV", text, failed)
	}
	// The second tailoring of the same vacancy is the existing copy, so the pro ceiling of one is not spent again.
	if text, failed := callTool("cv_tailor", map[string]any{"job_slug": "backend-eng"}); failed || !strings.Contains(text, cvID) {
		t.Fatalf("repeat cv_tailor = %q (failed %v), want the same copy", text, failed)
	}

	if text, failed := callTool("cv_get", map[string]any{"id": cvID}); failed || strings.Contains(text, "Ada Lovelace") {
		t.Fatalf("cv_get = %q (failed %v), want the document without the contact block", text, failed)
	}
	// An edit is the agent's: a claim with no evidence is refused and the CV is untouched.
	uncited := map[string]any{"id": cvID, "ops": []any{map[string]any{"kind": "set", "path": "summary", "value": "Led a team of 40 engineers"}}}
	if text, failed := callTool("cv_edit", uncited); !failed {
		t.Fatalf("uncited cv_edit = %q, want it refused", text)
	}

	place := mcpc.ok("call_tool", map[string]any{"name": "experience_add_employment", "arguments": map[string]any{"company": "Acme", "role": "Engineer"}})
	placeID := place["id"].(string)
	atom := mcpc.ok("call_tool", map[string]any{"name": "experience_add_achievement", "arguments": map[string]any{
		"claim": "Cut deploy time from 20 minutes to 2", "employment_id": placeID}})
	if atom["provenance"] != string(experience.ProvenanceManual) {
		t.Fatalf("added achievement = %v, want provenance manual", atom)
	}
	// A partial correction keeps what it did not name.
	updated := mcpc.ok("call_tool", map[string]any{"name": "experience_update_employment", "arguments": map[string]any{"id": placeID, "role": "Senior Engineer"}})
	if updated["company"] != "Acme" || updated["role"] != "Senior Engineer" {
		t.Fatalf("updated place = %v, want company kept and role changed", updated)
	}
	if text, failed := callTool("experience_remove_employment", map[string]any{"id": placeID}); !failed || !strings.Contains(text, "achievement") {
		t.Fatalf("remove occupied place = %q (failed %v), want the 409 naming its achievements", text, failed)
	}
	if text, failed := callTool("experience_list", nil); failed || !strings.Contains(text, "Cut deploy time") {
		t.Fatalf("experience_list = %q (failed %v)", text, failed)
	}
}
