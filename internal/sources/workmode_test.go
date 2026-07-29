package sources

import "testing"

func TestWorkModeFromRemote(t *testing.T) {
	if got := workModeFromRemote(true); got != "remote" {
		t.Errorf("workModeFromRemote(true) = %q, want remote", got)
	}
	// A false flag does not imply onsite vs hybrid — leave it unknown.
	if got := workModeFromRemote(false); got != "" {
		t.Errorf("workModeFromRemote(false) = %q, want empty", got)
	}
}

func TestWorkplaceTypeMode(t *testing.T) {
	cases := map[string]string{
		"remote":      "remote",
		"hybrid":      "hybrid",
		"on-site":     "onsite",
		"onsite":      "onsite",
		"On-Site":     "onsite",
		"unspecified": "",
		"":            "",
		"weird":       "",
	}
	for in, want := range cases {
		if got := workplaceTypeMode(in); got != want {
			t.Errorf("workplaceTypeMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkModeFromRemoteHybrid(t *testing.T) {
	cases := []struct {
		remote, hybrid bool
		want           string
	}{
		{remote: true, hybrid: false, want: "remote"},
		{remote: false, hybrid: true, want: "hybrid"},
		// Both false is "not marked", which an ATS cannot distinguish from "office" —
		// so it stays unknown rather than becoming a guessed onsite.
		{remote: false, hybrid: false, want: ""},
		// Around 2% of Recruitee offers set both, and Recruitee renders them as
		// "Remote job" — the broader arrangement wins.
		{remote: true, hybrid: true, want: "remote"},
	}
	for _, c := range cases {
		if got := workModeFromRemoteHybrid(c.remote, c.hybrid); got != c.want {
			t.Errorf("workModeFromRemoteHybrid(%v, %v) = %q, want %q", c.remote, c.hybrid, got, c.want)
		}
	}
}
