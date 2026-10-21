package job

import "testing"

func TestIsDirectApply(t *testing.T) {
	cases := []struct {
		name, source, externalID string
		want                     bool
	}{
		{"employer on-site vacancy", EmployerSource, DirectApplyPrefix + "0b6f", true},
		{"employer vacancy with an external link", EmployerSource, "https://acme.example/jobs/1", false},
		{"prefix on another source is not ours", "manual", DirectApplyPrefix + "0b6f", false},
		{"bare prefix carries no identity", EmployerSource, DirectApplyPrefix, false},
		{"prefix must lead", EmployerSource, "x" + DirectApplyPrefix + "0b6f", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsDirectApply(c.source, c.externalID); got != c.want {
				t.Fatalf("IsDirectApply(%q, %q) = %v, want %v", c.source, c.externalID, got, c.want)
			}
		})
	}
}
