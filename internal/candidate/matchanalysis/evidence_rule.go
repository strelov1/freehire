package matchanalysis

// evidenceStrengthRule is what a `required` requirement's weak evidence is worth, in the
// vocabulary Stage 1 stamps onto each requirement.
//
// It is one sentence in one place because BOTH scoring stages need it. It used to live in
// the audit's prompt alone, and production measured the cost of that: over three vacancies
// on 2026-09-08 Stage 2 scored skills_coverage 95, 100 and 93, and the audit pulled each
// down to 62, 70 and 74. Same direction, near-identical magnitude, every time. A spread that
// consistent is not two models judging differently — it is one stage applying a rule the
// other was never given, and paying a whole extra model call to apply it.
//
// The words matter as much as the rule. `synonym-only` and `keyword` are not descriptions
// of weakness; they are the exact statuses Stage 1 writes onto a requirement (see
// Requirement.Status and evidence_strength). A stage told "weak evidence counts for less"
// without them is being asked to guess which of its inputs count as weak.
func evidenceStrengthRule() string {
	return "For any requirement marked \"required\", treat weak evidence as thin support: a " +
		"\"synonym-only\" match, or a \"covered\" match graded \"keyword\" strength (a bare " +
		"mention rather than a metric-, scope-, or responsibility-backed one), is adjacent " +
		"exposure, not direct ownership — it may earn partial credit but must not by itself " +
		"sustain a high skills_coverage score."
}
