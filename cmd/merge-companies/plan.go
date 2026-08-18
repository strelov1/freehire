package main

import (
	"cmp"
	"slices"

	"github.com/strelov1/freehire/internal/normalize"
)

// The two ways a company slug can be a duplicate of another, recorded on every alias so a
// later reversal can target one class without disturbing the other.
//
// They rest on different evidence, which is the whole reason to tell them apart. A
// legal_form merge is a pure rule — normalize.CompanySlug reaches the canonical slug from
// the alias's own name, and the write path now applies it unaided, so these aliases exist
// only to redirect URLs the catalogue already minted. A spelling merge is a judgement: both
// spellings are honest output of that same rule, and only the job count separates them.
const (
	reasonLegalForm = "legal_form"
	reasonSpelling  = "spelling"
)

// company is one catalogue company the merge considers: its stored slug, the display name
// the slug was derived from, and the open-job count that elects a winner.
type company struct {
	Slug     string
	Name     string
	JobCount int
}

// alias is one slug retiring into a canonical one.
type alias struct {
	Slug     string
	Reason   string
	JobCount int
}

// merge is one folded group's decision.
type merge struct {
	Canonical string
	FoldedKey string
	Aliases   []alias
	// Jobs is the group's combined open jobs — what --min-jobs bounds a wave by, and the
	// figure that says how much of the catalogue a wave moves.
	Jobs int
}

// planMerges groups companies by their folded company key and decides, per group, which slug
// survives and which retire into it.
//
// The key is normalize.CompanyKey of the NAME, not a fold of the stored slug, and that is
// what makes one pass cover both duplicate classes: it applies the current legal-form rule to
// the name the catalogue holds, so "RingCentral, Inc." and "RingCentral" meet there, and then
// folds the word breaks, so "DollarTree" and "Dollar Tree" meet too.
//
// frozen is the set of slugs already elected canonical by an earlier wave; one of those wins
// its group outright, whatever the counts now say. minJobs drops groups whose combined open
// jobs fall short, so a wave can be reviewed at a size a human can actually read.
//
// The result is deterministic — groups sorted by key, ties broken on the slug — because the
// plan a human reviews in a dry run has to be the plan --apply then performs.
func planMerges(companies []company, frozen map[string]bool, minJobs int) []merge {
	groups := make(map[string][]company)
	for _, c := range companies {
		if c.Slug == "" {
			continue
		}
		key := normalize.CompanyKey(c.Name)
		if key == "" {
			// A name that folds to nothing says nothing about who the employer is; grouping
			// on it would merge every untransliterable name into one company.
			continue
		}
		groups[key] = append(groups[key], c)
	}

	var out []merge
	for key, members := range groups {
		if len(members) < 2 {
			continue
		}
		// Sort by job count descending, then by slug, so the election and the alias order
		// are both stable.
		slices.SortFunc(members, func(a, b company) int {
			if d := cmp.Compare(b.JobCount, a.JobCount); d != 0 {
				return d
			}
			return cmp.Compare(a.Slug, b.Slug)
		})

		winner := electCanonical(members, frozen)
		m := merge{Canonical: winner.Slug, FoldedKey: key}
		for _, c := range members {
			m.Jobs += c.JobCount
			if c.Slug == winner.Slug {
				continue
			}
			m.Aliases = append(m.Aliases, alias{
				Slug:     c.Slug,
				Reason:   reasonFor(c, winner),
				JobCount: c.JobCount,
			})
		}
		// len(m.Aliases) is always >= 1 here: the group holds at least two companies and
		// exactly one of them wins.
		if m.Jobs < minJobs {
			continue
		}
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b merge) int { return cmp.Compare(a.FoldedKey, b.FoldedKey) })
	return out
}

// electCanonical picks the group's surviving slug: an already-frozen canon if the group holds
// one, otherwise the member with the most open jobs (members arrive pre-sorted).
func electCanonical(members []company, frozen map[string]bool) company {
	for _, c := range members {
		if frozen[c.Slug] {
			return c
		}
	}
	return members[0]
}

// reasonFor classifies why a slug is retiring. The test is whether the current pure rule,
// applied to the alias's OWN name, already reaches the canonical slug: if it does, this
// duplicate exists only because the catalogue was keyed before that rule did, and nothing but
// the redirect needs the registry.
func reasonFor(c, winner company) string {
	if normalize.CompanySlug(c.Name) == winner.Slug {
		return reasonLegalForm
	}
	return reasonSpelling
}
