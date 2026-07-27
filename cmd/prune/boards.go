package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/strelov1/freehire/internal/sources"
)

// notBoardFiles are files under sources/ that are not board lists. telegram.yml is the
// channel list cmd/tg-ingest crawls — a `channels:` mapping, not a list of companies —
// so parsing it as a board file fails outright. Named explicitly rather than skipped on
// parse error: tolerating parse errors would re-open the fail-open hole this function
// exists to close, since a malformed board file would then read as a directory of
// retired boards.
var notBoardFiles = map[string]bool{"telegram.yml": true}

// boardKey is the identity the source files and the catalogue agree on exactly.
//
// The company slug is NOT that identity, though it looks like it. Many adapters take
// the company name from the posting payload rather than from the board entry —
// icims, jazzhr, careerplug, careerspage, jibe, geekjob and others all prefer
// HiringOrganization.Name — so jobs.company_slug and normalize.Slug(yaml.company)
// diverge wherever the payload spells the company differently. Measured on prod: of
// jazzhr's 3940 companies only 2453 match the board file, and of careerplug's 8014
// only 71. A guard keyed on the slug reads all the rest as "retired" while their
// boards are crawled hourly.
//
// The board does not have that problem. The write path namespaces every crawled
// posting's external_id as "<board>:<native id>", and the board files are keyed on
// (provider, board), so the join is exact.
type boardKey struct{ Provider, Board string }

// boards is the set of boards a crawl still visits.
type boards struct {
	listed map[boardKey]bool
	// byProvider indexes the boards of one provider, so resolving a job's board from
	// its external_id does not scan the whole set.
	byProvider map[string]map[string]bool
}

// loadBoards reads every board file under dir.
//
// It fails closed. An unreadable directory, a file that will not parse, or a directory
// holding no board entries at all is an error rather than a short listing — a missing
// entry reads as "this board is retired", and an empty listing would read as "every
// board is retired", which arms the irreversible company-scoped rules on the entire
// catalogue at once.
func loadBoards(dir string) (boards, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		return boards{}, fmt.Errorf("prune: scan %s: %w", dir, err)
	}
	b := boards{listed: map[boardKey]bool{}, byProvider: map[string]map[string]bool{}}
	for _, path := range paths {
		if notBoardFiles[filepath.Base(path)] {
			continue
		}
		cfg, err := sources.LoadConfig(path)
		if err != nil {
			return boards{}, err
		}
		for _, e := range cfg.Sources {
			b.listed[boardKey{Provider: e.Provider, Board: e.Board}] = true
			if b.byProvider[e.Provider] == nil {
				b.byProvider[e.Provider] = map[string]bool{}
			}
			b.byProvider[e.Provider][e.Board] = true
		}
	}
	if len(b.listed) == 0 {
		if _, statErr := os.Stat(dir); statErr != nil {
			return boards{}, fmt.Errorf("prune: source directory %s: %w", dir, statErr)
		}
		return boards{}, fmt.Errorf("prune: no board entries under %s", dir)
	}
	return b, nil
}

// knownProvider reports whether a source is a crawled board platform at all.
//
// It is a separate gate from crawls(), and the dry run showed why it has to be. A
// source with no boards — Telegram extraction, moderator rows, anything written
// outside the ingest pipeline — trivially satisfies "the board is absent", which is
// what the company-scoped rules require. So the very property that protects those
// postings from the title rule was qualifying them for the other two: the first prod
// dry run planned to delete 2991 hand-curated Telegram vacancies, which no crawl could
// ever restore. The spec forbids removing them by ANY rule; this is the gate that says so.
func (b boards) knownProvider(source string) bool {
	return b.byProvider[source] != nil
}

// crawls reports whether a posting came from a board the crawl still visits, resolved
// from the "<board>:<native id>" external_id the write path writes.
//
// One boolean answers both questions the rules ask, in opposite directions. The title
// rule needs it TRUE: a listed board is by definition re-crawlable, which is the whole
// reason a title deletion is recoverable — remove an over-broad dictionary term and the
// postings come back. The company-scoped rules need it FALSE: they have no counterpart
// at crawl time, so what they remove returns within the hour unless the board is gone.
//
// It also settles the cases a provider allow-list got wrong. A link-source import or a
// moderator row is stored under a real ATS provider but has no listed board, so it is
// never re-crawled and never deletable — which is what the spec requires and a
// provider-level check could not express. An aggregator's postings all carry its listed
// region board, so the company rules refuse them without needing to know it is an
// aggregator.
//
// A board id may itself contain a colon, so every colon-prefix is tried rather than
// splitting on the first one.
func (b boards) crawls(provider, externalID string) bool {
	_, ok := b.boardOf(provider, externalID)
	return ok
}

// boardOf resolves the listed board a posting came from. A board id may itself contain
// a colon, so every colon-prefix is tried against the provider's board set rather than
// splitting on the first one.
func (b boards) boardOf(provider, externalID string) (string, bool) {
	byBoard := b.byProvider[provider]
	if byBoard == nil {
		return "", false
	}
	for i, r := range externalID {
		if r == ':' && byBoard[externalID[:i]] {
			return externalID[:i], true
		}
	}
	return "", false
}
