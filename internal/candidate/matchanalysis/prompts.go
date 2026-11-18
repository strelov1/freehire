package matchanalysis

import (
	"encoding/json"
	"strings"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// freeTextLanguageDirective tells the model which language to write every free-text field of a
// stage's output in. These are the candidate's own reading of their fit, not CV text, so they
// follow the caller's profile language rather than the vacancy's (unlike tailoring's cv_edit).
func freeTextLanguageDirective(language string) string {
	return "\n\nWrite every free-text value in " + assistant.LanguageName(language) + ", regardless of what language the job posting or the candidate's CV is in.\n"
}

// stage1LanguageDirective is freeTextLanguageDirective plus one exception: the "quote" field
// is a verbatim excerpt of the job description, so translating it would make it unverifiable.
func stage1LanguageDirective(language string) string {
	return freeTextLanguageDirective(language) +
		"Exception: \"quote\" must stay a verbatim excerpt copied from the job posting, in the posting's own language — never translate it.\n"
}

// stage1SystemPrompt pins the ATS extract-and-match contract.
func stage1SystemPrompt(language string) string {
	var b strings.Builder
	b.WriteString("You are an ATS (applicant tracking system) parser. Return ONLY a JSON object.\n\n")
	b.WriteString("From the job posting, extract the explicit requirements (skills, tools, experience, ")
	b.WriteString("responsibilities) plus the role-title and seniority signals. Classify each against the ")
	b.WriteString("candidate's CV. Return exactly these keys:\n")
	b.WriteString("- \"requirements\": an array (max 30) of objects, each:\n")
	b.WriteString("  - \"text\": the requirement, short.\n")
	b.WriteString("  - \"priority\": \"required\" or \"preferred\".\n")
	b.WriteString("  - \"status\": one of \"covered\" (present verbatim/trivial inflection), ")
	b.WriteString("\"synonym-only\" (the concept is present under a different term), ")
	b.WriteString("\"missing-have\" (the CV evidences it elsewhere but never states the term), ")
	b.WriteString("\"missing-gap\" (a genuine gap — absent, no close equivalent held).\n")
	b.WriteString("  - \"evidence\": where it appears in the CV, or why it is absent.\n")
	b.WriteString("  - \"evidence_strength\": for \"covered\"/\"synonym-only\" only, grade the cited evidence ")
	b.WriteString("as \"metric\" (an accomplishment with a number, scale, or measured outcome), ")
	b.WriteString("\"scope\" (breadth: teams, systems, regions), \"responsibility\" (clear ownership with ")
	b.WriteString("tools or methods), or \"keyword\" (the term is present but only a bare mention or ")
	b.WriteString("duty). Omit it for \"missing-have\"/\"missing-gap\".\n")
	b.WriteString("Base every judgement only on the CV text. NEVER fabricate a skill the CV does not ")
	b.WriteString("evidence — a genuine gap is \"missing-gap\", never hidden.\n\n")
	b.WriteString("Also return \"hidden_signals\": an array (max 5) of unstated culture/pace/team-stage ")
	b.WriteString("signals read from the posting's own wording — not from any explicit requirement. Each ")
	b.WriteString("object has \"quote\" (a short verbatim excerpt from the job description) and \"insight\" ")
	b.WriteString("(one line on what that wording implies about pace, ownership expectations, team stage, ")
	b.WriteString("or culture — e.g. \"comfortable with ambiguity\" implies no one will hand the candidate a ")
	b.WriteString("spec). Base every signal on wording actually present in the posting; if the posting is ")
	b.WriteString("short or generic and carries no distinctive wording, return an empty array — never invent ")
	b.WriteString("a signal to fill it.\n")
	b.WriteString(stage1LanguageDirective(language))
	return b.String()
}

// stage2SystemPrompt pins the recruiter six-dimension scoring contract.
func stage2SystemPrompt(language string) string {
	var b strings.Builder
	b.WriteString("You are a senior technical recruiter judging how well a candidate fits ONE role. ")
	b.WriteString("Return ONLY a JSON object. Base every judgement on the CV and the requirement match ")
	b.WriteString("provided; do not invent facts.\n\n")
	b.WriteString("Score each dimension 0-100 with a one-sentence \"comment\". Job-title alignment and ")
	b.WriteString("experience relevance matter most. Return exactly these keys:\n")
	b.WriteString("- \"title_alignment\": does the candidate's current/target title match this role's title?\n")
	b.WriteString("- \"experience_relevance\": how relevant is their domain and role-type experience?\n")
	b.WriteString("- \"seniority_fit\": does their level match the role's seniority?\n")
	b.WriteString("- \"skills_coverage\": consistent with the provided deterministic skills match. ")
	// See evidenceStrengthRule.
	b.WriteString(evidenceStrengthRule())
	b.WriteString("\n")
	b.WriteString("- \"company_context\": fit with the company's stage/industry (from the company info).\n")
	b.WriteString("- \"location_fit\": can the candidate actually take the role given the job's location/work ")
	b.WriteString("mode and their location preferences (accepted work modes, remote reach, base, relocation)? ")
	b.WriteString("For a REMOTE role, judge ONLY whether its region or countries fall within the candidate's ")
	b.WriteString("remote reach — a reach of \"global\", or one naming the job's region, covers any city or ")
	b.WriteString("country in that region; ignore the candidate's physical base and relocation entirely, and ")
	b.WriteString("never treat a remote posting's office city as a relocation requirement. Relocation matters ")
	b.WriteString("only for onsite or hybrid roles: an onsite job where they are based or will relocate scores ")
	b.WriteString("high; an onsite job far from their base with no relocation and a remote-only preference ")
	b.WriteString("scores low. Honour any NOTE about remote reach in the input. If the candidate stated no ")
	b.WriteString("preferences, judge on the job alone and do not penalise.\n")
	b.WriteString("Each of the six is an object {\"score\": int 0-100, \"comment\": string}.\n")
	b.WriteString("Also return \"strengths\" (array, max 6), \"gaps\" (array, max 6), and a ")
	b.WriteString("\"recommendation\" string: two or three short prose paragraphs of hiring judgement ")
	b.WriteString("(no headings, no lists). Do not recap per-requirement statuses or evidence strengths ")
	b.WriteString("— those are already on the page. Do NOT return an overall score — it is computed separately.\n")
	b.WriteString(freeTextLanguageDirective(language))
	return b.String()
}

// stage3SystemPrompt pins the adversarial-audit contract (same output shape as Stage 2).
func stage3SystemPrompt(language string) string {
	var b strings.Builder
	b.WriteString("You are a skeptical hiring manager auditing a recruiter's fit verdict. ")
	b.WriteString("Return ONLY a JSON object in the SAME shape as the verdict you are given.\n\n")
	b.WriteString("Challenge it against the CV evidence: lower any inflated dimension score, remove ")
	b.WriteString("strengths the CV does not actually support, and surface gaps that were glossed over. ")
	// The audit must still check the rule the recruiter scores under.
	b.WriteString(evidenceStrengthRule())
	b.WriteString(" ")
	b.WriteString("Keep what is well-supported. Return the corrected verdict with the same keys ")
	b.WriteString("(title_alignment, experience_relevance, seniority_fit, skills_coverage, ")
	b.WriteString("company_context, location_fit, strengths, gaps, recommendation). ")
	b.WriteString("The recommendation remains two or three short prose paragraphs of hiring judgement ")
	b.WriteString("(no headings, no lists); do not recap per-requirement statuses or evidence strengths. ")
	b.WriteString("Do NOT fabricate anything.\n")
	b.WriteString(freeTextLanguageDirective(language))
	return b.String()
}

// stage1UserPrompt carries the de-identified structured résumé (the candidate context — no raw
// CV is sent), the (bounded) job text and the deterministic anchor.
func stage1UserPrompt(in Input, candidate string) string {
	var b strings.Builder
	writeCandidate(&b, candidate)
	writeJob(&b, in)
	writeAnchor(&b, in.Match)
	writeBlockers(&b, in.Blockers)
	return b.String()
}

// stage2UserPrompt adds the company info and the Stage-1 requirement match.
func stage2UserPrompt(in Input, reqs []Requirement, candidate string) string {
	var b strings.Builder
	writeCandidate(&b, candidate)
	writeJob(&b, in)
	if info := strings.TrimSpace(in.CompanyInfo); info != "" {
		b.WriteString("Company info (JSON):\n")
		b.WriteString(llm.TruncateRunes(info, maxCompanyRunes))
		b.WriteString("\n\n")
	}
	writeAnchor(&b, in.Match)
	writeLocation(&b, in)
	writeBlockers(&b, in.Blockers)
	writeRequirements(&b, reqs)
	return b.String()
}

// stage3UserPrompt carries the Stage-2 verdict to audit plus the same evidence.
func stage3UserPrompt(in Input, reqs []Requirement, v recruiterVerdict, candidate string) string {
	var b strings.Builder
	writeCandidate(&b, candidate)
	b.WriteString("Verdict to audit (JSON):\n")
	if blob, err := json.Marshal(v); err == nil {
		b.Write(blob)
		b.WriteString("\n\n")
	}
	writeBlockers(&b, in.Blockers)
	writeRequirements(&b, reqs)
	return b.String()
}
