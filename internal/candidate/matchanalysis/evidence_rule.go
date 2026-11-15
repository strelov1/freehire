package matchanalysis

// evidenceStrengthRule is what a `required` requirement's weak evidence is worth, in the
// vocabulary Stage 1 stamps onto each requirement.
//
// Both scoring stages need it: with the rule only in the audit, Stage 2 over-scored and the
// audit spent a model call correcting it. `synonym-only` and `keyword` are the exact statuses
// Stage 1 writes (see Requirement.Status and evidence_strength); without them a stage must
// guess which inputs count as weak.
func evidenceStrengthRule() string {
	return "For any requirement marked \"required\", treat weak evidence as thin support: a " +
		"\"synonym-only\" match, or a \"covered\" match graded \"keyword\" strength (a bare " +
		"mention rather than a metric-, scope-, or responsibility-backed one), is adjacent " +
		"exposure, not direct ownership — it may earn partial credit but must not by itself " +
		"sustain a high skills_coverage score."
}
