package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// The whole reason descriptions get their own module: contracts.ts is loaded on every
// page, and the description text is several times the size of the labels beside it. A
// description that leaked into the shared module would be downloaded by every visitor to
// serve a hover most of them never perform.
func TestSharedContractsCarryNoDescriptionText(t *testing.T) {
	shared := genVocab()
	for slug, description := range skilltag.Descriptions() {
		// The ESCAPED form, which is how a leak would actually appear: a description
		// carrying an apostrophe is emitted as `Google\'s` and a raw comparison misses it.
		if strings.Contains(shared, quoteTS(description)) {
			t.Errorf("contracts.ts carries the description of %q; it belongs in %s", slug, skillDescriptionsPath)
		}
	}
	// The labels stay where they were — this is an addition, not a move.
	if !strings.Contains(shared, "SKILL_LABELS") {
		t.Error("contracts.ts no longer carries SKILL_LABELS")
	}
}

func TestGenSkillDescriptionsEmitsEveryDescribedSkill(t *testing.T) {
	got := genSkillDescriptions()

	if !strings.HasPrefix(got, header) {
		t.Error("the generated module is missing the do-not-edit header")
	}
	if !strings.Contains(got, "export const SKILL_DESCRIPTIONS = {") {
		t.Error("the generated module is missing the SKILL_DESCRIPTIONS catalog")
	}
	for slug, description := range skilltag.Descriptions() {
		if !strings.Contains(got, quoteTS(slug)) {
			t.Errorf("the generated module is missing %q", slug)
		}
		if !strings.Contains(got, quoteTS(description)) {
			t.Errorf("the generated module is missing the description of %q", slug)
		}
	}
}

// The alias map is server-only: the glossary page renders "also written as" into its
// HTML, so no client ever needs the table. It is the largest of the three catalogs and
// the one the fewest readers benefit from, which is why it is neither eager nor bundled
// with the descriptions the tooltip fetches.
func TestGenSkillAliasesCoversTheVocabulary(t *testing.T) {
	got := genSkillAliases()

	if !strings.HasPrefix(got, header) {
		t.Error("the generated module is missing the do-not-edit header")
	}
	if !strings.Contains(got, "export const SKILL_ALIASES = {") {
		t.Error("the generated module is missing the SKILL_ALIASES catalog")
	}
	for _, canonical := range skilltag.Canonicals() {
		if !strings.Contains(got, quoteTS(canonical)+": [") {
			t.Errorf("SKILL_ALIASES is missing %q", canonical)
		}
	}
	// The spellings themselves, not just the keys: "k8s" is the whole point.
	if !strings.Contains(got, quoteTS("k8s")) {
		t.Error("SKILL_ALIASES lost the alias spellings")
	}
}

// An apostrophe in a description ("a language's runtime") would end the TS literal early
// and break the whole module, so the emitter's escaping is what keeps a curated sentence
// from being a build break.
func TestGenSkillDescriptionsEscapesQuotes(t *testing.T) {
	got := emitMap("X", "X_MAP", map[string]string{"go": "Google's language."})
	if !strings.Contains(got, `'Google\'s language.'`) {
		t.Errorf("emitMap did not escape the apostrophe: %q", got)
	}
}
