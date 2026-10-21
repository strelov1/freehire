package browser

import (
	"slices"
	"testing"
)

// The stealth pair is the whole point of this package existing rather than each caller
// spelling the flags out, so the test names them rather than counting them.
func TestStealthFlagsCarryThePair(t *testing.T) {
	got := stealthFlags()
	for _, want := range []flag{
		{name: "headless", value: true},
		{name: "disable-blink-features", value: "AutomationControlled"},
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s=%v in %v", want.name, want.value, got)
		}
	}
}
