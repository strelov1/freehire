package jobview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// AIInterviewReports is served straight from the jobs column — a denormalized copy of
// the owning company's counter, written in the same transaction as the report so a card
// never disagrees with the company page.
func TestFromRow_AIInterviewReportsFromColumn(t *testing.T) {
	got, err := FromRow(db.Job{ID: 1, AiInterviewReports: 7})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if got.AIInterviewReports != 7 {
		t.Fatalf("AIInterviewReports = %d, want 7", got.AIInterviewReports)
	}
}

// Zero reports must be the ABSENCE of the field, not a zero. The label is required to
// be shown with its count, so a rendered `"ai_interview_reports": 0` invites a badge
// that says a practice was reported when nobody reported it.
func TestFromRow_AIInterviewReportsOmittedAtZero(t *testing.T) {
	got, err := FromRow(db.Job{ID: 1})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if got.AIInterviewReports != 0 {
		t.Fatalf("AIInterviewReports = %d, want 0", got.AIInterviewReports)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "ai_interview_reports") {
		t.Fatalf("zero count was serialized; payload must omit it: %s", raw)
	}
}

// The card is the list surface, and it carries the same field under the same rule —
// a badge on a card is exactly where a rendered zero would be seen.
func TestCard_AIInterviewReportsRoundTrips(t *testing.T) {
	labelled := NewCard(CardInput{PublicSlug: "a", AIInterviewReports: 2})
	if labelled.AIInterviewReports != 2 {
		t.Fatalf("card count = %d, want 2", labelled.AIInterviewReports)
	}
	raw, err := json.Marshal(NewCard(CardInput{PublicSlug: "b"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "ai_interview_reports") {
		t.Fatalf("unlabelled card serialized the field: %s", raw)
	}
}
