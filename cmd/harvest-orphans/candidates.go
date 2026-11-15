package main

import (
	"sort"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// minCandidate is the shortest candidate board id worth probing: a one- or two-character id matches
// an unrelated tenant, and the name gate cannot save it on platforms that publish no name.
const minCandidate = 3

// orphan is one company the catalogue holds only through aggregators.
type orphan struct {
	Slug    string
	Company string
}

// seedEntry is one candidate board in the {board, company} shape cmd/harvest-boards reads. The
// company is the employer the corroboration gate compares against the platform's name.
type seedEntry struct {
	Board   string `json:"board"`
	Company string `json:"company"`
}

// candidates proposes board ids for one company from its catalogue slug and display name alone,
// never from a fetched website. Five renderings cover how ATS tenants are named: the catalogue slug,
// the name with its corporate form stripped and hyphenated, the same unseparated, and the last two
// again with a board-name tail off. Duplicates collapse.
//
// The tail-stripped forms exist because the company KEY cannot strip those tails and a board id
// routinely does (see normalize.BoardNameSlug). E.g. Slug turns the Italian S.p.A. into `s-p-a`, so
// the first three renderings propose ids no platform issues. An extra candidate costs one probe;
// harvest-boards' corroboration gate keeps an over-trim ("Hilton Luxor Resort & Spa" ->
// `hilton-luxor-resort`) from being filed under the wrong employer.
func candidates(slug, company string) []string {
	var out []string
	add := func(c string) {
		if len(c) < minCandidate {
			return
		}
		for _, seen := range out {
			if seen == c {
				return
			}
		}
		out = append(out, c)
	}
	add(slug)
	add(normalize.CompanySlug(company))
	add(normalize.CompanyKey(company))
	board := normalize.BoardNameSlug(company)
	add(board)
	add(normalize.FoldSlug(board))
	return out
}

// seedEntries turns the worklist into board-sorted seed entries, dropping any candidate two
// DIFFERENT employers both propose: it cannot be attributed, and emitting it twice would let the
// entry sorting last decide which employer the gate compares against.
//
// "Different" is judged on the folded name, not the raw label: one employer often appears under two
// spellings ("Stripe" and "Stripe, Inc.", since company_slug keeps legal forms), and their shared
// candidate is the best one the tool can produce. A name that folds to nothing identifies nobody, so
// a second claimant always contests it.
//
// Sorting makes the output deterministic regardless of row order.
func seedEntries(orphans []orphan) []seedEntry {
	type claim struct{ company, key string }
	claims := make(map[string]claim, len(orphans))
	contested := make(map[string]bool)
	for _, o := range orphans {
		key := normalize.CompanyKey(o.Company)
		for _, c := range candidates(o.Slug, o.Company) {
			if prev, taken := claims[c]; taken {
				if key == "" || prev.key != key {
					contested[c] = true
				}
				continue
			}
			claims[c] = claim{company: o.Company, key: key}
		}
	}

	out := make([]seedEntry, 0, len(claims))
	for board, c := range claims {
		if contested[board] {
			continue
		}
		out = append(out, seedEntry{Board: board, Company: c.company})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Board < out[j].Board })
	return out
}
