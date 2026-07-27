package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBoardFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// The company-scoped rules have no ingest counterpart, so a deletion under one is
// undone by the next hourly crawl unless the board is struck from the source files.
// The worker therefore has to know which companies are still listed — matched by the
// same slug normalization ingest writes, or the guard would compare different strings.
func TestListedCompaniesMatchesIngestSlugging(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", `
- company: "Acme Corp."
  board: acme
- company: "Beta & Co"
  board: beta
`)
	writeBoardFile(t, dir, "custom.yml", `
- company: "Gamma"
  provider: lever
  board: gamma
`)
	// Not a board file: must not be read as one.
	writeBoardFile(t, dir, "README.md", "not yaml")

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}

	for _, want := range []boardKey{
		{"greenhouse", "acme"}, {"greenhouse", "beta"}, {"lever", "gamma"},
	} {
		if !b.listed[want] {
			t.Errorf("%+v not listed; got %v", want, b.listed)
		}
	}
	if len(b.listed) != 3 {
		t.Errorf("listed %d entries, want 3 — README.md is not a board file", len(b.listed))
	}
	// A posting is matched to its board through the namespaced external_id.
	if !b.crawls("greenhouse", "acme:12345") {
		t.Error("a posting of a listed board must read as crawled")
	}
	// The same board id under another provider is a different board.
	if b.crawls("workday", "acme:12345") {
		t.Error("a board listed under greenhouse must not shield the same id under workday")
	}
}

// A slug the source files do not mention is retired, and only those may be deleted
// under a company-scoped rule.
func TestListedCompaniesOmitsUnlisted(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: Acme\n  board: acme\n")

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}
	if b.crawls("greenhouse", "retired-co:1") {
		t.Error("a board absent from every file must not read as crawled")
	}
	// A link-source import or a moderator row carries a real provider but no listed
	// board, and nothing re-crawls it — so it must never read as crawled.
	if b.crawls("greenhouse", "some-unlisted-board:99") {
		t.Error("an unlisted board must not read as crawled, whatever its provider")
	}
}

// A source directory that cannot be read must stop the run rather than yield an empty
// set: an empty set reads as "every board is retired", which would let the company
// rules delete the whole catalogue.
func TestListedCompaniesFailsClosed(t *testing.T) {
	if _, err := loadBoards(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("an unreadable source directory must be an error, not an empty listing — empty means every board is retired")
	}
}

// The guard runs against the real sources/ directory on every invocation, and that
// directory holds files that are not board lists — telegram.yml is the channel list for
// cmd/tg-ingest and does not parse as one. A fixture-only test cannot see that, and did
// not: --boards errored on every real run until this case existed.
func TestLoadBoardsReadsTheRealSourcesDirectory(t *testing.T) {
	b, err := loadBoards("../../sources")
	if err != nil {
		t.Fatalf("loadBoards on the real directory: %v", err)
	}
	if len(b.listed) < 100 {
		t.Errorf("listed %d entries, want the real catalogue's boards", len(b.listed))
	}
	if b.byProvider["greenhouse"] == nil {
		t.Error("greenhouse must have boards")
	}
	for _, notCrawled := range []string{"telegram", "manual", ""} {
		if b.byProvider[notCrawled] != nil {
			t.Errorf("%q is not a board provider and must have no boards", notCrawled)
		}
	}
}

// Retired boards live in sources/retired/, and the guard depends on not seeing them: a
// glob that descended into subdirectories would read them as live, and the rules that
// require a retired board would quietly stop firing.
func TestLoadBoardsIgnoresRetiredSubdirectory(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: Live Co\n  board: live\n")
	if err := os.MkdirAll(filepath.Join(dir, "retired"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeBoardFile(t, filepath.Join(dir, "retired"), "greenhouse.yml", "- company: Gone Co\n  board: gone\n")

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}
	if !b.crawls("greenhouse", "live:1") {
		t.Error("the live board must still read as crawled")
	}
	if b.crawls("greenhouse", "gone:1") {
		t.Error("a retired board must not read as crawled — moving it out of sources/ is what retires it")
	}
}
