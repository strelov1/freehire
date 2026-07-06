// Package roletag derives a job's natural roles deterministically from its
// already-resolved seniority and category and its title. It is a curated
// dictionary, not a model — the same doctrine as internal/classify and
// internal/skilltag: it emits canonical role slugs for what it can resolve and
// nothing for what it cannot (it never guesses).
//
// A role is either a composite {seniority}_{category} (emitted only when both
// axes are resolved) or a named role matched from the title for roles that do
// not decompose into the seniority×category grid (e.g. founding_engineer,
// fractional_cto). The package also owns the catalog (slug → human label), the
// source of truth for the picker labels emitted into the web contracts.
package roletag

import (
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// seniorityLabel maps each enrich.SeniorityValues canonical to its display word.
var seniorityLabel = map[string]string{
	"intern":    "Intern",
	"junior":    "Junior",
	"middle":    "Middle",
	"senior":    "Senior",
	"lead":      "Lead",
	"staff":     "Staff",
	"principal": "Principal",
	"c_level":   "C-Level",
}

// categoryNoun maps each enrich.CategoryValues canonical (except "other", which
// yields no useful natural role) to its role noun. Composite labels are built as
// "{seniorityLabel} {categoryNoun}", e.g. senior + backend → "Senior Backend
// Engineer".
var categoryNoun = map[string]string{
	"backend":             "Backend Engineer",
	"frontend":            "Frontend Engineer",
	"fullstack":           "Fullstack Engineer",
	"mobile":              "Mobile Engineer",
	"devops":              "DevOps Engineer",
	"sre":                 "Site Reliability Engineer",
	"network_engineering": "Network Engineer",
	"data_engineering":    "Data Engineer",
	"data_science":        "Data Scientist",
	"data_analytics":      "Data Analyst",
	"ml_ai":               "ML Engineer",
	"ai_engineering":      "AI Engineer",
	"qa":                  "QA Engineer",
	"security":            "Security Engineer",
	"hardware":            "Hardware Engineer",
	"embedded":            "Embedded Engineer",
	"blockchain":          "Blockchain Engineer",
	"architecture":        "Architect",
	"design":              "Designer",
	"product":             "Product Manager",
	"project_management":  "Project Manager",
	"management":          "Manager",
	"marketing":           "Marketing Specialist",
	"sales":               "Sales Specialist",
	"support":             "Support Specialist",
}

// namedRole pairs a title alias with its canonical slug. The list is ordered
// most-specific-first so an overlapping general alias never shadows a specific
// one ("cloud solutions engineer" wins over "solutions engineer"). At most one
// named role is emitted per title — the first match — mirroring classify.
var namedRoles = []struct{ alias, slug string }{
	{"cloud solutions engineer", "cloud_solutions_engineer"},
	{"solutions engineer", "solutions_engineer"},
	{"founding engineer", "founding_engineer"},
	{"technical lead", "technical_lead"},
	{"tech lead", "technical_lead"},
	{"fractional cto", "fractional_cto"},
	{"staff engineer", "staff_engineer"},
}

// namedLabel maps each named-role slug to its display label.
var namedLabel = map[string]string{
	"cloud_solutions_engineer": "Cloud Solutions Engineer",
	"solutions_engineer":       "Solutions Engineer",
	"founding_engineer":        "Founding Engineer",
	"technical_lead":           "Technical Lead",
	"fractional_cto":           "Fractional CTO",
	"staff_engineer":           "Staff Engineer",
}

// Derive returns a job's canonical role slugs from its resolved seniority,
// resolved category, and title. It emits the composite {seniority}_{category}
// when both axes are non-empty and the category has a natural role noun, plus at
// most one named role matched whole-word in the title. That is at most one of
// each and their slug namespaces cannot collide, so the result carries no
// duplicates. Every slug exists in Catalog; an unresolved input contributes
// nothing.
func Derive(seniority, category, title string) []string {
	var roles []string

	if seniority != "" && category != "" {
		// categoryNoun membership is the decomposable-category set: a composite is
		// emitted only for a category that has a natural role noun (which excludes
		// "other", where "{Seniority} Other" would be meaningless).
		if _, ok := categoryNoun[category]; ok {
			roles = append(roles, seniority+"_"+category)
		}
	}

	lower := strings.ToLower(title)
	for _, nr := range namedRoles {
		if wordmatch.Contains(lower, nr.alias, wordmatch.UnicodeBoundary) {
			roles = append(roles, nr.slug)
			break
		}
	}

	return roles
}

// Catalog returns the full role catalog — every derivable slug mapped to its
// human label. Composites are generated from the seniority × category label
// maps; named roles are curated. It is the source of truth for picker labels.
func Catalog() map[string]string {
	cat := make(map[string]string, len(seniorityLabel)*len(categoryNoun)+len(namedLabel))
	for sen, senLabel := range seniorityLabel {
		for c, noun := range categoryNoun {
			cat[sen+"_"+c] = senLabel + " " + noun
		}
	}
	for slug, label := range namedLabel {
		cat[slug] = label
	}
	return cat
}
