package ojcp

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// Priorities as the requirement extractor records them: the posting's own word for how
// badly it wants the skill.
const (
	priorityRequired  = "required"
	priorityPreferred = "preferred"
)

// skillsFor splits a posting's skills into what it demands and what it would merely like.
//
// Do not use `jobview.Job.Skills`: that facet resolves every skill named anywhere in the
// description, "nice to have" blocks included, so it cannot tell a demand from a mention.
// Instead run the dictionary over each stated requirement block (model-extracted or derived
// from markup; jobview folds the two) by its priority.
//
// A posting with no stated requirements yields nothing; falling back to the facet would
// restore the claim this split removes.
func skillsFor(j jobview.Job) (required, preferred []string) {
	byPriority := map[string][]string{}
	for _, requirement := range j.Enrichment.Requirements {
		byPriority[requirement.Priority] = append(byPriority[requirement.Priority], requirement.Text)
	}

	required = namedSkillsIn(byPriority[priorityRequired])
	preferred = namedSkillsIn(byPriority[priorityPreferred])

	// A skill named in both blocks stays only in required.
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
// It uses skilltag.PreferredFromText rather than Parse for two reasons: it returns the
// curated spelling ("C++") rather than the internal key (`cpp`), and it resolves the whole
// block at once, which ambiguous words need ("Go" counts only where the text also names a
// technology outright).
func namedSkillsIn(texts []string) []string {
	if len(texts) == 0 {
		return nil
	}

	var names []string
	for _, display := range skilltag.PreferredFromText(strings.Join(texts, "\n")) {
		names = append(names, display)
	}
	// The map order is random; sort so the projection is stable between reads.
	slices.Sort(names)
	return names
}
