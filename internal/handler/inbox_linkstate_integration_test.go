//go:build integration

// Integration tests for the inbox link-state filter against a real Postgres: the
// three states each narrow the listing, they partition the mailbox, and the
// filter composes with the existing label filter. The pagination total is
// asserted alongside every listing — a predicate applied to the page query but
// not to its count reports a filtered page with an unfiltered total. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"testing"
)

// linkEmail attaches an email to an application, or (when suggested) records a
// pending suggestion instead of a link.
func (f *harnessInboxFixture) linkEmail(emailID, jobID int64, suggested bool) {
	f.t.Helper()
	if suggested {
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE emails SET suggested_job_id = $1 WHERE id = $2`, jobID, emailID); err != nil {
			f.t.Fatalf("suggest email %d: %v", emailID, err)
		}
		return
	}
	// A link is to the application; the posting is where it was found. Setting job_id
	// alone is what the link paths stopped doing, so a fixture that did it would test a
	// state the product can no longer produce.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE emails
		    SET job_id         = $1,
		        application_id = (SELECT a.id FROM applications a
		                           WHERE a.user_id = emails.user_id AND a.job_id = $1)
		  WHERE emails.id = $2`, jobID, emailID); err != nil {
		f.t.Fatalf("link email %d: %v", emailID, err)
	}
}

// classify stamps a message with a status signal, so link state can be combined
// with the label filter.
func (f *harnessInboxFixture) classify(emailID int64, signal string) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE emails SET status_signal = $1, classified_at = now() WHERE id = $2`,
		signal, emailID); err != nil {
		f.t.Fatalf("classify email %d: %v", emailID, err)
	}
}

// listLinkState requests the listing under one link state and returns the message
// ids it produced together with the reported pagination total.
func (f *harnessInboxFixture) listLinkState(query string) ([]int64, int) {
	f.t.Helper()
	code, body := f.callKey("GET", "/api/v1/me/inbox?"+query, nil)
	if code != 200 {
		f.t.Fatalf("listing ?%s = %d, want 200", query, code)
	}
	ids := make([]int64, 0)
	for _, m := range messages(f.t, body) {
		id, _ := m["id"].(float64)
		ids = append(ids, int64(id))
	}
	meta, _ := body["meta"].(map[string]any)
	total, _ := meta["total"].(float64)
	return ids, int(total)
}

// TestInboxLinkStateFilter asserts the three link states narrow the listing, that
// each narrowing is reflected in the pagination total, and that together they
// partition the caller's mail.
func TestInboxLinkStateFilter(t *testing.T) {
	f := newHarnessInboxFixture(t, "linkstate@example.test")
	jobID := f.applyToJob("acme-go-dev", "applied")

	linked := f.seedEmail(f.userID, "hosted", "ls-1", "Linked", "body")
	suggested := f.seedEmail(f.userID, "hosted", "ls-2", "Suggested", "body")
	orphanA := f.seedEmail(f.userID, "hosted", "ls-3", "Orphan A", "body")
	orphanB := f.seedEmail(f.userID, "hosted", "ls-4", "Orphan B", "body")

	f.linkEmail(linked, jobID, false)
	f.linkEmail(suggested, jobID, true)

	_, unfiltered := f.listLinkState("")
	if unfiltered != 4 {
		t.Fatalf("unfiltered total = %d, want 4", unfiltered)
	}

	cases := []struct {
		state string
		want  []int64
	}{
		{"linked", []int64{linked}},
		{"suggested", []int64{suggested}},
		{"unlinked", []int64{orphanB, orphanA}}, // newest first
	}
	seen := map[int64]string{}
	sum := 0
	for _, tc := range cases {
		ids, total := f.listLinkState("link=" + tc.state)
		if total != len(tc.want) {
			t.Errorf("?link=%s total = %d, want %d", tc.state, total, len(tc.want))
		}
		if len(ids) != len(tc.want) {
			t.Errorf("?link=%s returned %d messages, want %d", tc.state, len(ids), len(tc.want))
			continue
		}
		for i, id := range ids {
			if id != tc.want[i] {
				t.Errorf("?link=%s message %d = %d, want %d", tc.state, i, id, tc.want[i])
			}
			if prev, dup := seen[id]; dup {
				t.Errorf("message %d appears in both %s and %s listings", id, prev, tc.state)
			}
			seen[id] = tc.state
		}
		sum += total
	}
	// The three states partition the mailbox: every message lands in exactly one.
	if sum != unfiltered {
		t.Errorf("link-state totals sum to %d, want the unfiltered total %d", sum, unfiltered)
	}
}

// TestMarkAllReadHonoursLinkState asserts mark-all-read respects the link filter
// like every other active filter. Without this, a caller working through the
// confirmation queue who presses "mark all read" would silently mark their whole
// mailbox read — the filter would narrow what they see but not what they act on.
func TestMarkAllReadHonoursLinkState(t *testing.T) {
	f := newHarnessInboxFixture(t, "linkmarkall@example.test")
	jobID := f.applyToJob("acme-markall", "applied")

	suggested := f.seedEmail(f.userID, "hosted", "ma-1", "Suggested", "body")
	orphan := f.seedEmail(f.userID, "hosted", "ma-2", "Orphan", "body")
	f.linkEmail(suggested, jobID, true)

	code, body := f.callKey("POST", "/api/v1/me/inbox/read-all?link=suggested", nil)
	if code != 200 {
		t.Fatalf("mark-all-read = %d, want 200", code)
	}
	data, _ := body["data"].(map[string]any)
	if marked, _ := data["marked"].(float64); int(marked) != 1 {
		t.Errorf("marked = %v, want 1 (only the suggested message)", marked)
	}

	var unread bool
	if err := f.pool.QueryRow(context.Background(),
		`SELECT read_at IS NULL FROM emails WHERE id = $1`, orphan).Scan(&unread); err != nil {
		t.Fatalf("read orphan state: %v", err)
	}
	if !unread {
		t.Error("a message outside the link filter was marked read")
	}
}

// TestInboxLinkStateComposesWithLabel asserts the link filter narrows alongside
// the classification label rather than replacing it, in both the page and the
// total.
func TestInboxLinkStateComposesWithLabel(t *testing.T) {
	f := newHarnessInboxFixture(t, "linkcompose@example.test")
	jobID := f.applyToJob("acme-compose", "applied")

	linkedRejection := f.seedEmail(f.userID, "hosted", "lc-1", "Linked rejection", "body")
	orphanRejection := f.seedEmail(f.userID, "hosted", "lc-2", "Orphan rejection", "body")
	orphanInvite := f.seedEmail(f.userID, "hosted", "lc-3", "Orphan invite", "body")

	f.linkEmail(linkedRejection, jobID, false)
	f.classify(linkedRejection, "rejection")
	f.classify(orphanRejection, "rejection")
	f.classify(orphanInvite, "interview_invitation")

	ids, total := f.listLinkState("link=unlinked&status=rejection")
	if total != 1 {
		t.Errorf("composed total = %d, want 1", total)
	}
	if len(ids) != 1 || ids[0] != orphanRejection {
		t.Errorf("composed listing = %v, want [%d]", ids, orphanRejection)
	}
}
