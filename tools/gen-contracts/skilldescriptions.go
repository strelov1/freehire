package main

import "github.com/strelov1/freehire/internal/dict/skilltag"

// The skill glossary is its own module rather than one more map in contracts.ts: contracts.ts
// loads on every page and a description is a sentence, so the catalogue would cost every
// visitor for a hover most never perform. Separate, the SPA can `await import` it when a
// definition is opened. The API does not serve it either: a description belongs to the
// vocabulary, not to each posting.
const skillDescriptionsPath = "../web/src/lib/generated/skillDescriptions.ts"

// genSkillDescriptions renders the slug→description catalogue as a standalone module.
// Generated from the same dictionary that decides the skills, so a description cannot
// drift from the skill it describes, and an undescribed canonical is simply absent —
// the SPA reads a missing key as "no definition", which is what it is.
func genSkillDescriptions() string {
	return header + emitMap("SkillDescriptions", "SKILL_DESCRIPTIONS", skilltag.Descriptions())
}

// The alias table is server-only: the glossary page server-renders "also written as" into its
// HTML, so no browser needs the map. It is not folded into skillDescriptions.ts, which every
// job-page tooltip fetches.
const skillAliasesPath = "../web/src/lib/generated/skillAliases.ts"

// genSkillAliases renders canonical → the spellings the parser accepts, for every
// canonical rather than only the described ones: the glossary page needs it for the
// skill it is rendering, and gating it on coverage would make the module's contents
// shift under the waves for no benefit.
func genSkillAliases() string {
	canonicals := skilltag.Canonicals()
	aliases := make(map[string][]string, len(canonicals))
	for _, c := range canonicals {
		aliases[c] = skilltag.Aliases(c)
	}
	return header + emitMapOfSlices("SkillAliases", "SKILL_ALIASES", aliases)
}
