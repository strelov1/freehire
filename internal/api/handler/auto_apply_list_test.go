package handler

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestProjectAutoApplyListSubmissionBetweenReads(t *testing.T) {
	started := time.Date(2026, time.November, 14, 12, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	// A submission deletes its queue row after the live read but before the history read.
	live := []db.ListAutoApplyEntriesRow{
		{PublicSlug: "completed-job", Title: "Engineer", CreatedAt: pgtype.Timestamptz{Time: started, Valid: true}},
		{PublicSlug: "pending-job", Title: "Designer", CreatedAt: pgtype.Timestamptz{Time: started, Valid: true}},
	}
	submitted := []db.ListRecentAutoApplySubmissionsRow{
		{PublicSlug: "completed-job", Title: "Engineer", CreatedAt: pgtype.Timestamptz{Time: finished, Valid: true}},
	}
	got := projectAutoApplyList(live, submitted).Attempts
	if len(got) != 2 {
		t.Fatalf("attempts = %+v, want one row per job", got)
	}
	if got[0].JobSlug != "completed-job" || got[0].Status != autoApplyStatusSubmitted || !got[0].LastActivityAt.Equal(finished) {
		t.Errorf("newest attempt = %+v, want submitted at %s", got[0], finished)
	}
	if got[1].JobSlug != "pending-job" {
		t.Errorf("remaining live attempt = %+v", got[1])
	}
}
