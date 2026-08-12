package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestRunAcceptsCleanDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "greenhouse.yml", `
- company: Cohere
  board: cohere
- company: Stripe
  board: stripe
`)
	writeFile(t, dir, "telegram.yml", `
channels:
  - channel: hrlunapark
    kind: authored
  - channel: it_vakansii_jobs
    kind: board
`)

	problems, checked, err := run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("problems = %v, want none", problems)
	}
	if checked != 2 {
		t.Errorf("checked = %d, want 2", checked)
	}
}

// A board entered twice under different casing is exactly what internal/sources'
// dedupeBoards silently drops at ingest time; the validator's job is to fail loudly on
// it instead so it never reaches main.
func TestRunCatchesCaseVariantDuplicateBoard(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "smartrecruiters.yml", `
- company: Agile IT
  board: AgileIT
- company: AgileIT
  board: agileit
`)

	problems, _, err := run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "duplicate board") {
		t.Fatalf("problems = %v, want one duplicate-board problem", problems)
	}
}

func TestRunCatchesEmptyCompany(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "greenhouse.yml", "- board: cohere\n")

	problems, _, err := run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "empty company") {
		t.Fatalf("problems = %v, want one empty-company problem", problems)
	}
}

func TestRunCatchesUnknownField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "greenhouse.yml", "- company: Cohere\n  boad: cohere\n")

	problems, _, err := run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(problems) != 1 {
		t.Fatalf("problems = %v, want one parse problem for the typo'd field", problems)
	}
}

func TestRunCatchesUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notaprovider.yml", "- company: Cohere\n  board: cohere\n")

	problems, _, err := run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "unknown provider") {
		t.Fatalf("problems = %v, want one unknown-provider problem", problems)
	}
}

// telegram.yml carries its own schema and its own Validate — a duplicate channel is its
// analogue of a duplicate board, and the directory-wide validator must surface it too.
func TestRunCatchesDuplicateTelegramChannel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "telegram.yml", `
channels:
  - channel: hrlunapark
    kind: authored
  - channel: hrlunapark
    kind: board
`)

	problems, _, err := run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "duplicate channel") {
		t.Fatalf("problems = %v, want one duplicate-channel problem", problems)
	}
}

func TestRunErrorsOnEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	if _, _, err := run(dir); err == nil {
		t.Fatal("expected an error for a directory with no board files, got nil")
	}
}
