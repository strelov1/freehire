package mailtpl

import "testing"

func TestNewJobAsksTheProxyForAMonogramFallback(t *testing.T) {
	j := NewJob("Go engineer", "Acme Co", "", "https://freehire.me/j/1")
	if want := "https://logo.freehire.me/Acme%20Co?fallback=monogram"; j.LogoURL != want {
		t.Errorf("LogoURL = %q, want %q", j.LogoURL, want)
	}
	if j.Monogram != "A" {
		t.Errorf("Monogram = %q, want A", j.Monogram)
	}
	if NewJob("Go engineer", "", "", "").LogoURL != "" {
		t.Error("a job without a company must have no logo tile")
	}
}
