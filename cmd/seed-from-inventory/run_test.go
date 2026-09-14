package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFailsWithoutWritingOnMalformedInput(t *testing.T) {
	inPath := filepath.Join(t.TempDir(), "in.csv")
	if err := os.WriteFile(inPath, []byte("name,slug\nAcme,acme\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	outDir := filepath.Join(t.TempDir(), "out")

	var stdout bytes.Buffer
	code := run(inPath, outDir, &stdout)

	if code == 0 {
		t.Fatalf("run: want non-zero exit for missing url column, got 0 (stdout: %s)", stdout.String())
	}
	if _, err := os.Stat(outDir); err == nil {
		t.Error("run: expected no output directory to be created on a structural failure")
	}
}

func TestRunPrintsPerProviderSummary(t *testing.T) {
	inPath := filepath.Join(t.TempDir(), "in.csv")
	csv := "name,slug,url\n" +
		"Acme,acme,https://boards.greenhouse.io/acme\n" +
		"Beta,beta,https://careers.vanityco.example\n"
	if err := os.WriteFile(inPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	outDir := filepath.Join(t.TempDir(), "out")

	var stdout bytes.Buffer
	code := run(inPath, outDir, &stdout)

	if code != 0 {
		t.Fatalf("run: want exit 0, got %d (stdout: %s)", code, stdout.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "greenhouse: 1") {
		t.Errorf("stdout %q: want a greenhouse count of 1", got)
	}
	if !strings.Contains(got, "1 row") && !strings.Contains(got, "1 unrecognized") {
		t.Errorf("stdout %q: want the unrecognized-row count reported", got)
	}
}
