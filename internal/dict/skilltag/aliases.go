package skilltag

import "slices"

// aliasIndex inverts every alias tier at once: canonical → the spellings the parser
// accepts for it. Built at init, because the tables it reads are package-level and the
// answer never changes at runtime.
//
// The tiers do not resolve under the same conditions — a résumé acronym is not accepted
// on job text, a category-scoped one only inside its categories — and the index
// deliberately flattens that. Its consumer is the glossary, which answers "what else is
// this called?", not the parser, which answers "may I resolve this here?". Anything
// that needs the second question must go through Parse.
//
// Spellings are kept verbatim: the acronym tiers are case-sensitive, and "SAFe" written
// as "safe" would name the ordinary English word the table exists to avoid.
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

	// Sorted then compacted: the tiers overlap ("RAG" is in two acronym tables), and
	// sorting is needed anyway because map iteration would reorder the list per build.
	for canonical, aliases := range index {
		slices.Sort(aliases)
		index[canonical] = slices.Compact(aliases)
	}
	return index
}()

// Aliases is every spelling the PARSER accepts for a canonical skill, sorted and
// without repeats — "k8s" and "kubernetes", "c++" and "c/c++", "RAG" and "retrieval
// augmented generation". Empty for a slug the dictionary does not know.
//
// "The parser", not "resolves to it": Canonicalize additionally admits a bare canonical
// through canonicalSet, and a canonical is not automatically an alias of itself
// (canonicalize.go). This lists the alias tables and nothing else.
//
// The glossary renders this as the skill's other names, which is why the order is fixed
// rather than map order: the same page must read the same way on every build.
func Aliases(canonical string) []string {
	return slices.Clone(aliasIndex[canonical])
}
