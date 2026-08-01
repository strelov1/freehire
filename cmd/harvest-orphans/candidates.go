package main

import (
	"sort"

	"github.com/strelov1/freehire/internal/normalize"
)

// minCandidate is the shortest candidate board id worth probing. A one- or two-character id
// matches an unrelated tenant far more often than the intended employer, and the harvest's
// name gate cannot save it on the platforms that publish no name.
const minCandidate = 3

// orphan is one company the catalogue holds only through aggregators.
type orphan struct {
	Slug    string
	Company string
}

// seedEntry is one candidate board written to the seed, in the {board, company} shape
// cmd/harvest-boards reads. The company is the employer the board is expected to belong to —
// the harvest's corroboration gate compares it against the name the platform publishes.
type seedEntry struct {
	Board   string `json:"board"`
	Company string `json:"company"`
}

// candidates proposes board ids for one company from its catalogue slug and display name
// alone — never from a fetched website, so discovery does not depend on resolving a domain.
// Three renderings cover how ATS tenants are actually named: the slug the catalogue already
// derived, the name with its corporate form stripped and words hyphenated, and the same
// unseparated. Duplicates collapse, so a single-word company proposes exactly one candidate.
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
	return out
}

// seedEntries turns the worklist into board-sorted seed entries, dropping any candidate two
// DIFFERENT employers both propose. Such a candidate cannot be attributed: emitting it twice
// would probe the same board twice and let the entry that happens to sort last decide which
// employer the gate compares against, quietly filing the board under the wrong company.
//
// "Different" is judged on the folded name, not the raw label. One employer routinely
// reaches the catalogue under two spellings — `company_slug` is normalize.Slug, which keeps
// legal forms — so "Stripe" and "Stripe, Inc." are two worklist rows whose shared candidate
// is one employer's board, and the best candidate the tool can produce: two independent
// sources arriving at the same id. Comparing raw labels would discard exactly that one and
// keep the junk renderings. A name that folds to nothing identifies nobody, so a second
// claimant always contests it.
//
// Sorting makes a run's output deterministic regardless of row order.
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
