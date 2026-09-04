package main

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/platform/db"
)

// boardKey is the identity the catalog and the job rows agree on exactly.
//
// The company slug is NOT that identity, though it looks like it. Many adapters take
// the company name from the posting payload rather than from the board entry —
// icims, jazzhr, careerplug, careerspage, jibe, geekjob and others all prefer
// HiringOrganization.Name — so jobs.company_slug and normalize.Slug(board.company)
// diverge wherever the payload spells the company differently. Measured on prod: of
// jazzhr's 3940 companies only 2453 match the catalog, and of careerplug's 8014
// only 71. A guard keyed on the slug reads all the rest as "retired" while their
// boards are crawled hourly.
//
// The board does not have that problem. The write path namespaces every crawled
// posting's external_id as "<board>:<native id>", and the catalog is keyed on
// (provider, board, region), so the join is exact. Region is not part of this key: it
// separates two catalog rows, never two postings, since the external_id carries only
// the board.
type boardKey struct{ Provider, Board string }

// boards is the set of boards a crawl still visits, read from the boards table.
type boards struct {
	listed map[boardKey]bool
	// byProvider indexes the boards of one provider, so resolving a job's board from
	// its external_id does not scan the whole set.
	byProvider map[string]map[string]bool
	// regions records every region a board is listed under, because retiring one means
	// naming (provider, board, region) — the catalog's own identity — and an aggregator
	// board exists once per region it covers.
	regions map[boardKey][]string
	// liveByProvider counts a provider's live rows, so the retire path can refuse to
	// take its last one.
	liveByProvider map[string]int
}

// boardLister is the read cmd/prune needs from the catalog.
type boardLister interface {
	ListLiveBoards(ctx context.Context) ([]db.ListLiveBoardsRow, error)
}

// loadBoards reads every live (pending or active) board from the catalog.
//
// It fails closed. A query error, or a catalog holding no live board at all, is an
// error rather than a short listing — a missing entry reads as "this board is retired",
// and an empty listing would read as "every board is retired", which arms the
// irreversible company-scoped rules on the entire catalogue at once.
func loadBoards(ctx context.Context, q boardLister) (boards, error) {
	rows, err := q.ListLiveBoards(ctx)
	if err != nil {
		return boards{}, fmt.Errorf("prune: list live boards: %w", err)
	}
	b := boards{
		listed:         map[boardKey]bool{},
		byProvider:     map[string]map[string]bool{},
		regions:        map[boardKey][]string{},
		liveByProvider: map[string]int{},
	}
	for _, row := range rows {
		key := boardKey{Provider: row.Provider, Board: row.Board}
		b.listed[key] = true
		if b.byProvider[row.Provider] == nil {
			b.byProvider[row.Provider] = map[string]bool{}
		}
		b.byProvider[row.Provider][row.Board] = true
		b.regions[key] = append(b.regions[key], row.Region)
		b.liveByProvider[row.Provider]++
	}
	if len(b.listed) == 0 {
		return boards{}, fmt.Errorf("prune: no live boards in the catalog")
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
