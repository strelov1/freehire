package main

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

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
		m := merge{Canonical: winner, FoldedKey: key}
		for _, c := range members {
			m.Jobs += c.JobCount
			if c.Slug == winner {
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

// electCanonical picks the group's surviving slug. Members arrive sorted by open jobs
// descending, so "the first that qualifies" is "the biggest that qualifies".
//
// Three rules, in order:
//
//  1. An already-frozen canon wins outright, even one carrying a corporate form. It has been
//     redirecting and indexing; moving it costs more than a tidier slug is worth.
//  2. Otherwise the biggest slug the rule can REPRODUCE — a fixed point of CompanySlug. Pure
//     job count is not enough: the first prod dry run elected `danaher-corporation` over
//     `danaher` (714 open jobs) because it happened to be larger, which would have made the
//     catalogue's canonical url the one carrying the form and 301'd the better-known slug into
//     it. It is also unstable, since every new posting derives the stripped slug and would need
//     an alias row for the canon to reach itself.
//  3. Failing that, the slug the rule YIELDS for the biggest member, even though no row holds
//     it yet. A group where every member carries a form is real — carnival-corporation and
//     carnival-corporation-plc were two of four in the >=100-job wave — and picking the least
//     bad existing row would leave the form on the canonical url, which is the outcome rule 2
//     exists to prevent. The derived slug is what every future posting keys to, and the
//     reconcile that follows the re-key creates its row.
//
// Returning a slug no member holds is safe: if a company already used it, its name would fold
// to the same key and it would BE a member — and would have won rule 2.
func electCanonical(members []company, frozen map[string]bool) string {
	for _, c := range members {
		if frozen[c.Slug] {
			return c.Slug
		}
	}
	var fixed []company
	for _, c := range members {
		if normalize.CompanySlug(c.Slug) == c.Slug {
			fixed = append(fixed, c)
		}
	}
	if len(fixed) > 0 {
		// Among spellings the rule can reproduce, prefer one the employer WRITES in several
		// words. Job count alone cannot see this: it elects `westerndigital` over
		// `western-digital` and `acehardware` over `ace-hardware` purely on volume, 73 times
		// in the >=100-job wave.
		//
		// The signal is the NAME, never the slug. "Domino's" slugs to `domino-s`, where an
		// apostrophe is indistinguishable from a word break — which is exactly why "prefer
		// the more hyphenated slug" elected `domino-s` over `dominos` and had to be dropped.
		//
		// It only speaks when it discriminates. Where no name is multi-word (Domino's) or
		// every name is (Alfa Bank beside Al Fa Bank), it says nothing and the count decides,
		// as it did before.
		for _, c := range fixed {
			if nameWords(c.Name) > 1 {
				return c.Slug
			}
		}
		return fixed[0].Slug
	}
	return normalize.CompanySlug(members[0].Name)
}

// nameWords counts the words of a name that are the name proper, trailing legal forms
// dropped: "Ace Hardware Corporation" is a two-word employer, not a three-word one.
//
// A HYPHEN separates words as surely as a space — Kimberly-Clark and T-Mobile write themselves
// that way — but an apostrophe does not: Brink's and Domino's are one word each. That is the
// distinction a slug cannot preserve, since Slug renders every one of them as a hyphen, and it
// is the whole reason this reads the name.
func nameWords(name string) int {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-'
	})
	for len(fields) > 1 && normalize.IsLegalForm(fields[len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	return len(fields)
}

// reasonFor classifies why a slug is retiring. The test is whether the current pure rule,
// applied to the alias's OWN name, already reaches the canonical slug: if it does, this
// duplicate exists only because the catalogue was keyed before that rule did, and nothing but
// the redirect needs the registry.
func reasonFor(c company, winner string) string {
	if normalize.CompanySlug(c.Name) == winner {
		return reasonLegalForm
	}
	return reasonSpelling
}
