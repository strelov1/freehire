package rederive

import "testing"

// Postgres re-spaces jsonb on read, so a byte compare would flag every stored list as moved.
func TestRequirementsMoved(t *testing.T) {
	derived := []byte(`[{"text":"Go","priority":"required"}]`)
	for name, tc := range map[string]struct {
		stored []byte
		want   bool
	}{
		"same list, jsonb spacing": {[]byte(`[{"priority": "required", "text": "Go"}]`), false},
		"different list":           {[]byte(`[]`), true},
		"unreadable stored":        {[]byte(`{`), true},
	} {
		if got := requirementsMoved(derived, tc.stored); got != tc.want {
			t.Errorf("%s: requirementsMoved = %v, want %v", name, got, tc.want)
		}
	}
}
