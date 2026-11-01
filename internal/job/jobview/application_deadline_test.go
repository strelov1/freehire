package jobview

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestApplicationDeadlineIsAStatedCalendarDate(t *testing.T) {
	deadline := time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC)
	full, err := FromRow(db.Job{ApplicationDeadline: pgtype.Date{Time: deadline, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	for name, view := range map[string]any{
		"detail": full,
		"card":   NewCard(CardInput{ApplicationDeadline: &deadline}),
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["application_deadline"] != "2026-11-12" {
				t.Fatalf("deadline = %v, want date without time zone conversion", fields["application_deadline"])
			}
		})
	}
}

func TestApplicationDeadlineIsAbsentWhenSourceDoesNotStateOne(t *testing.T) {
	full, err := FromRow(db.Job{})
	if err != nil {
		t.Fatal(err)
	}
	for name, view := range map[string]any{"detail": full, "card": NewCard(CardInput{})} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if _, ok := fields["application_deadline"]; ok {
				t.Fatalf("unknown deadline was estimated: %s", raw)
			}
		})
	}
}
