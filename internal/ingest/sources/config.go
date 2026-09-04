package sources

import (
	"fmt"
	"strings"
)

// Config is a set of boards to crawl plus the provider they belong to. It is what
// cmd/ingest builds from the board catalog (internal/ingest/boardcatalog) and hands to
// pipeline.Runner.
type Config struct {
	Provider string
	Sources  []CompanyEntry
}

// boardIdentity maps a provider to the function that folds every spelling of a board into
// the one thing it addresses. Case folding is not enough where a platform accepts more than
// one FORM of board id: iCIMS boards are written both as a bare slug and as the full
// "careers-<slug>.icims.com" host, and icimsHost resolves both to that host, so the two
// spellings are one crawl target — 37 pairs were once stored and crawled as if they were two.
// Dayforce is the same shape for a different reason: its optional culture segment chooses
// which translations of a site's postings to read, and one posting keeps the same id in every
// culture, so two entries differing only in culture would store each shared posting twice.
// A provider absent here folds on case alone.
var boardIdentity = map[string]func(board string) string{
	"icims":    icimsHost,
	"dayforce": dayforceSiteID,
	"gusto":    gustoBoardIdentity,
	"ukgready": ukgreadyTenant,
}

// BoardDedupeKey is the identity two catalog rows are the same crawl target under:
// case-insensitive board (folded through boardIdentity), provider, and region. ok is
// false for a boardless entry, which has no tenant id and so is never a duplicate of
// anything.
//
// The catalog's unique index (provider, lower(board), region) enforces the case half of
// this on its own. It cannot enforce the FORM half — the fold is Go, not SQL — which is
// why boardcatalog checks this key before inserting. It matters because the pipeline
// namespaces external_id with the LITERAL board string (see externalid.Namespace): two
// spellings of one board crawl identical postings yet store them as a second row-set
// under a different namespace but the SAME company_slug. The post-run unseen sweep is
// scoped by company_slug, not by board, so whenever a run refreshes one spelling and not
// the other it closes the un-refreshed spelling's still-live rows — a false-close.
func BoardDedupeKey(e CompanyEntry) (key string, ok bool) {
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

// Validate checks every entry against the registry by its provider, so the ingest
// command fails fast instead of silently skipping a misconfigured board. Entries are
// expected resolved — provider set — as boardcatalog produces them.
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
