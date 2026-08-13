package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// retireBoards moves the named entries out of sources/<file>.yml and into
// sources/retired/<file>.yml — the move IS the retirement, since cmd/ingest takes a
// board file by path and cmd/prune globs one level, so neither sees what lives in the
// subdirectory. It returns how many entries it moved.
//
// The edit is line-based rather than a YAML round-trip, and deliberately so: the board
// files are hand-maintained, carrying a header that records where a provider's board id
// comes from and group comments above runs of entries. Re-serialising the parsed config
// would drop every one of them and reflow the rest, burying the actual change in a
// diff nobody can review. Here an entry's own lines leave and the file is otherwise
// untouched, byte for byte.
//
// It refuses to move a provider's last entries. That is the one irreversible step in a
// reversible operation: a provider with nothing left in sources/ is never crawled again,
// and the company-scoped rules refuse a job they cannot re-crawl, so its postings become
// permanently un-prunable. Prune those jobs first, then move the last entry deliberately.
// It returns those files' names alongside the count, because a silent refusal reads as
// "there was nothing to do" — and the answer is not "never move it", it is "later".
func retireBoards(dir string, retire []boardKey) (int, []string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		return 0, nil, fmt.Errorf("prune: scan %s: %w", dir, err)
	}
	wanted := map[boardKey]bool{}
	for _, k := range retire {
		wanted[k] = true
	}

	moved := 0
	var held []string
	for _, path := range paths {
		if notBoardFiles[filepath.Base(path)] {
			continue
		}
		n, wouldEmpty, err := retireFromFile(dir, path, wanted)
		if err != nil {
			return moved, held, err
		}
		if wouldEmpty {
			held = append(held, strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".yml"), ".yaml"))
		}
		moved += n
	}
	sort.Strings(held)
	return moved, held, nil
}

// retireFromFile rewrites one board file, moving out the entries the caller wants.
func retireFromFile(dir, path string, wanted map[boardKey]bool) (moved int, wouldEmpty bool, err error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, false, fmt.Errorf("prune: read %s: %w", path, err)
	}
	lines := strings.Split(string(body), "\n")
	// A trailing newline splits to a final empty element; drop it and restore it on
	// write, so a file does not gain or lose one on every pass.
	trailing := len(lines) > 0 && lines[len(lines)-1] == ""
	if trailing {
		lines = lines[:len(lines)-1]
	}

	fileProvider := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".yml"), ".yaml")
	entries := splitEntries(lines)

	var going []entryBlock
	total := 0
	for _, e := range entries {
		provider := e.provider
		if provider == "" {
			provider = fileProvider
		}
		if e.board == "" {
			continue // not an entry we can key on; leave it exactly where it is
		}
		total++
		if wanted[boardKey{Provider: provider, Board: e.board}] {
			going = append(going, e)
		}
	}
	if len(going) == 0 {
		return 0, false, nil
	}
	// The one-way door: never leave a file with no entries at all.
	if len(going) == total {
		return 0, true, nil
	}

	keep := make([]string, 0, len(lines))
	cut := map[int]bool{}
	for _, e := range going {
		for i := e.start; i < e.end; i++ {
			cut[i] = true
		}
	}
	for i, line := range lines {
		if !cut[i] {
			keep = append(keep, line)
		}
	}
	out := strings.Join(keep, "\n")
	if trailing {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		return 0, false, fmt.Errorf("prune: write %s: %w", path, err)
	}

	if err := appendRetired(dir, filepath.Base(path), going, lines); err != nil {
		return 0, false, err
	}
	return len(going), false, nil
}

// entryBlock is one list item and the lines it owns: the "- " line plus the indented
// continuation lines under it. A blank line, a column-0 comment or the next item ends
// it, which is what keeps a group comment with the group rather than with an entry.
type entryBlock struct {
	start, end int
	provider   string
	board      string
}

func splitEntries(lines []string) []entryBlock {
	var out []entryBlock
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "- ") {
			continue
		}
		e := entryBlock{start: i, end: i + 1}
		for e.end < len(lines) {
			l := lines[e.end]
			if l == "" || !strings.HasPrefix(l, " ") {
				break
			}
			e.end++
		}
		for _, l := range lines[e.start:e.end] {
			if v, ok := scalarField(l, "provider"); ok {
				e.provider = v
			}
			if v, ok := scalarField(l, "board"); ok {
				e.board = v
			}
		}
		out = append(out, e)
		i = e.end - 1
	}
	return out
}

// scalarField reads `key: value` off a list-item line, tolerating the "- key: value"
// form of the first line and the quoting the files use for names with punctuation.
func scalarField(line, key string) (string, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
	rest, ok := strings.CutPrefix(s, key+":")
	if !ok {
		return "", false
	}
	v := strings.TrimSpace(rest)
	v = strings.Trim(v, `"'`)
	return v, v != ""
}

// appendRetired adds the moved entries to sources/retired/<same file name>, creating it
// with a header when it does not exist. The file name mirrors the source file rather
// than the provider: provider resolution is "the entry's own provider, else the file
// name", so mirroring keeps a custom.yml entry meaningful where it lands.
func appendRetired(dir, name string, going []entryBlock, lines []string) error {
	retiredDir := filepath.Join(dir, "retired")
	if err := os.MkdirAll(retiredDir, 0o755); err != nil {
		return fmt.Errorf("prune: create %s: %w", retiredDir, err)
	}
	path := filepath.Join(retiredDir, name)

	var b strings.Builder
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintf(&b, "# Retired %s boards: no longer crawled, kept so they can be picked up again.\n"+
			"# See README.md — a file here has the same shape as one in sources/ and is never read.\n\n", strings.TrimSuffix(name, filepath.Ext(name)))
	}
	sort.Slice(going, func(i, j int) bool { return going[i].board < going[j].board })
	for _, e := range going {
		for _, l := range lines[e.start:e.end] {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("prune: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(b.String()); err != nil {
		return fmt.Errorf("prune: append %s: %w", path, err)
	}
	return nil
}
