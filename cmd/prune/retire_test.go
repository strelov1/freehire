package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// retireBoards edits the source files in place, so every property here is about NOT
// losing something: the file's own comments, the entries that stay, and the provider
// whose last entry must not leave before its jobs do.

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// A board file is hand-maintained YAML: a header explaining where the board id comes
// from, and group comments above runs of entries. A YAML round-trip would silently
// drop all of it, so the move is line-based — the entry's own lines leave, everything
// else stays byte for byte.
func TestRetireBoardsPreservesCommentsAndOtherEntries(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", `# greenhouse boards. Provider is the filename.
# board = the tenant slug.

- company: Live Co
  board: live
- company: Dead Co
  board: dead
- company: Other Co
  board: other
`)

	moved, _, err := retireBoards(dir, []boardKey{{"greenhouse", "dead"}})
	if err != nil {
		t.Fatalf("retireBoards: %v", err)
	}
	if moved != 1 {
		t.Errorf("moved %d entries, want 1", moved)
	}

	src := readFile(t, filepath.Join(dir, "greenhouse.yml"))
	if !strings.Contains(src, "# greenhouse boards. Provider is the filename.") {
		t.Errorf("the file header must survive; got:\n%s", src)
	}
	if strings.Contains(src, "Dead Co") || strings.Contains(src, "board: dead") {
		t.Errorf("the retired entry must be gone from sources/; got:\n%s", src)
	}
	for _, keep := range []string{"Live Co", "board: live", "Other Co", "board: other"} {
		if !strings.Contains(src, keep) {
			t.Errorf("%q must stay; got:\n%s", keep, src)
		}
	}

	got := readFile(t, filepath.Join(dir, "retired", "greenhouse.yml"))
	if !strings.Contains(got, "company: Dead Co") || !strings.Contains(got, "board: dead") {
		t.Errorf("the entry must arrive whole in retired/; got:\n%s", got)
	}
}

// An entry naming its own provider (custom.yml) keeps that line, and lands in the
// retired file mirroring its SOURCE FILE — provider resolution is "the entry's own
// provider, else the filename", so mirroring the filename keeps the entry valid where
// it lands.
func TestRetireBoardsKeepsAnEntrysOwnProvider(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "custom.yml", `# single-source configs
- company: Chainlink Labs
  provider: ashbygraphql
  board: chainlink-labs
- company: Toggl
  provider: ashbygraphql
  board: toggl
`)

	if _, _, err := retireBoards(dir, []boardKey{{"ashbygraphql", "toggl"}}); err != nil {
		t.Fatalf("retireBoards: %v", err)
	}

	got := readFile(t, filepath.Join(dir, "retired", "custom.yml"))
	if !strings.Contains(got, "provider: ashbygraphql") || !strings.Contains(got, "board: toggl") {
		t.Errorf("the entry must keep its own provider line; got:\n%s", got)
	}
	if strings.Contains(readFile(t, filepath.Join(dir, "custom.yml")), "board: toggl") {
		t.Error("the retired entry must leave custom.yml")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, "custom.yml")), "board: chainlink-labs") {
		t.Error("the other entry must stay")
	}
}

// The one-way door. cmd/ingest takes a board file by path, so a provider with nothing
// left in sources/ is never crawled again — and the company-scoped rules refuse a job
// they cannot re-crawl, so its postings can never be pruned either. Moving the last
// entry before its jobs are gone makes the dead weight permanent, so the mover refuses
// and says so; the report's CAUTION line names the same providers.
func TestRetireBoardsRefusesToEmptyAProvider(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "tinyats.yml", "- company: One\n  board: one\n- company: Two\n  board: two\n")

	moved, _, err := retireBoards(dir, []boardKey{{"tinyats", "one"}, {"tinyats", "two"}})
	if err != nil {
		t.Fatalf("retireBoards: %v", err)
	}
	if moved != 0 {
		t.Errorf("moved %d entries, want 0 — emptying a provider is irreversible", moved)
	}
	src := readFile(t, filepath.Join(dir, "tinyats.yml"))
	for _, keep := range []string{"board: one", "board: two"} {
		if !strings.Contains(src, keep) {
			t.Errorf("%q must stay until the provider's jobs are pruned; got:\n%s", keep, src)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "retired", "tinyats.yml")); !os.IsNotExist(err) {
		t.Error("nothing was moved, so no retired file should have been created")
	}
}

// A second wave appends to the file the first wave created rather than replacing it —
// the point of retired/ is that it accumulates what was considered.
func TestRetireBoardsAppendsToAnExistingRetiredFile(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: A\n  board: a\n- company: B\n  board: b\n- company: C\n  board: c\n")
	if err := os.MkdirAll(filepath.Join(dir, "retired"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeBoardFile(t, filepath.Join(dir, "retired"), "greenhouse.yml", "- company: Earlier\n  board: earlier\n")

	if _, _, err := retireBoards(dir, []boardKey{{"greenhouse", "a"}}); err != nil {
		t.Fatalf("retireBoards: %v", err)
	}

	got := readFile(t, filepath.Join(dir, "retired", "greenhouse.yml"))
	if !strings.Contains(got, "board: earlier") {
		t.Errorf("the earlier wave must survive; got:\n%s", got)
	}
	if !strings.Contains(got, "board: a") {
		t.Errorf("the new entry must be appended; got:\n%s", got)
	}
}

// A board named in the list but absent from the files is not an error — the report and
// the move can be minutes apart, and another PR may have retired it already. It must
// not take the run down with it, and it must not be counted as moved.
func TestRetireBoardsIgnoresAnEntryThatIsAlreadyGone(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: A\n  board: a\n- company: B\n  board: b\n")

	moved, _, err := retireBoards(dir, []boardKey{{"greenhouse", "vanished"}})
	if err != nil {
		t.Fatalf("a board that is already gone must not be an error: %v", err)
	}
	if moved != 0 {
		t.Errorf("moved %d, want 0", moved)
	}
}

// A refusal that does not say so is indistinguishable from "there was nothing to do".
// The operator has to learn WHICH provider was held back, because the answer is not
// "never move it" — it is "prune its jobs first, then move it deliberately".
func TestRetireBoardsReportsWhatItRefusedToEmpty(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "tinyats.yml", "- company: One\n  board: one\n- company: Two\n  board: two\n")
	writeBoardFile(t, dir, "greenhouse.yml", "- company: A\n  board: a\n- company: B\n  board: b\n")

	moved, held, err := retireBoards(dir, []boardKey{
		{"tinyats", "one"}, {"tinyats", "two"}, {"greenhouse", "a"},
	})
	if err != nil {
		t.Fatalf("retireBoards: %v", err)
	}
	if moved != 1 {
		t.Errorf("moved %d, want 1 — greenhouse keeps a board, so its entry moves", moved)
	}
	if len(held) != 1 || held[0] != "tinyats" {
		t.Errorf("held back %v, want [tinyats] — the caller cannot explain the refusal otherwise", held)
	}
}

// Fixtures prove the algorithm; the real directory proves the assumptions. These files
// are 140-odd hand-maintained YAML documents, some of them 400KB, with headers, group
// comments, quoted company names containing "&" and ":", and custom.yml entries that
// name their own provider. The strongest check is a round trip: whatever the move
// leaves behind must still load through the real parser, because a file that stops
// parsing takes the whole prune run down with it — loadBoards fails closed.
func TestRetireBoardsOnACopyOfTheRealSources(t *testing.T) {
	src := "../../sources"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("real sources not available: %v", err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatalf("copy sources: %v", err)
	}

	before, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards on the copy: %v", err)
	}
	// Two entries of different shapes: a plain one whose provider is the file name, and
	// a custom.yml one carrying its own provider line.
	var plain, own boardKey
	for k := range before.listed {
		if k.Provider == "betterteam" && plain.Board == "" {
			plain = k
		}
		if k.Provider == "ashbygraphql" && own.Board == "" {
			own = k
		}
	}
	if plain.Board == "" || own.Board == "" {
		t.Skip("the real files no longer carry the shapes this test samples")
	}

	moved, _, err := retireBoards(dir, []boardKey{plain, own})
	if err != nil {
		t.Fatalf("retireBoards: %v", err)
	}
	if moved != 2 {
		t.Fatalf("moved %d entries, want 2", moved)
	}

	after, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("the edited files must still parse: %v", err)
	}
	if after.listed[plain] || after.listed[own] {
		t.Error("a moved entry must no longer read as listed")
	}
	if got, want := len(after.listed), len(before.listed)-2; got != want {
		t.Errorf("listed %d entries after the move, want %d — nothing else may leave", got, want)
	}
	// The retired copies live beside, not instead: a glob one level up must not see them,
	// which is exactly what makes the move a retirement.
	for _, name := range []string{"betterteam.yml", "custom.yml"} {
		if _, err := os.Stat(filepath.Join(dir, "retired", name)); err != nil {
			t.Errorf("retired/%s must exist: %v", name, err)
		}
	}
}
