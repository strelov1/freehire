package skilltag

import "slices"

// aliasIndex inverts every alias tier at once: canonical → the spellings the parser accepts for
// it. Built at init; the tables never change at runtime.
//
// The tiers resolve under different conditions (a résumé acronym is not accepted on job text, a
// category-scoped one only inside its categories) and the index flattens that: its consumer is
// the glossary ("what else is this called?"), not the parser. Anything that needs "may I resolve
// this here?" must go through Parse.
//
// Spellings are kept verbatim: the acronym tiers are case-sensitive, and "SAFe" written as "safe"
// would name the ordinary English word the table exists to avoid.
var aliasIndex = func() map[string][]string {
	index := map[string][]string{}
	add := func(canonical, alias string) {
		index[canonical] = append(index[canonical], alias)
	}

	for alias, canonical := range wordAliases {
		add(canonical, alias)
	}
	for _, p := range phraseAliases {
		add(p.canonical, p.alias)
	}
	for alias, canonical := range sharedAcronyms {
		add(canonical, alias)
	}
	for alias, canonical := range resumeAcronyms {
		add(canonical, alias)
	}
	for alias, scoped := range categoryScopedAcronyms {
		add(scoped.canonical, alias)
	}

	// Sorted then compacted: the tiers overlap ("RAG" is in two acronym tables), and map iteration
	// would reorder the list per build.
	for canonical, aliases := range index {
		slices.Sort(aliases)
		index[canonical] = slices.Compact(aliases)
	}
	return index
}()

// Aliases is every spelling the PARSER accepts for a canonical skill, sorted and without
// repeats ("k8s" and "kubernetes", "c++" and "c/c++"). Empty for an unknown slug.
//
// It lists the alias tables and nothing else: Canonicalize also admits a bare canonical through
// canonicalSet, and a canonical is not automatically an alias of itself (canonicalize.go). The
// order is fixed so the glossary page reads the same on every build.
func Aliases(canonical string) []string {
	return slices.Clone(aliasIndex[canonical])
}
