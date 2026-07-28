//go:build integration

// Integration tests for recording an application from a piece of mail: the
// application is created and the email linked in one call, dated by the mail
// rather than by the moment of recording, counted exactly once, and refused for
// mail that still carries a pending suggestion. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// backdate moves a seeded message's received_at into the past, so the recorded
// application's date can be told apart from now().
func (f *agentInboxFixture) backdate(emailID int64, at time.Time) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE emails SET received_at = $1 WHERE id = $2`, at, emailID); err != nil {
		f.t.Fatalf("backdate email %d: %v", emailID, err)
	}
}

// insertJobOnly seeds a catalog job the caller has no interaction with.
func (f *agentInboxFixture) insertJobOnly(slug string) int64 {
	f.t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('test', $1, 'http://example.test/'||$1, 'Go Dev', 'Acme', $1) RETURNING id`,
		slug).Scan(&id); err != nil {
		f.t.Fatalf("seed job %s: %v", slug, err)
	}
	return id
}

// application reads the caller's interaction with a job.
func (f *agentInboxFixture) application(jobID int64) (appliedAt time.Time, stage string, found bool) {
	f.t.Helper()
	var st *string
	var at *time.Time
	err := f.pool.QueryRow(context.Background(),
		`SELECT applied_at, stage FROM user_jobs WHERE user_id = $1 AND job_id = $2`,
		f.userID, jobID).Scan(&at, &st)
	if err != nil {
		return time.Time{}, "", false
	}
	if at != nil {
		appliedAt = *at
	}
	if st != nil {
		stage = *st
	}
	return appliedAt, stage, true
}

