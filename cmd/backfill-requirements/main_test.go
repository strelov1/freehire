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
	t.Run("only the rows that yield a list are written", func(t *testing.T) {
		ids, payloads := derive([]db.ListJobsForRequirementsBackfillRow{
			row(1, `<h3>Requirements</h3><ul><li>Go</li></ul>`),
			row(2, `<p>Just some prose about the role.</p>`),
			row(3, `<h3>What we offer</h3><ul><li>Free lunch</li></ul>`),
			row(4, ``),
			row(5, `<h3>Nice to have</h3><ul><li>Rust</li></ul>`),
		})

		if want := []int64{1, 5}; !equalIDs(ids, want) {
			t.Errorf("ids = %v, want %v — a row yielding nothing already holds [] and "+
				"sending it would only make the batch bigger", ids, want)
		}
		if len(payloads) != len(ids) {
			t.Fatalf("payloads = %d, ids = %d: the two arrays are unnested in step", len(payloads), len(ids))
		}
	})

	t.Run("the payload is a requirements list the readers can decode", func(t *testing.T) {
		_, payloads := derive([]db.ListJobsForRequirementsBackfillRow{
			row(1, `<h3>Requirements</h3><ul><li>5+ years of Go</li></ul>
			        <h3>Nice to have</h3><ul><li>Kubernetes</li></ul>`),
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
