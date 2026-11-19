package applicationdigest

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

func TestFollowupsExcludePendingAndTakeFiveOldest(t *testing.T) {
	now := time.Date(2030, 1, 7, 12, 0, 0, 0, time.UTC)
	var rows []db.ListApplicationDigestSilentCandidatesRow
	for i := int64(0); i < 8; i++ {
		rows = append(rows, db.ListApplicationDigestSilentCandidatesRow{ID: i, Stage: "applied", AppliedAt: pgconv.Instant(now.AddDate(0, 0, -40)), LastActivityAt: pgconv.Instant(now.AddDate(0, 0, -30-int(i))), HasPendingSuggestion: i == 7})
	}
	got := selectFollowups(rows, now)
	if len(got) != 5 || got[0].ID != 6 || got[4].ID != 2 {
		t.Fatalf("followups=%+v", got)
	}
	rows[0].AppliedAt = pgtype.Timestamptz{}
	if got := selectFollowups(rows[:1], now); len(got) != 0 {
		t.Fatal("unapplied row included")
	}
}
func TestWeekUsesLocalMonday(t *testing.T) {
	at := time.Date(2030, 1, 7, 1, 0, 0, 0, time.UTC)
	got := localWeek(Candidate{Timezone: "America/Sao_Paulo"}, at)
	if got.Time.Format("2006-01-02") != "2029-12-31" {
		t.Fatal(got)
	}
}
