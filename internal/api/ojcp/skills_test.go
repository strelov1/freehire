package ojcp

import (
	"encoding/json"
	"slices"
	"testing"
)

// requirementsJSON builds the stored enrichment for a posting whose requirements the
// pipeline has extracted, each carrying the priority the posting itself stated.
func requirementsJSON(t *testing.T, items ...[2]string) json.RawMessage {
	t.Helper()

	type requirement struct {
		Text     string `json:"text"`
		Priority string `json:"priority"`
	}
	reqs := make([]requirement, 0, len(items))
	for _, item := range items {
		reqs = append(reqs, requirement{Text: item[0], Priority: item[1]})
	}
	raw, err := json.Marshal(map[string]any{"requirements": reqs})
	if err != nil {
		t.Fatalf("marshalling enrichment: %v", err)
	}
	return raw
}

func TestSkillsSplitByWhatThePostingActuallyDemands(t *testing.T) {
	// jobs.skills resolves every skill named ANYWHERE in the description, including a
	// "nice to have" block. Publishing that as skills_required tells an agent a candidate
	// without Kubernetes does not qualify for a role that never asked for it.
	row := openPostingRow()
	row.Enrichment = requirementsJSON(t,
		[2]string{"5+ years of Go and PostgreSQL in production", "required"},
		[2]string{"Familiarity with Kubernetes and Terraform is a plus", "preferred"},
	)

	posting := projector().JobPosting(viewOf(row), nil)

	if !slices.Contains(posting.SkillsRequired, "Go") {
		t.Errorf("skills_required = %v, want Go", posting.SkillsRequired)
	}
	if slices.Contains(posting.SkillsRequired, "Kubernetes") {
		t.Errorf("skills_required = %v, want Kubernetes out of it — the posting called it a plus", posting.SkillsRequired)
	}
	if !slices.Contains(posting.SkillsPreferred, "Kubernetes") {
		t.Errorf("skills_preferred = %v, want Kubernetes", posting.SkillsPreferred)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting with split skills rejected: %v", err)
	}
}

func TestSkillsAreNamedTheWayAReaderWritesThem(t *testing.T) {
	// The facet stores canonical slugs — `postgresql`, `cpp`, `ci-cd`. An agent relaying one
	// to a candidate, or matching it against a CV, is working with our internal key rather
	// than with the skill, so the curated spelling is what goes out.
	row := openPostingRow()
	row.Enrichment = requirementsJSON(t, [2]string{"Deep PostgreSQL and Kubernetes experience", "required"})

	posting := projector().JobPosting(viewOf(row), nil)

	if slices.Contains(posting.SkillsRequired, "postgresql") {
		t.Errorf("skills_required carries the internal slug: %v", posting.SkillsRequired)
	}
	if !slices.Contains(posting.SkillsRequired, "PostgreSQL") {
		t.Errorf("skills_required = %v, want the reader-facing spelling", posting.SkillsRequired)
	}
}

func TestSkillsSayNothingWhenTheRequirementsAreUnknown(t *testing.T) {
	// A posting whose requirements were never extracted has no evidence of what is demanded
	// versus merely mentioned. Falling back to the whole skills facet would restore exactly
	// the wrong claim this split exists to remove.
	row := openPostingRow()
	row.Skills = []string{"go", "kubernetes"}

	posting := projector().JobPosting(viewOf(row), nil)

	if len(posting.SkillsRequired) != 0 {
		t.Errorf("skills_required = %v, want nothing without stated requirements", posting.SkillsRequired)
	}
	if len(posting.SkillsPreferred) != 0 {
		t.Errorf("skills_preferred = %v, want nothing without stated requirements", posting.SkillsPreferred)
	}
}

func TestASkillIsNeverBothRequiredAndPreferred(t *testing.T) {
	// A posting may name the same skill in both blocks. Listing it twice would let an agent
	// read the preferred entry and treat a hard requirement as optional.
	row := openPostingRow()
	row.Enrichment = requirementsJSON(t,
		[2]string{"Go and PostgreSQL in production", "required"},
		[2]string{"Go tooling experience welcome", "preferred"},
	)

	posting := projector().JobPosting(viewOf(row), nil)

	if !slices.Contains(posting.SkillsRequired, "Go") {
		t.Errorf("skills_required = %v, want Go", posting.SkillsRequired)
	}
	if slices.Contains(posting.SkillsPreferred, "Go") {
		t.Errorf("skills_preferred = %v, want Go listed only as required", posting.SkillsPreferred)
	}
}
