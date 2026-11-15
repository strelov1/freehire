package main

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// The two ways a slug duplicates another, recorded on every alias so a reversal can target one
// class. A legal_form merge is a pure rule (normalize.CompanySlug reaches the canon from the
// alias's own name); a spelling merge is a judgement, since both spellings are valid output of
// the rule and only the job count separates them.
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
	// FoldedKey is CompanyKey of THIS alias's own name; ingest resolves future postings through
	// it (ResolveCompanySlugAliases keys on folded_key, never alias_slug). It lives on the alias,
	// not the group, because a curated group's members do not share a fold.
	FoldedKey string
	JobCount  int
}

// merge is one group's decision.
type merge struct {
	Canonical string
	// GroupKey is what the members were grouped by: CompanyKey of the shared name, or
	// curatedGroupKey's sentinel. Never stored; it only gives the plan a stable order.
	GroupKey string
	Aliases  []alias
	// Jobs is the group's combined open jobs, which --min-jobs bounds a wave by.
	Jobs int
}

// planMerges groups companies by their folded company key and decides, per group, which slug
// survives and which retire into it.
//
// The key is normalize.CompanyKey of the NAME, not of the stored slug: it applies the legal-form
// rule and folds word breaks, so one pass covers both duplicate classes.
//
// frozen is the set of slugs elected canonical by an earlier wave; one wins its group outright.
// minJobs drops groups whose combined open jobs fall short.
//
// curated (see curated.go) overrides the grouping for human-named pairs: those companies leave
// their folded groups and form one group per canonical slug, with the canon the list names.
//
// The result is deterministic (sorted by key, ties on slug) so a dry run is the plan --apply performs.
func planMerges(companies []company, frozen map[string]bool, minJobs int, curated map[string]string) []merge {
	canons := curatedCanons(curated)
	// The canon each curated group elects; its names disagree, so it cannot be re-derived.
	curatedWinner := make(map[string]string, len(canons))

	// The folds a curated canon owns (a canon is named by slug, a fold computed from the name).
	// The curated group absorbs the canon's fold whole; otherwise pulling the canon out would
	// leave its rule-grouped siblings ("Acme Inc" beside "Acme") alone in a group of one, dropped.
	curatedFold := make(map[string]string, len(canons))
	for _, c := range companies {
		if c.Slug == "" || !canons[c.Slug] {
			continue
		}
		key := normalize.CompanyKey(c.Name)
		if key == "" {
			continue
		}
		// Two canons with one fold are a mistake (curated_test.go guards it); the smaller slug
		// wins so the plan does not depend on slice order.
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
		// A curated member, the canon itself, and anything the rule grouped with the canon all
		// join the canon's group. Key and winner are set together so they cannot disagree.
		var canon string
		switch named, isMember := curated[c.Slug]; {
		case isMember:
			// Comma-ok, not a non-empty test: an empty canon is a malformed entry
			// (TestCuratedAliasesAreWellFormed rejects it), not silence.
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
		// A lone folded member has nothing to duplicate. A curated group of one still counts: the
		// canon need not hold a catalogue row yet (see electCanonical).
		if len(members) < 2 && !isCuratedGroup && !keyedBeforeTheRule(members[0], frozen) {
			continue
		}
		// Job count descending, then slug, so the election and alias order are stable.
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
		// A curated group whose only member is its canon has nothing to retire.
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

// keyedBeforeTheRule reports whether a company with no twin still has to move: its stored slug
// was minted by an older rule from the same name ([normalize.SlugPredatesRule]). Planning it
// records the redirect in the same reviewed wave as a legal-form merge. A frozen canon never moves.
func keyedBeforeTheRule(c company, frozen map[string]bool) bool {
	return !frozen[c.Slug] && normalize.SlugPredatesRule(c.Slug, c.Name)
}

// electCanonical picks the group's surviving slug. Members arrive sorted by open jobs
// descending, so the first that qualifies is the biggest.
//
//  1. An already-frozen canon wins outright, exactly as stored, even carrying a form.
//  2. Otherwise the canon is the slug the rule yields for the elected member's name, preferring
//     a multi-word name. Deriving makes the canon a fixed point (CompanySlug of it is itself)
//     and does not skip a row just because its stored slug carries a form.
//
// The word-shape preference only applies when it discriminates (Alfa Bank beside Al Fa Bank
// does not); otherwise the job count decides.
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

// nameWords counts the words of a name, trailing legal forms dropped ("Ace Hardware
// Corporation" is two). A hyphen separates words but an apostrophe does not (Brink's is one);
// a slug cannot preserve that, so this reads the name.
func nameWords(name string) int {
	fields := strings.FieldsFunc(normalize.CloseSpacedForms(name), func(r rune) bool {
		return unicode.IsSpace(r) || r == '-'
	})
	next := ""
	for len(fields) > 1 && normalize.IsLegalFormBefore(fields[len(fields)-1], next) {
		next = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	return len(fields)
}

// reasonFor classifies why a slug is retiring: legal_form if the current rule, applied to the
// alias's own name, already reaches the canonical slug, else spelling.
func reasonFor(c company, winner string) string {
	if normalize.CompanySlug(c.Name) == winner {
		return reasonLegalForm
	}
	return reasonSpelling
}

// onlyReason keeps the groups whose every alias retires for reason; "" keeps them all.
//
// It lets a wave take the pure rule without the judgement: legal_form retirements are safe to
// apply wholesale, spelling merges are reviewed. A mixed group is left out whole, since its
// canon was elected over both.
func onlyReason(plan []merge, reason string) []merge {
	if reason == "" {
		return plan
	}
	var out []merge
	for _, m := range plan {
		if !slices.ContainsFunc(m.Aliases, func(a alias) bool { return a.Reason != reason }) {
			out = append(out, m)
		}
	}
	return out
}

// onlySlugs keeps the groups holding at least one listed slug, as canon or as alias.
//
// It takes a few already-reviewed groups without the rest of the backlog. The whole group
// goes, never part of it, since its canon was elected over every member.
func onlySlugs(plan []merge, slugs map[string]bool) []merge {
	var out []merge
	for _, m := range plan {
		if slugs[m.Canonical] || slices.ContainsFunc(m.Aliases, func(a alias) bool { return slugs[a.Slug] }) {
			out = append(out, m)
		}
	}
	return out
}
