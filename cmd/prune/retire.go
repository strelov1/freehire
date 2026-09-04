package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/strelov1/freehire/internal/platform/db"
)

// boardRetirer is the write cmd/prune needs from the catalog.
type boardRetirer interface {
	RetireBoard(ctx context.Context, arg db.RetireBoardParams) (int64, error)
}

// retireBoards flips the named boards to status='retired' in the catalog — the status
// change IS the retirement, since cmd/ingest crawls only 'pending' and 'active' rows.
// It returns how many catalog rows it retired.
//
// A board listed under several regions retires under all of them: the postings the
// report judged carry only the board, so a verdict about the board cannot single out
// one of its regional rows.
//
// It refuses to retire a provider's last live boards. That is the one irreversible step
// in a reversible operation: a provider with nothing live in the catalog is never
// crawled again, and the company-scoped rules refuse a job they cannot re-crawl, so its
// postings become permanently un-prunable. Prune those jobs first, then retire the last
// board deliberately (cmd/add-board --retire). It returns those providers' names
// alongside the count, because a silent refusal reads as "there was nothing to do" —
// and the answer is not "never retire it", it is "later".
func retireBoards(ctx context.Context, q boardRetirer, brd boards, retire []boardKey) (int, []string, error) {
	// Count what each provider would lose before retiring anything, so the refusal is
	// decided on the whole list rather than emerging halfway through it.
	losing := map[string]int{}
	for _, k := range retire {
		losing[k.Provider] += len(brd.regionsOf(k))
	}
	held := map[string]bool{}
	var heldNames []string
	for provider, n := range losing {
		if n >= brd.liveRows(provider) {
			held[provider] = true
			heldNames = append(heldNames, provider)
		}
	}
	sort.Strings(heldNames)

	retired := 0
	for _, k := range retire {
		if held[k.Provider] {
			continue
		}
		for _, region := range brd.regionsOf(k) {
			n, err := q.RetireBoard(ctx, db.RetireBoardParams{
				Provider: k.Provider, Lower: k.Board, Region: region,
			})
			if err != nil {
				return retired, heldNames, fmt.Errorf("prune: retire %s/%s: %w", k.Provider, k.Board, err)
			}
			retired += int(n)
		}
	}
	return retired, heldNames, nil
}