func (f *agentInboxFixture) appliedCount(jobID int64) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT applied_count FROM jobs WHERE id = $1`, jobID).Scan(&n); err != nil {
		f.t.Fatalf("read applied_count: %v", err)
	}
	return n
}

func applicationPath(emailID int64) string {
	return "/api/v1/me/emails/" + strconv.FormatInt(emailID, 10) + "/application"
}

// TestApplicationFromMailRecordsAndLinks asserts the whole point: mail about an
// application the caller never recorded becomes a tracked, linked application
// dated by that mail.
func TestApplicationFromMailRecordsAndLinks(t *testing.T) {
	f := newAgentInboxFixture(t, "fromMail@example.test")
	jobID := f.insertJobOnly("acme-from-mail")

	when := time.Now().Add(-21 * 24 * time.Hour).UTC().Truncate(time.Second)
	emailID := f.seedEmail(f.userID, "hosted", "fm-1", "Interview invitation", "body")
	f.backdate(emailID, when)

	code, body := f.callKey("POST", applicationPath(emailID), map[string]string{"slug": "acme-from-mail"})
	if code != 200 {
		t.Fatalf("record from mail = %d, want 200 (body %v)", code, body)
	}

	appliedAt, stage, found := f.application(jobID)
	if !found {
		t.Fatal("no application was created")
	}
	if got := appliedAt.UTC().Truncate(time.Second); !got.Equal(when) {
		t.Errorf("applied_at = %v, want the email's received_at %v", got, when)
	}
	if stage != "applied" {
		t.Errorf("stage = %q, want applied", stage)
	}
	if n := f.appliedCount(jobID); n != 1 {
		t.Errorf("applied_count = %d, want 1", n)
	}

	// The email now reads as linked, and as a manual link — the caller's decision
	// is never recorded as the matcher's.
	data, _ := body["data"].(map[string]any)
	if got, _ := data["linked_slug"].(string); got != "acme-from-mail" {
		t.Errorf("linked_slug = %q, want acme-from-mail", got)
	}
	if got, _ := data["link_source"].(string); got != "manual" {
		t.Errorf("link_source = %q, want manual", got)
	}
}

// TestApplicationFromMailKeepsAnEarlierDate asserts a second recording neither
// rewrites the original application date nor counts a second application.
func TestApplicationFromMailKeepsAnEarlierDate(t *testing.T) {
	f := newAgentInboxFixture(t, "fromMailKeep@example.test")
	jobID := f.applyToJob("acme-already", "applied")
	original, _, _ := f.application(jobID)
	// The fixture seeds user_jobs directly and so never touched the counter;
	// asserting the delta keeps this about the recording, not about the seed.
	before := f.appliedCount(jobID)

	emailID := f.seedEmail(f.userID, "hosted", "fm-2", "Follow-up", "body")
	f.backdate(emailID, time.Now().Add(-2*24*time.Hour))

	if code, body := f.callKey("POST", applicationPath(emailID), map[string]string{"slug": "acme-already"}); code != 200 {
		t.Fatalf("record on an existing application = %d, want 200 (body %v)", code, body)
	}
	after, _, _ := f.application(jobID)
	if !after.Equal(original) {
		t.Errorf("applied_at = %v, want the original %v", after, original)
	}
	if n := f.appliedCount(jobID); n != before {
		t.Errorf("applied_count = %d, want it unchanged at %d — the application already existed", n, before)
	}
}

// TestApplicationFromMailIsIdempotent asserts repeating the call changes nothing.
func TestApplicationFromMailIsIdempotent(t *testing.T) {
	f := newAgentInboxFixture(t, "fromMailIdem@example.test")
	jobID := f.insertJobOnly("acme-idem")
	emailID := f.seedEmail(f.userID, "hosted", "fm-3", "Invite", "body")

	for i := range 2 {
		if code, _ := f.callKey("POST", applicationPath(emailID), map[string]string{"slug": "acme-idem"}); code != 200 {
			t.Fatalf("call %d = %d, want 200", i+1, code)
		}
	}
	if n := f.appliedCount(jobID); n != 1 {
		t.Errorf("applied_count = %d, want 1 after two identical calls", n)
	}
}

// TestApplicationFromMailRefusesPendingSuggestion asserts mail the matcher has
// already proposed an answer for must be confirmed or rejected first, so the
// resulting link's provenance is never ambiguous.
func TestApplicationFromMailRefusesPendingSuggestion(t *testing.T) {
	f := newAgentInboxFixture(t, "fromMailSuggested@example.test")
	suggestedJob := f.applyToJob("acme-suggested", "applied")
	f.insertJobOnly("acme-other")

	emailID := f.seedEmail(f.userID, "hosted", "fm-4", "Ambiguous", "body")
	f.linkEmail(emailID, suggestedJob, true)

	code, _ := f.callKey("POST", applicationPath(emailID), map[string]string{"slug": "acme-other"})
	if code != 409 {
		t.Errorf("record over a pending suggestion = %d, want 409", code)
	}
	if _, _, found := f.application(f.insertJobOnly("acme-unused")); found {
		t.Error("an application was created despite the refusal")
	}

	// Rejecting the suggestion clears the way.
	if code, _ := f.callKey("POST", "/api/v1/me/emails/"+strconv.FormatInt(emailID, 10)+"/reject", nil); code != 200 {
		t.Fatalf("reject suggestion = %d, want 200", code)
	}
	if code, _ := f.callKey("POST", applicationPath(emailID), map[string]string{"slug": "acme-other"}); code != 200 {
		t.Errorf("record after rejecting = %d, want 200", code)
	}
}

// TestApplicationFromMailRefusals covers the scoping and validation failures.
func TestApplicationFromMailRefusals(t *testing.T) {
	f := newAgentInboxFixture(t, "fromMailRefuse@example.test")
	f.insertJobOnly("acme-refuse")
	emailID := f.seedEmail(f.userID, "hosted", "fm-5", "Mine", "body")

	if code, _ := f.callKey("POST", applicationPath(emailID), map[string]string{"slug": "no-such-job"}); code != 404 {
		t.Errorf("unknown slug = %d, want 404", code)
	}
	if code, _ := f.callKey("POST", applicationPath(emailID), map[string]string{}); code != 400 {
		t.Errorf("missing slug = %d, want 400", code)
	}

	var strangerID int64
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ('fromMailStranger@example.test') RETURNING id`).Scan(&strangerID); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	theirs := f.seedEmail(strangerID, "hosted", "fm-6", "Theirs", "body")
	if code, _ := f.callKey("POST", applicationPath(theirs), map[string]string{"slug": "acme-refuse"}); code != 404 {
		t.Errorf("another user's email = %d, want 404", code)
	}

	if code, _ := f.callAnon("POST", applicationPath(emailID)); code != 401 {
		t.Errorf("unauthenticated = %d, want 401", code)
	}
}
