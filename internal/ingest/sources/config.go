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
		// A board id never legitimately carries surrounding whitespace, and one that does is
		// a board that 404s: the adapters paste it into a URL and the pipeline namespaces
		// external_id with the literal string. It also hides a duplicate from the collapse
		// below — a harvested UKG board arrived with a trailing space and so did not collide
		// with the same board already in the file.
		entries[i].Board = strings.TrimSpace(entries[i].Board)
	}
	return entries, nil
}

// dedupeBoards collapses entries that address the same board on the same provider and
// region, keeping the first occurrence. Two things make one board look like two: case (ATS
// board ids are case-insensitive at the platform — SmartRecruiters serves the same tenant for
// "SopraSteria1" and "soprasteria1") and, on some platforms, the FORM of the id (see
// boardIdentity). Either way the pipeline namespaces external_id with the literal board string
// (see externalid.Namespace), so a case-variant duplicate crawls identical postings yet
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
			log.Printf("sources: dropping duplicate board %q (provider %s, company %q) — addresses the same site as an earlier entry",
				e.Board, e.Provider, e.Company)
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, e)
	}
	return kept
}

// boardIdentity maps a provider to the function that folds every spelling of a board into
// the one thing it addresses. Case folding is not enough where a platform accepts more than
// one FORM of board id: iCIMS boards are stored both as a bare slug and as the full
// "careers-<slug>.icims.com" host, and icimsHost resolves both to that host, so the two
// spellings are one crawl target — 37 pairs were committed and crawled as if they were two.
// Dayforce is the same shape for a different reason: its optional culture segment chooses
// which translations of a site's postings to read, and one posting keeps the same id in every
// culture, so two entries differing only in culture would store each shared posting twice.
// A provider absent here folds on case alone.
var boardIdentity = map[string]func(board string) string{
	"icims":    icimsHost,
	"dayforce": dayforceSiteID,
}

// boardDedupeKey is the identity dedupeBoards and DuplicateBoards collapse on:
// case-insensitive board (folded through boardIdentity), provider, and region. ok is false
// for a boardless entry, which has no tenant id and so is never a duplicate of anything.
func boardDedupeKey(e CompanyEntry) (key string, ok bool) {
	if e.Board == "" {
		return "", false
	}
	provider := strings.ToLower(e.Provider)
	board := e.Board
	if fold := boardIdentity[provider]; fold != nil {
		board = fold(board)
	}
	return provider + "\x00" + strings.ToLower(board) + "\x00" + strings.ToLower(e.Region), true
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
