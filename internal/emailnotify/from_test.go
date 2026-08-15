package emailnotify_test

import (
	"testing"

	"github.com/strelov1/freehire/internal/emailnotify"
)

func TestFrom(t *testing.T) {
	for _, tc := range []struct {
		name, display, address, want string
	}{
		{"bare address gains a name", "freehire", "notifications@freehire.me", `"freehire" <notifications@freehire.me>`},
		{"existing name is replaced", "Ilya from freehire", `freehire <notifications@freehire.me>`, `"Ilya from freehire" <notifications@freehire.me>`},
		{"no name leaves the address alone", "", "notifications@freehire.me", "notifications@freehire.me"},
		{"unparseable value passes through", "freehire", "not-an-address", "not-an-address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailnotify.From(tc.display, tc.address); got != tc.want {
				t.Errorf("From(%q, %q) = %q, want %q", tc.display, tc.address, got, tc.want)
			}
		})
	}
}
