package main

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// entry is the minimal board-file row we emit: company + board only, so a generated entry
// matches the hand-written file shape (no empty provider key).
type entry struct {
	Company string `yaml:"company"`
	Board   string `yaml:"board"`
}

// newBoards returns the seed slugs not already present in existing, preserving seed order,
// de-duplicating within the seed, and matching case-sensitively (Ashby slugs are
// case-sensitive, and the other platforms' tokens are lowercase already).
func newBoards(seed []string, existing map[string]bool) []string {
	var out []string
	seen := make(map[string]bool, len(seed))
	for _, s := range seed {
		if existing[s] || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// appendEntries returns the board-file content with the new entries appended, sorted by
// board. Existing content is preserved verbatim and the result ends with a trailing
// newline. YAML marshalling quotes only the names that need it.
func appendEntries(existing string, entries []entry) (string, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Board < entries[j].Board })
	block, err := yaml.Marshal(entries)
	if err != nil {
		return "", err
	}
	existing = strings.TrimRight(existing, "\n")
	if existing != "" {
		existing += "\n"
	}
	return existing + string(block), nil
}
