package sources

import (
	"testing"
	"time"
)

func TestNotFuture(t *testing.T) {
	now := time.Now()
	past := now.Add(-72 * time.Hour)
	soon := now.Add(time.Hour) // within the skew grace (clock skew / timezone-ahead date-only)
	future := now.Add(30 * 24 * time.Hour)

	if got := NotFuture(nil); got != nil {
		t.Errorf("NotFuture(nil) = %v, want nil", got)
	}
	if got := NotFuture(&past); got == nil || !got.Equal(past) {
		t.Errorf("NotFuture(past) = %v, want it unchanged", got)
	}
	if got := NotFuture(&soon); got == nil || !got.Equal(soon) {
		t.Errorf("NotFuture(now+1h) = %v, want it kept within the skew grace", got)
	}
	if got := NotFuture(&future); got != nil {
		t.Errorf("NotFuture(now+30d) = %v, want nil (a posting can't be from the future)", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{"a", "b"}, "a"},
		{[]string{"", "b"}, "b"},
		{[]string{"", "", "c"}, "c"},
		{[]string{"  ", "b"}, "  "}, // exact-empty check: whitespace-only is NOT blank (drop-in for `== ""`)
		{[]string{"", ""}, ""},
		{nil, ""},
		{[]string{" verbatim "}, " verbatim "}, // returned verbatim
	}
	for _, c := range cases {
		if got := firstNonEmpty(c.parts...); got != c.want {
			t.Errorf("firstNonEmpty(%q) = %q, want %q", c.parts, got, c.want)
		}
	}
}
