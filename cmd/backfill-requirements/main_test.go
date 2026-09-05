package main

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
)

func row(id int64, description string) db.ListJobsForRequirementsBackfillRow {
	return db.ListJobsForRequirementsBackfillRow{ID: id, Description: description}
}

// derive decides what one chunk actually writes. Everything the pass costs — the batch
// size, the dead tuples, the time — follows from how narrow this is.
func TestDerive(t *testing.T) {
	// Every row is written, including the ones that now yield nothing. That is what
	// makes the worker a repair path: a heading-vocabulary fix that removes a false
	// positive reaches a stored row through this pass and through nothing else.
	t.Run("every row is written, and a row yielding nothing is cleared", func(t *testing.T) {
		ids, payloads := derive([]db.ListJobsForRequirementsBackfillRow{
			row(1, `<h3>Requirements</h3><ul><li>Go</li></ul>`),
			row(2, `<p>Just some prose about the role.</p>`),
			row(3, `<h3>What we offer</h3><ul><li>Free lunch</li></ul>`),
			row(4, ``),
			row(5, `<h3>Nice to have</h3><ul><li>Rust</li></ul>`),
		})

		if want := []int64{1, 2, 3, 4, 5}; !equalIDs(ids, want) {
			t.Errorf("ids = %v, want %v", ids, want)
		}
		if len(payloads) != len(ids) {
			t.Fatalf("payloads = %d, ids = %d: the two arrays are unnested in step", len(payloads), len(ids))
		}
		for i, id := range ids {
			wantEmpty := id != 1 && id != 5
			if got := string(payloads[i]) == "[]"; got != wantEmpty {
				t.Errorf("job %d payload = %s, wantEmpty=%v", id, payloads[i], wantEmpty)
			}
		}
	})

	t.Run("the payload is a requirements list the readers can decode", func(t *testing.T) {
		_, payloads := derive([]db.ListJobsForRequirementsBackfillRow{
			row(1, `<h3>Requirements</h3><ul><li>5+ years of Go</li></ul>`+
				`<h3>Nice to have</h3><ul><li>Kubernetes</li></ul>`),
		})
		if len(payloads) != 1 {
			t.Fatalf("payloads = %d, want 1", len(payloads))
		}

		var got []enrich.Requirement
		if err := json.Unmarshal(payloads[0], &got); err != nil {
			t.Fatalf("payload is not a requirements list: %v", err)
		}
		want := []enrich.Requirement{
			{Text: "5+ years of Go", Priority: "required"},
			{Text: "Kubernetes", Priority: "preferred"},
		}
		if len(got) != len(want) {
			t.Fatalf("payload = %+v, want %+v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("payload[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("an empty chunk writes nothing", func(t *testing.T) {
		ids, payloads := derive(nil)
		if len(ids) != 0 || len(payloads) != 0 {
			t.Errorf("derive(nil) = %v/%v, want empty", ids, payloads)
		}
	})
}

// A knob that is set but unreadable fails the run rather than falling back. A typo in
// BACKFILL_REQUIREMENTS_FROM_ID would otherwise silently re-walk the whole table and
// look exactly like an ordinary run — the same reasoning HYDRATION_RETRY_DAYS uses.
func TestEnvInt64(t *testing.T) {
	const knob = "BACKFILL_REQUIREMENTS_TEST_KNOB"

	t.Run("unset takes the default", func(t *testing.T) {
		got, err := envInt64(knob, 42)
		if err != nil || got != 42 {
			t.Errorf("envInt64 = %d, %v; want 42, nil", got, err)
		}
	})

	t.Run("empty takes the default", func(t *testing.T) {
		t.Setenv(knob, "   ")
		got, err := envInt64(knob, 42)
		if err != nil || got != 42 {
			t.Errorf("envInt64 = %d, %v; want 42, nil", got, err)
		}
	})

	t.Run("a positive value is used", func(t *testing.T) {
		t.Setenv(knob, " 7 ")
		got, err := envInt64(knob, 42)
		if err != nil || got != 7 {
			t.Errorf("envInt64 = %d, %v; want 7, nil", got, err)
		}
	})

	for _, bad := range []string{"abc", "0", "-1", "1_000"} {
		t.Run("a set but unusable value fails: "+bad, func(t *testing.T) {
			t.Setenv(knob, bad)
			if _, err := envInt64(knob, 42); err == nil {
				t.Errorf("envInt64(%q) = nil error, want a failure rather than the default", bad)
			}
		})
	}
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
