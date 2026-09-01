package main

import (
	"maps"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// The skill glossary ships as its OWN module rather than as one more map in
// contracts.ts, and the split is the point.
//
// contracts.ts is imported on every page. The labels beside these descriptions are a
// word each; a description is a sentence, so the whole catalogue is several times the
// size of everything the vocabulary section holds today — paid by every visitor to serve
// a hover most of them never perform. In its own file the SPA can `await import` it when
// a definition is actually opened, and the bundler gives it a chunk of its own.
//
// It is not served by the API for the same reason from the other side: a description
// belongs to the vocabulary, not to a posting, so putting it in the job payload would
// ship the same sentence with every posting that names the skill.
const skillDescriptionsPath = "web/src/lib/generated/skillDescriptions.ts"

// genSkillDescriptions renders the slug→description catalogue as a standalone module.
// Generated from the same dictionary that decides the skills, so a description cannot
// drift from the skill it describes, and an undescribed canonical is simply absent —
// the SPA reads a missing key as "no definition", which is what it is.
func genSkillDescriptions() string {
	return header + emitMap("SkillDescriptions", "SKILL_DESCRIPTIONS", skilltag.Descriptions())
}

// The alias table is SERVER-ONLY: the glossary page server-renders "also written as"
// into its HTML, so no browser needs the map. That makes it the cheapest of the three
// catalogs to ship despite being the largest — it never crosses the wire as data.
//
// It is deliberately not folded into skillDescriptions.ts. That module is fetched by
// every tooltip on every job page, and the aliases would ride along to serve a page
// almost none of those readers will open.
const skillAliasesPath = "web/src/lib/generated/skillAliases.ts"

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

// emitDescribedSkills renders WHICH skills have an entry — the slugs, no prose — into
// the shared, eagerly loaded module.
//
// This one has to be eager while the sentences must not be. A skill chip carries a
// "what is this?" affordance, and that affordance may not appear on a skill with no
// definition behind it: the decision is made as the chip renders, before any lazily
// imported chunk could have arrived. Slugs are a fraction of the weight of the
// sentences they key, which is what makes paying for them upfront reasonable.
//
// It is also temporary. Once every canonical is described, "described" and "canonical"
// name the same set and SKILL_LABELS already lists it eagerly — so the last wave
// deletes this alongside describedFloor.
//
// No `as const`: nothing needs the union of 863 string literals, only membership.
func emitDescribedSkills(descriptions map[string]string) string {
	slugs := slices.Sorted(maps.Keys(descriptions))
	quoted := make([]string, len(slugs))
	for i, s := range slugs {
		quoted[i] = quoteTS(s)
	}
	return "export const SKILL_DESCRIBED = [" + strings.Join(quoted, ", ") + "];\n"
}
