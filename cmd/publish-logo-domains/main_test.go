package main

import "testing"

func TestOutputPathIsUnsetByDefault(t *testing.T) {
	t.Setenv("LOGO_DOMAIN_MAP_OUT", "")
	if got := outputPath(); got != "" {
		t.Errorf("outputPath() = %q, want empty — the worker ships dark", got)
	}
}

func TestOutputPathReadsItsKnob(t *testing.T) {
	t.Setenv("LOGO_DOMAIN_MAP_OUT", "/var/lib/freehire-logo-map/domains.json")
	if got, want := outputPath(), "/var/lib/freehire-logo-map/domains.json"; got != want {
		t.Errorf("outputPath() = %q, want %q", got, want)
	}
}

func TestOutputPathTrimsWhitespace(t *testing.T) {
	// A trailing newline in an EnvironmentFile line would otherwise become part of the
	// path, and the resulting file would be invisible to the proxy under its real name.
	t.Setenv("LOGO_DOMAIN_MAP_OUT", "  /tmp/domains.json\n")
	if got, want := outputPath(), "/tmp/domains.json"; got != want {
		t.Errorf("outputPath() = %q, want %q", got, want)
	}
}
