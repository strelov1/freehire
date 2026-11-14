//go:build integration

// Integration tests for the follow-up draft (application-followup-draft): the gate on the silence
// state, the two recipient paths, and the recorded chase.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

// appliedDaysAgo backdates an application so it lands on the far side of its stage's threshold.
func appliedDaysAgo(t *testing.T, f *harnessInboxFixture, jobID int64, days int, stage string) {
	t.Helper()
	at := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	if _, err := f.pool.Exec(context.Background(),
		`WITH mark AS (
		     INSERT INTO user_jobs (user_id, job_id) VALUES ($1,$2)
		     ON CONFLICT (user_id, job_id) DO NOTHING
		 )
		 INSERT INTO applications (user_id, job_id, company_slug, role_title, applied_at, stage)
		 SELECT $1, $2, j.company_slug, j.title, $3, $4 FROM jobs j WHERE j.id = $2
		 ON CONFLICT (user_id, job_id) WHERE job_id IS NOT NULL
		 DO UPDATE SET applied_at = $3, stage = $4`,
		f.userID, jobID, at, stage); err != nil {
		t.Fatalf("seed application: %v", err)
	}
}

func TestFollowUpDraft_SilentApplicationGetsADraft(t *testing.T) {
	f := newHarnessInboxFixture(t, "fu-silent@example.test")
	jid := seedJobSlug(t, f.pool, "followup-silent")
	appliedDaysAgo(t, f, jid, 24, "applied") // past the 21-day `applied` threshold

	status, body := f.callKey(fiber.MethodGet, "/api/v1/me/tracking/followup-silent/followup", nil)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, _ := body["data"].(map[string]any)
	subject, _ := data["subject"].(string)
	text, _ := data["body"].(string)
	if subject == "" || text == "" {
		t.Fatalf("draft = %+v, want a subject and a body", data)
	}
	if recipient, ok := data["recipient"].(string); ok && recipient != "" {
		t.Errorf("recipient = %q, want none: nobody ever replied to this application", recipient)
	}
}

func TestFollowUpDraft_ActiveApplicationIsRefused(t *testing.T) {
	f := newHarnessInboxFixture(t, "fu-active@example.test")
	jid := seedJobSlug(t, f.pool, "followup-active")
	appliedDaysAgo(t, f, jid, 3, "applied") // well inside the tolerated silence

	status, _ := f.callKey(fiber.MethodGet, "/api/v1/me/tracking/followup-active/followup", nil)

	if status != fiber.StatusConflict {
		t.Errorf("status = %d, want 409 — an application answering promptly has nothing to chase", status)
	}
}

func TestFollowUpDraft_UntrackedSlugIsNotFound(t *testing.T) {
	f := newHarnessInboxFixture(t, "fu-untracked@example.test")
	seedJobSlug(t, f.pool, "followup-untracked")

	if status, _ := f.callKey(fiber.MethodGet, "/api/v1/me/tracking/followup-untracked/followup", nil); status != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404 for a job the caller does not track", status)
	}
}

// TestFollowUpDraft_PrefillsTheRecipientFromLinkedMail covers the other half of the constraint that
// shaped this feature: an address exists only when somebody wrote back and then went quiet.
func TestFollowUpDraft_PrefillsTheRecipientFromLinkedMail(t *testing.T) {
	f := newHarnessInboxFixture(t, "fu-replied@example.test")
	jid := seedJobSlug(t, f.pool, "followup-replied")
	appliedDaysAgo(t, f, jid, 40, "screening")
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO emails (user_id, external_id, thread_id, from_addr, from_name, subject, body_text, received_at, source, job_id)
		 VALUES ($1,'fu-1','t-1','mara@acme.test','Mara Lin','Re: your application','...',$2,'test',$3)`,
		f.userID, time.Now().Add(-30*24*time.Hour), jid); err != nil {
		t.Fatalf("seed mail: %v", err)
	}

	status, body := f.callKey(fiber.MethodGet, "/api/v1/me/tracking/followup-replied/followup", nil)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, _ := body["data"].(map[string]any)
	if got, _ := data["recipient"].(string); got != "mara@acme.test" {
		t.Errorf("recipient = %q, want the sender of the linked mail", got)
	}
	if text, _ := data["body"].(string); text == "" {
		t.Fatal("no body")
	}
}

func TestFollowUpRecord_WritesTheChaseAndIsOwnerScoped(t *testing.T) {
	f := newHarnessInboxFixture(t, "fu-record@example.test")
	jid := seedJobSlug(t, f.pool, "followup-record")
	appliedDaysAgo(t, f, jid, 24, "applied")

	if status, _ := f.callKey(fiber.MethodPost, "/api/v1/me/tracking/followup-record/followup", nil); status != fiber.StatusNoContent {
		t.Fatalf("record status = %d, want 204", status)
	}

	row, err := f.q.GetUserApplication(context.Background(),
		db.GetUserApplicationParams{UserID: f.userID, JobID: jid})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !row.FollowedUpAt.Valid {
		t.Error("followed_up_at is unset after recording a follow-up")
	}
	// The silence it was a response to must survive it.
	if !row.LastActivityAt.Valid || row.LastActivityAt.Time.After(time.Now().Add(-20*24*time.Hour)) {
		t.Errorf("last_activity_at = %v after the chase, want it still ~24 days back", row.LastActivityAt)
	}

	// A second press is not an error.
	if status, _ := f.callKey(fiber.MethodPost, "/api/v1/me/tracking/followup-record/followup", nil); status != fiber.StatusNoContent {
		t.Errorf("second record status = %d, want 204 — a double click must not error", status)
	}
}

func TestFollowUpRecord_UntrackedSlugWritesNothing(t *testing.T) {
	f := newHarnessInboxFixture(t, "fu-record-untracked@example.test")
	jid := seedJobSlug(t, f.pool, "followup-record-untracked")

	if status, _ := f.callKey(fiber.MethodPost, "/api/v1/me/tracking/followup-record-untracked/followup", nil); status != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM applications WHERE user_id=$1 AND job_id=$2 AND followed_up_at IS NOT NULL`,
		f.userID, jid).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("wrote %d follow-up rows for an untracked job, want 0", n)
	}
}
