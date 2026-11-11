package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/api/apigen"
)

// A contract timestamp keeps its fraction, as encoding/json wrote it before the contract:
// a client that hands one back as a cursor must land on the same row. The layout comes
// from scripts/openapi-time-format.mjs; without it ogen writes whole seconds.
func TestContractTimestampsKeepTheirFraction(t *testing.T) {
	at := time.Date(2026, 11, 11, 17, 0, 0, 123456789, time.UTC)
	raw, err := (&apigen.WatchedCompany{Slug: "acme", Name: "Acme", AddedAt: at}).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := `"added_at":"2026-11-11T17:00:00.123456789Z"`; !strings.Contains(string(raw), want) {
		t.Errorf("encoded %s, want %s", raw, want)
	}

	var back apigen.WatchedCompany
	if err := back.UnmarshalJSON([]byte(`{"slug":"a","name":"A","added_at":"2026-11-11T17:00:00Z"}`)); err != nil {
		t.Errorf("a timestamp without a fraction must still parse: %v", err)
	}
}
