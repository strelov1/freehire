//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
)

// Reloading /tailor/<slug> re-runs the bootstrap: the address carries no CV reference, so the
// page asks for a tailored CV again. That must reach the SAME CV and the SAME conversation —
// three CVs appeared on one vacancy in half an hour on production, each with an empty chat,
// because every refresh minted another copy and rebound the session.
func TestTailorCVBootstrapIsIdempotentPerVacancy(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)

	user := seedSubscriber(t, pool, "tailor-reload@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	jobID := seedJobSlug(t, pool, "backend-eng")
	seedAnalysis(t, h, user, jobID)
	seedFreshResume(t, pool, user)

	bootstrap := func() tailorCVResponse {
		t.Helper()
		resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"})
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
		}
		var got struct {
			Data tailorCVResponse `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		return got.Data
	}

	first := bootstrap()
	if !first.ColdStartRunning {
		t.Errorf("first bootstrap: cold_start_running = false, want true (it just created the CV)")
	}
	// Something was said in that conversation, so losing it would be losing real work.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO assistant_messages (session_id, seq, role, content)
		 VALUES ($1, 1, 'user', '{"text":"tailor it for me"}'::jsonb)`, first.SessionID); err != nil {
		t.Fatalf("seed a message: %v", err)
	}

	second := bootstrap()

	if second.TailorCVID != first.TailorCVID {
		t.Errorf("reload made a new CV (%s vs %s)", second.TailorCVID, first.TailorCVID)
	}
	if second.SessionID != first.SessionID {
		t.Errorf("reload rebound the conversation (%s vs %s) — the candidate's messages are on the old one", second.SessionID, first.SessionID)
	}
	if second.ColdStartRunning {
		t.Errorf("reload: cold_start_running = true, want false — the CV already existed")
	}

	var cvs, sessions int
	if err := pool.QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM cvs WHERE user_id = $1 AND job_id = $2),
		        (SELECT count(*) FROM assistant_sessions WHERE user_id = $1 AND job_id = $2)`,
		user, jobID).Scan(&cvs, &sessions); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cvs != 1 || sessions != 1 {
		t.Errorf("after two bootstraps: %d CVs and %d sessions, want one of each", cvs, sessions)
	}

	// And the charge happened once, not twice.
	var charges int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM usage_ledger WHERE user_id = $1 AND kind = 'consume' AND feature = 'tailor'`,
		user).Scan(&charges); err != nil {
		t.Fatalf("count charges: %v", err)
	}
	if charges != 1 {
		t.Errorf("tailoring charges = %d, want 1 — a reload is not a second purchase", charges)
	}
}

// TestTailorCVAlignsSkillSurfaceOnMint: a fresh tailored copy stores the vacancy's preferred
// spelling of a skill the seed already carries (IaC → infrastructure as code). A second
// bootstrap must not re-align — a candidate who put the acronym back keeps it.
func TestTailorCVAlignsSkillSurfaceOnMint(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)

	user := seedSubscriber(t, pool, "align-mint@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	seedJobSlugDesc(t, pool, "align-mint-job",
		"We practice infrastructure as code and Terraform daily.")
	seedFreshResumeSkills(t, pool, user, []string{"IaC", "Terraform"})

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "align-mint-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()

	read := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+got.Data.TailorCVID, tok, nil)
	var doc struct {
		Data struct {
			Document cv.Document `json:"document"`
		} `json:"data"`
	}
	_ = json.NewDecoder(read.Body).Decode(&doc)
	read.Body.Close()
	if len(doc.Data.Document.Skills) == 0 || len(doc.Data.Document.Skills[0].Items) == 0 {
		t.Fatalf("skills = %+v, want at least one chip", doc.Data.Document.Skills)
	}
	chip := doc.Data.Document.Skills[0].Items[0]
	if chip != "infrastructure as code" {
		t.Fatalf("skill chip = %q, want vacancy preferred form", chip)
	}

	// Candidate puts the acronym back; second bootstrap must leave it alone.
	patch := doCV(t, app, fiber.MethodPatch, "/api/v1/me/cvs/"+got.Data.TailorCVID, tok, map[string]any{
		"ops": []map[string]any{{"kind": "set", "path": "skills[0].items[0]", "value": "IaC"}},
	})
	if patch.StatusCode != fiber.StatusOK {
		t.Fatalf("patch = %d, want 200", patch.StatusCode)
	}
	patch.Body.Close()

	again := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "align-mint-job"})
	if again.StatusCode != fiber.StatusOK && again.StatusCode != fiber.StatusCreated {
		t.Fatalf("second bootstrap = %d", again.StatusCode)
	}
	var againGot struct {
		Data tailorCVResponse `json:"data"`
	}
	_ = json.NewDecoder(again.Body).Decode(&againGot)
	again.Body.Close()
	if againGot.Data.TailorCVID != got.Data.TailorCVID {
		t.Fatalf("second bootstrap minted a new CV %s, want existing %s", againGot.Data.TailorCVID, got.Data.TailorCVID)
	}

	read2 := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+got.Data.TailorCVID, tok, nil)
	_ = json.NewDecoder(read2.Body).Decode(&doc)
	read2.Body.Close()
	if doc.Data.Document.Skills[0].Items[0] != "IaC" {
		t.Fatalf("after second bootstrap chip = %q, want user-edited IaC left in place", doc.Data.Document.Skills[0].Items[0])
	}
}
