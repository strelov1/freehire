package ojcp

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// Priorities as the requirement extractor records them. A requirement carries the posting's
// OWN word for how badly it wants the skill, which is the only evidence available for the
// split OJCP asks for.
const (
	priorityRequired  = "required"
	priorityPreferred = "preferred"
)

// skillsFor splits a posting's skills into what it DEMANDS and what it would merely like,
// which is the distinction `skills_required` and `skills_preferred` are defined by.
//
// The obvious source is wrong. `jobview.Job.Skills` is the dictionary facet, which resolves
// every skill named ANYWHERE in the description — a "nice to have" block included — and
// nothing in it separates a demand from a mention. Publishing it as `skills_required` tells
// an agent that a candidate without Kubernetes does not qualify for a role that called
// Kubernetes a plus, and makes a filter on required skills return postings that merely
// mention them.
//
// The evidence is in the requirements the posting itself stated (extracted by the model, or
// derived from the posting's own markup where the model never reached it — jobview folds
// the two). Each carries a priority, and running the dictionary over one block's text says
// which skills that block demanded.
//
// A posting with NO stated requirements yields nothing for either field. Falling back to the
// whole facet there would restore exactly the claim this split exists to remove.
func skillsFor(j jobview.Job) (required, preferred []string) {
	byPriority := map[string][]string{}
	for _, requirement := range j.Enrichment.Requirements {
		byPriority[requirement.Priority] = append(byPriority[requirement.Priority], requirement.Text)
	}

	required = namedSkillsIn(byPriority[priorityRequired])
	preferred = namedSkillsIn(byPriority[priorityPreferred])

	// A posting may name one skill in both blocks. Left in both, an agent reading the
	// preferred entry treats a hard requirement as optional, so the stronger claim wins.
	preferred = slices.DeleteFunc(preferred, func(skill string) bool {
		return slices.Contains(required, skill)
	})
	if len(preferred) == 0 {
		preferred = nil
	}
	return required, preferred
}

// namedSkillsIn resolves the skills stated across some requirement texts, spelled the way a
// reader writes them.
//
// It uses skilltag.PreferredFromText rather than Parse, for two reasons. It returns the
// curated SPELLING beside the canonical — the facet's values are internal keys (`cpp`,
// `ci-cd`, `dotnet`), and an agent relaying one to a candidate, or matching it against a CV
// that says "C++", is working with our key rather than with the skill. And it resolves the
// whole block at once, which is what lets an ambiguous spelling through: "Go" is an
// ordinary English word and counts only where the text also names a technology outright, a
// judgement that cannot be made one requirement line at a time.
func namedSkillsIn(texts []string) []string {
	if len(texts) == 0 {
		return nil
	}

	var names []string
	for _, display := range skilltag.PreferredFromText(strings.Join(texts, "\n")) {
		names = append(names, display)
	}
	// PreferredFromText answers a map, so the order is Go's. Sorting keeps a posting's
	// projection identical between reads, which an agent caching or diffing depends on.
	slices.Sort(names)
	return names
}
