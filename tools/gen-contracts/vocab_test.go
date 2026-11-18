package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/application/mailclassify"
	"github.com/strelov1/freehire/internal/application/userjob"
	"github.com/strelov1/freehire/internal/job/collections"
)

// The role catalogue is retired, so the generated contracts must stop carrying its
// label map — a label table for a facet nothing serves is exactly the kind of dead
// vocabulary a picker keeps offering long after the filter behind it went away.
func TestGenVocabNoLongerEmitsRoleLabels(t *testing.T) {
	got := genVocab()
	if strings.Contains(got, "export const ROLE_LABELS") {
		t.Error("genVocab() still emits ROLE_LABELS")
	}
	if strings.Contains(got, "founding_engineer") {
		t.Error("genVocab() still emits a named role slug")
	}
	// The specialization labels that answer the same question stay.
	if !strings.Contains(got, "export const CATEGORY_VALUES") {
		t.Errorf("genVocab() lost the category vocabulary:\n%s", got)
	}
}

func TestEmitVocab(t *testing.T) {
	got := emitVocab("Seniority", "SENIORITY_VALUES", []string{"junior", "senior"})
	want := "export const SENIORITY_VALUES = ['junior', 'senior'] as const;\n" +
		"export type Seniority = (typeof SENIORITY_VALUES)[number];\n"
	if got != want {
		t.Errorf("emitVocab mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestEmitVocabEmpty(t *testing.T) {
	got := emitVocab("X", "X_VALUES", nil)
	want := "export const X_VALUES = [] as const;\n" +
		"export type X = (typeof X_VALUES)[number];\n"
	if got != want {
		t.Errorf("emitVocab(empty) = %q, want %q", got, want)
	}
}

func TestEmitMap(t *testing.T) {
	// Keys must be emitted in sorted order — the output is committed, so it has to be
	// deterministic regardless of Go's random map iteration.
	got := emitMap("CityCountry", "CITY_COUNTRY_MAP", map[string]string{"Berlin": "de", "Amsterdam": "nl"})
	want := "export const CITY_COUNTRY_MAP = {\n" +
		"  'Amsterdam': 'nl',\n" +
		"  'Berlin': 'de',\n" +
		"} as const;\n" +
		"export type CityCountry = typeof CITY_COUNTRY_MAP;\n"
	if got != want {
		t.Errorf("emitMap mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestEmitMapEmpty(t *testing.T) {
	got := emitMap("X", "X_MAP", nil)
	want := "export const X_MAP = {} as const;\n" +
		"export type X = typeof X_MAP;\n"
	if got != want {
		t.Errorf("emitMap(empty) = %q, want %q", got, want)
	}
}

// The groups are emitted as an ordered array rather than a map: pipeline order IS the board's
// column order and the funnel's band order, and a map would hand the SPA the membership while
// making it re-state the sequence.
func TestEmitStageGroups(t *testing.T) {
	got := emitStageGroups([]userjob.Group{
		{ID: "applied", Label: "Applied", Stages: []string{"applied", "screening"}},
		{ID: "offer", Label: "Offer", Stages: []string{"offer"}},
	})
	want := "export const STAGE_GROUPS = [\n" +
		"  { id: 'applied', label: 'Applied', stages: ['applied', 'screening'] },\n" +
		"  { id: 'offer', label: 'Offer', stages: ['offer'] },\n" +
		"] as const;\n" +
		"export type StageGroup = (typeof STAGE_GROUPS)[number];\n"
	if got != want {
		t.Errorf("emitStageGroups mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// Every stage the SPA knows must be placed by the emitted groups, and every stage the groups
// name must be a real one. The Go-side binding test guards the table; this guards what actually
// reaches the browser, which is the artifact the board reads.
func TestGenVocabEmitsEveryStageInAGroup(t *testing.T) {
	got := genVocab()
	for _, stage := range userjob.Stages {
		if !strings.Contains(got, "'"+stage+"'") {
			t.Errorf("genVocab() never mentions stage %q", stage)
		}
		if userjob.GroupOf(stage) == "" {
			t.Errorf("stage %q reaches the SPA in no group", stage)
		}
	}
	if !strings.Contains(got, "export const STAGE_GROUPS = [") {
		t.Errorf("genVocab() missing STAGE_GROUPS:\n%s", got)
	}
	if !strings.Contains(got, "export const STAGE_LABELS = {") {
		t.Errorf("genVocab() missing STAGE_LABELS:\n%s", got)
	}
}

// What a signal implies must reach the reader from the same table the classifier uses. A label
// with nothing said about the stage is exactly the silence this change removes: seven emails on
// an application, a stage that never moved, and no way to learn why.
func TestGenVocabEmitsWhatEverySignalImplies(t *testing.T) {
	got := genVocab()
	if !strings.Contains(got, "export const SIGNAL_STAGE = {") {
		t.Fatalf("genVocab() missing SIGNAL_STAGE:\n%s", got)
	}
	// A rejection implies `rejected` and still advances nothing — the pair the UI reads to
	// say "does not move the stage" instead of leaving a bare chip.
	if !strings.Contains(got, "'rejection': { stage: 'rejected', advances: false }") {
		t.Errorf("SIGNAL_STAGE missing the rejection implication:\n%s", got)
	}
	if !strings.Contains(got, "'acknowledgement': { stage: 'applied', advances: true }") {
		t.Errorf("SIGNAL_STAGE missing the acknowledgement implication:\n%s", got)
	}
	// `other` means the classifier could not tell, so it implies nothing.
	if !strings.Contains(got, "'other': { stage: '', advances: false }") {
		t.Errorf("SIGNAL_STAGE should carry `other` with no implied stage:\n%s", got)
	}
	for _, s := range mailclassify.SignalValues {
		if !strings.Contains(got, "'"+s+"': { stage:") {
			t.Errorf("SIGNAL_STAGE is missing signal %q", s)
		}
	}
}

func TestEmitCollections_RendersTheRegistryWithItsKinds(t *testing.T) {
	got := emitCollections([]collections.Collection{
		{Slug: "yc", Title: "Y Combinator", Description: "Open roles at YC companies.", Kind: collections.KindEditorial},
		{Slug: "uk-skilled-worker-sponsor", Title: "Licensed UK sponsor", Description: "It's a licence.", Kind: collections.KindCredential},
	})

	for _, want := range []string{
		"export const COLLECTIONS = [",
		"{ slug: 'yc', title: 'Y Combinator', description: 'Open roles at YC companies.', kind: 'editorial' },",
		"{ slug: 'uk-skilled-worker-sponsor', title: 'Licensed UK sponsor', description: 'It\\'s a licence.', kind: 'credential' },",
		"] as const;",
		"export type Collection = (typeof COLLECTIONS)[number];",
		"export type CollectionKind = Collection['kind'];",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("emitCollections output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestEmitCollections_KeepsRegistryOrder(t *testing.T) {
	// Display order is the registry's, not alphabetical — the hub and the facet
	// render in it.
	got := emitCollections([]collections.Collection{
		{Slug: "zeta", Kind: collections.KindEditorial},
		{Slug: "alpha", Kind: collections.KindEditorial},
	})
	if strings.Index(got, "'zeta'") > strings.Index(got, "'alpha'") {
		t.Error("emitCollections reordered the registry")
	}
}
