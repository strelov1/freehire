package main

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/dict/normalize"
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
	Slug   string
	Reason string
	// FoldedKey is CompanyKey of THIS alias's own name, and it is what ingest resolves a
	// future posting through — ResolveCompanySlugAliases keys on folded_key, never on
	// alias_slug, so a spelling nobody merged still lands on the canon its fold owns.
	//
	// It sits on the alias rather than on the group because a CURATED group's members do not
	// share a fold: that is the definition of a curated group. Writing the group's key on
	// every row would leave "Exadel Inc (Website)" folding to a key no row holds, so the next
	// crawl of that board would mint the duplicate again and the merge would read as done.
	// For a folded group this is exactly the group's key, so the two agree where they used to.
	FoldedKey string
	JobCount  int
}

// merge is one group's decision.
type merge struct {
	Canonical string
	// GroupKey is what the members were grouped BY, and only sometimes a folded key: for a
	// folded group it is normalize.CompanyKey of the shared name, for a curated one it is
	// curatedGroupKey's sentinel. Nothing is stored from it — the alias carries the key the
	// registry reads — so its whole remaining job is to sort the plan into a stable order.
	GroupKey string
	Aliases  []alias
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
// curated overrides the grouping for the pairs a human has named (see curated.go). Those
// companies leave their folded groups entirely and form one group per canonical slug, where
// the canon is the one the list names rather than the one an election would reach. A curated
// group is the only kind whose members do not share a fold, which is why an alias carries its
// own folded key.
//
// The result is deterministic — groups sorted by key, ties broken on the slug — because the
// plan a human reviews in a dry run has to be the plan --apply then performs.
func planMerges(companies []company, frozen map[string]bool, minJobs int, curated map[string]string) []merge {
	canons := curatedCanons(curated)
	// The canon each curated group elects, so the loop below does not re-derive it from names
	// that were grouped precisely because their names do not agree.
	curatedWinner := make(map[string]string, len(canons))

	// The folds a curated canon owns, found in a first pass because a canon is named by SLUG
	// while a fold is computed from the NAME.
	//
	// This is what stops the list from breaking merges the rule already made. Naming a canon
	// pulls it out of its folded group, and everything the rule had grouped WITH it — "Acme
	// Inc" beside "Acme" — would be left in a group of one and silently dropped. Curating one
	// duplicate of an employer would therefore un-merge its others. So the curated group
	// absorbs the canon's fold whole: the rule's members and the list's members end up in the
	// same group, which is the truth both were describing.
	curatedFold := make(map[string]string, len(canons))
	for _, c := range companies {
		if c.Slug == "" || !canons[c.Slug] {
			continue
		}
		key := normalize.CompanyKey(c.Name)
		if key == "" {
			continue
		}
		// Two canons folding the same way would make the result depend on slice order, so the
		// smaller slug wins and the outcome is stable. It is a mistake either way — the guard
		// in curated_test.go rejects one canon retiring into another — but a deterministic
		// plan is what makes a dry run worth reading.
		if prev, ok := curatedFold[key]; !ok || c.Slug < prev {
			curatedFold[key] = c.Slug
		}
	}

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
		// A curated member joins its canon's group; the canon joins its own; and so does
		// anything the RULE would have grouped with the canon. Three ways in, one canon out,
		// and the key and the winner are set together — they have to agree, and setting them
		// per branch is how they would come to disagree.
		var canon string
		switch named, isMember := curated[c.Slug]; {
		case isMember:
			// Comma-ok rather than a non-empty test: "the list names this slug" is the
			// question, and an entry with an empty canon is a malformed list — which
			// TestCuratedAliasesAreWellFormed rejects — not a slug the list stays silent
			// about. Reading it as silence would hide the malformed entry here instead.
			canon = named
		case canons[c.Slug]:
			canon = c.Slug
		default:
			canon = curatedFold[key] // "" unless a curated canon owns this fold
		}
		if canon != "" {
			key = curatedGroupKey(canon)
			curatedWinner[key] = canon
		}
		groups[key] = append(groups[key], c)
	}

	var out []merge
	for key, members := range groups {
		_, isCuratedGroup := curatedWinner[key]
		// A folded group of one says nothing: the fold IS the evidence, so a lone member has
		// nothing to be a duplicate of. A curated group of one does say something — the list
		// names the canon, and that canon need not hold a catalogue row yet (a slug no row
		// holds is a legitimate canon; see electCanonical). Judging it by member count would
		// silently drop exactly the entry a human took the trouble to write down.
		if len(members) < 2 && !isCuratedGroup {
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

		winner := curatedWinner[key]
		if !isCuratedGroup {
			winner = electCanonical(members, frozen)
		}
		m := merge{Canonical: winner, GroupKey: key}
		for _, c := range members {
			m.Jobs += c.JobCount
			if c.Slug == winner {
				continue
			}
			m.Aliases = append(m.Aliases, alias{
				Slug:      c.Slug,
				Reason:    reasonFor(c, winner),
				FoldedKey: normalize.CompanyKey(c.Name),
				JobCount:  c.JobCount,
			})
		}
		// A curated group whose only member IS its canon has nothing to retire — the entry has
		// already been applied, or the duplicate has closed out of the catalogue. Reporting it
		// as a group would put a line in the plan that moves nothing.
		if len(m.Aliases) == 0 {
			continue
		}
		if m.Jobs < minJobs {
			continue
		}
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b merge) int { return cmp.Compare(a.GroupKey, b.GroupKey) })
	return out
}

// electCanonical picks the group's surviving slug. Members arrive sorted by open jobs
// descending, so "the first that qualifies" is "the biggest that qualifies".
//
//  1. An already-frozen canon wins outright, exactly as stored — even carrying a form. It has
//     been redirecting and indexing, and moving it costs more than a tidier slug is worth.
//  2. Otherwise the canon is the slug the RULE yields for the elected member's name, and the
//     election prefers a member whose name is written in several words.
//
// Deriving rather than reusing a stored slug is what makes the canon a fixed point by
// construction: CompanySlug of a derived slug is itself, so there is no separate rule keeping
// forms off the canonical url. It also stops a row that carries a form from being ignored for
// carrying one — "Public Storage" exists in the catalogue only as `public-storage-inc`, and
// skipping it left the squashed `publicstorage` to win by default, 2,811 times across the
// catalogue.
//
// The word-shape preference only speaks when it discriminates. Where no name is multi-word
// (Dominos beside Domino's) or every name is (Alfa Bank beside Al Fa Bank), it says nothing and
// the job count decides.
func electCanonical(members []company, frozen map[string]bool) string {
	for _, c := range members {
		if frozen[c.Slug] {
			return c.Slug
		}
	}
	for _, c := range members {
		if nameWords(c.Name) > 1 {
			return normalize.CompanySlug(c.Name)
		}
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
