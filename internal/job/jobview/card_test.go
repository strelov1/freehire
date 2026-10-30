package jobview

import (
	"encoding/json"
	"strings"
	"testing"
)

// A card carries the posting's location line, trimmed, and omits the field when the
// posting states none — an empty caption must be an absent one.
func TestCard_LocationRoundTrips(t *testing.T) {
	if got := NewCard(CardInput{PublicSlug: "a", Location: "  Zagreb  "}).Location; got != "Zagreb" {
		t.Fatalf("card location = %q, want %q", got, "Zagreb")
	}
	raw, err := json.Marshal(NewCard(CardInput{PublicSlug: "b"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"location"`) {
		t.Fatalf("card without a location serialized the field: %s", raw)
	}
}
