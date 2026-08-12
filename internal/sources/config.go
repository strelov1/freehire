package sources

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is a parsed board file: the boards to crawl plus the file's default provider
// (its base name). Each entry's provider is normally this default, but an entry may name
// its own, so one file can list boards for several providers (e.g. a shared custom.yml).
type Config struct {
	Provider string
	Sources  []CompanyEntry
}

// LoadConfig reads a board file (e.g. sources/greenhouse.yml or sources/custom.yml). The
// file's base name is the default provider; an entry that names its own provider keeps it,
// so a per-provider file repeats nothing while a mixed file names the provider per entry.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("sources: read config %s: %w", path, err)
	}
	provider := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return ParseConfig(provider, data)
}

// ParseConfig parses a board-list, filling the file's default provider only where an entry
// left it blank — an entry's own provider wins — so every CompanyEntry ends up with a
// provider set for the rest of the pipeline.
func ParseConfig(provider string, data []byte) (Config, error) {
	entries, err := ParseRawEntries(provider, data)
	if err != nil {
		return Config{}, err
	}
	return Config{Provider: provider, Sources: dedupeBoards(entries)}, nil
}

// ParseRawEntries parses a board-list the same way ParseConfig does, but skips the
// case-variant board collapsing dedupeBoards performs — it exists for
// cmd/validate-sources, which needs to see the exact collisions ParseConfig fixes
// quietly and fail loudly on them instead (see DuplicateBoards). Production code should
// use ParseConfig or LoadConfig.
//
// Decoding is strict: an unrecognized key is a parse error rather than being silently
// dropped. A typo in Region or Hub — the two fields whose absence has no required-field
// error to catch it — would otherwise disable the behavior it names with no symptom
// until someone investigates why a board crawled the wrong host.
func ParseRawEntries(provider string, data []byte) ([]CompanyEntry, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var entries []CompanyEntry
	if err := dec.Decode(&entries); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("sources: parse config: %w", err)
	}
	// A `---`-separated second document would bypass KnownFields on this decoder (it only
	// inspects the document it decodes into), so reject anything past the first outright
	// rather than silently ignore it.
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("sources: parse config: unexpected second YAML document")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("sources: parse config: %w", err)
	}
	for i := range entries {
		if entries[i].Provider == "" {
			entries[i].Provider = provider
		}
	}
	return entries, nil
}

// dedupeBoards collapses entries that address the same case-insensitive board on the same
// provider and region, keeping the first occurrence. ATS board ids are case-insensitive at
// the platform (e.g. SmartRecruiters serves the same tenant for "SopraSteria1" and
// "soprasteria1"), but the pipeline namespaces external_id with the literal board string
// (see NamespaceExternalID), so a case-variant duplicate crawls identical postings yet
// stores them as a SECOND row-set under a different namespace but the SAME company_slug.
// The post-run unseen sweep is scoped by company_slug (not board), so whenever a run
// refreshes one variant but not the other, it closes the un-refreshed variant's still-live
// rows — a false-close. Collapsing here at load time keeps one row-set per board.
//
// Only board-bearing entries dedupe: a boardless entry (empty board) has no tenant id and
// is its own company, so an empty board is never a dedupe key. Region is part of the key so
// a same-name board on two regional hosts (a real, distinct crawl target) is preserved.
func dedupeBoards(entries []CompanyEntry) []CompanyEntry {
	seen := make(map[string]struct{}, len(entries))
	kept := make([]CompanyEntry, 0, len(entries))
	for _, e := range entries {
		key, ok := boardDedupeKey(e)
		if !ok {
			kept = append(kept, e)
			continue
		}
		if _, dup := seen[key]; dup {
			log.Printf("sources: dropping duplicate board %q (provider %s, company %q) — case-variant of an earlier entry",
				e.Board, e.Provider, e.Company)
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, e)
	}
	return kept
}

// boardDedupeKey is the identity dedupeBoards and DuplicateBoards collapse on:
// case-insensitive board, provider, and region. ok is false for a boardless entry, which
// has no tenant id and so is never a duplicate of anything.
func boardDedupeKey(e CompanyEntry) (key string, ok bool) {
	if e.Board == "" {
		return "", false
	}
	return strings.ToLower(e.Provider) + "\x00" + strings.ToLower(e.Board) + "\x00" + strings.ToLower(e.Region), true
}

// DuplicateBoards reports every entry that collides with an earlier one under
// boardDedupeKey — the exact pairs dedupeBoards drops silently at load time. It exists
// for cmd/validate-sources, which fails loudly on what production quietly self-heals.
func DuplicateBoards(entries []CompanyEntry) []string {
	seen := make(map[string]CompanyEntry, len(entries))
	var dups []string
	for _, e := range entries {
		key, ok := boardDedupeKey(e)
		if !ok {
			continue
		}
		if prev, dup := seen[key]; dup {
			dups = append(dups, fmt.Sprintf("duplicate board %q (provider %s): company %q collides with company %q",
				e.Board, e.Provider, e.Company, prev.Company))
			continue
		}
		seen[key] = e
	}
	return dups
}

// Validate checks every entry against the registry by its provider, so the ingest
// command fails fast instead of silently skipping a misconfigured board. Entries are
// expected resolved — provider set — as ParseConfig produces them.
func (c Config) Validate(registry map[string]Source) error {
	for _, e := range c.Sources {
		provider := e.Provider
		src, ok := registry[provider]
		if !ok {
			return fmt.Errorf("sources: unknown provider %q", provider)
		}
		if e.Company == "" {
			return fmt.Errorf("sources: %s entry has empty company", provider)
		}
		// A boardless provider crawls one company's own API and has no board id, so its
		// entries may omit board; every other provider still requires one.
		if _, noBoard := src.(boardless); !noBoard && e.Board == "" {
			return fmt.Errorf("sources: %s entry for company %q has empty board", provider, e.Company)
		}
	}
	return nil
}
