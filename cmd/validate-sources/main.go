// Command validate-sources checks every board file under sources/ (or the directory
// named by its one argument) for the two ways a hand-edited YAML file goes bad without
// anyone noticing at ingest time: a malformed or unrecognized-field entry, and a
// case-variant duplicate board that cmd/ingest would otherwise silently collapse
// (internal/sources.dedupeBoards) with only a log line to show for it.
//
// It touches no network and no database, so it is meant to run on every PR. On failure it
// prints one line per bad file — every duplicate board plus, if the file's structural
// Validate also fails, that error too — and exits non-zero rather than stopping at the
// first file.
//
// Usage:
//
//	go run ./cmd/validate-sources           # checks sources/
//	go run ./cmd/validate-sources sources    # same, explicit
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/telegram"
)

// notBoardFiles are files under sources/ that do not hold a CompanyEntry list and carry
// their own schema and validation — mirrors cmd/prune's boards.go.
var notBoardFiles = map[string]bool{"telegram.yml": true}

func main() {
	dir := "sources"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	problems, checked, err := run(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		fmt.Fprintf(os.Stderr, "validate-sources: %d problem(s) across %d file(s)\n", len(problems), checked)
		os.Exit(1)
	}
	fmt.Printf("validate-sources: OK — %d file(s) checked\n", checked)
}

func run(dir string) (problems []string, checked int, err error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		return nil, 0, fmt.Errorf("validate-sources: scan %s: %w", dir, err)
	}
	if len(paths) == 0 {
		return nil, 0, fmt.Errorf("validate-sources: no *.yml files under %s", dir)
	}

	registry := sources.All(nil)
	for _, path := range paths {
		checked++
		if notBoardFiles[filepath.Base(path)] {
			problems = append(problems, validateTelegramFile(path)...)
			continue
		}
		problems = append(problems, validateBoardFile(path, registry)...)
	}
	return problems, checked, nil
}

func validateBoardFile(path string, registry map[string]sources.Source) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	provider := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries, err := sources.ParseRawEntries(provider, data)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}

	var problems []string
	for _, dup := range sources.DuplicateBoards(entries) {
		problems = append(problems, fmt.Sprintf("%s: %s", path, dup))
	}
	if err := (sources.Config{Provider: provider, Sources: entries}).Validate(registry); err != nil {
		problems = append(problems, fmt.Sprintf("%s: %v", path, err))
	}
	return problems
}

func validateTelegramFile(path string) []string {
	cfg, err := telegram.LoadConfig(path)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	if err := cfg.Validate(); err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	return nil
}
