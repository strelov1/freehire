package coverletter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// The prompts are functions rather than constants so they are testable on their own, the same
// shape enrich and atscheck keep.

func selectSystemPrompt() string {
	var b strings.Builder
	b.WriteString("You are choosing which of a candidate's banked achievements a cover letter should stand on. ")
	b.WriteString("Return ONLY a JSON object.\n\n")
	b.WriteString("Return exactly this key:\n")
	b.WriteString("- \"selected\": an array of 2 to 4 achievement ids, copied verbatim from the list given to you.\n\n")
	b.WriteString("Choose the achievements that answer the requirements listed under REQUIREMENTS THE CV UNDERSELLS. ")
	b.WriteString("Those are requirements the candidate genuinely meets but whose evidence the CV leaves implicit — ")
	b.WriteString("closing that gap is the letter's job, because the CV cannot.\n")
	b.WriteString("Do NOT choose evidence for the requirements under GENUINE GAPS. The candidate does not meet those, ")
	b.WriteString("and the letter will not claim otherwise.\n")
	b.WriteString("Choose no id that is not in the list. Prefer achievements carrying a measured outcome.\n")
	return b.String()
}

func selectUserPrompt(in Input, atoms []experience.Atom) (string, error) {
	var b strings.Builder
	b.WriteString(vacancyBlock(in))
	b.WriteString("\nREQUIREMENTS THE CV UNDERSELLS:\n")
	b.WriteString(requirementList(in.Context.MissingHave))
	b.WriteString("\nGENUINE GAPS (do not answer these):\n")
	b.WriteString(requirementList(in.Context.MissingGap))
	b.WriteString("\nBANKED ACHIEVEMENTS:\n")
	blob, err := json.Marshal(atomsForPrompt(atoms))
	if err != nil {
		return "", fmt.Errorf("coverletter: marshal atoms: %w", err)
	}
	b.WriteString(string(blob))
	b.WriteString("\n")
	return b.String(), nil
}

func draftSystemPrompt(band Band, bounds Bounds) string {
	var b strings.Builder
	b.WriteString("You are writing a cover letter for a candidate applying to one vacancy. ")
	b.WriteString("Return ONLY a JSON object.\n\n")
	b.WriteString("Return exactly this key:\n")
	b.WriteString("- \"body\": the letter, as plain text with paragraph breaks. No markdown, no placeholders, ")
	fmt.Fprintf(&b, "no square brackets, and at most %d characters.\n\n", bounds.ceiling(band))
	b.WriteString("Rules:\n")
	b.WriteString("- Every statement about what the candidate has DONE must come from the achievements given to you. ")
	b.WriteString("Do not invent an achievement, a number, an employer or a technology.\n")
	b.WriteString("- Reframe an achievement in the vacancy's own vocabulary where it honestly matches. ")
	b.WriteString("Reframing is not inventing; asserting experience the achievements do not show is.\n")
	b.WriteString("- Say why the candidate is interested in this role and this employer. That is your own to write — ")
	b.WriteString("it is motivation, not a claim about their past.\n")
	b.WriteString("- Never claim experience with anything listed as a genuine gap. Adjacent evidence may be offered ")
	b.WriteString("as adjacent, named honestly, or left out.\n")
	b.WriteString("- Do not restate the CV. The letter earns its place by saying what the CV does not.\n")
	return b.String()
}

func draftUserPrompt(in Input, atoms []experience.Atom, language string) (string, error) {
	var b strings.Builder
	b.WriteString("Write the letter in this language (ISO code): " + language + ".\n")
	b.WriteString("It is the language of the VACANCY. The employer reads this letter, not the candidate.\n\n")
	b.WriteString(vacancyBlock(in))
	b.WriteString("\nREQUIREMENTS THE CV UNDERSELLS (this is what the letter is for):\n")
	b.WriteString(requirementList(in.Context.MissingHave))
	b.WriteString("\nGENUINE GAPS (claim no experience with these):\n")
	b.WriteString(requirementList(in.Context.MissingGap))
	b.WriteString("\nACHIEVEMENTS THE LETTER MAY STAND ON:\n")
	blob, err := json.Marshal(atomsForPrompt(atoms))
	if err != nil {
		return "", fmt.Errorf("coverletter: marshal atoms: %w", err)
	}
	b.WriteString(string(blob))
	b.WriteString("\n\nCANDIDATE (structured, contact-free):\n")
	cand, err := json.Marshal(in.Candidate)
	if err != nil {
		return "", fmt.Errorf("coverletter: marshal candidate: %w", err)
	}
	b.WriteString(llm.TruncateRunes(string(cand), maxCandidateRunes))
	b.WriteString("\n")
	return b.String(), nil
}

func auditSystemPrompt(band Band, bounds Bounds) string {
	var b strings.Builder
	b.WriteString("You are a skeptical reviewer of a cover letter. Your job is to CUT. ")
	b.WriteString("Return ONLY a JSON object.\n\n")
	b.WriteString("Return exactly these keys:\n")
	b.WriteString("- \"body\": the letter after your cuts, as plain text.\n")
	b.WriteString("- \"cited_atom_ids\": the ids of the achievements the SURVIVING letter still rests on, ")
	b.WriteString("copied verbatim from the list given to you. Omit an achievement whose sentence you cut — ")
	b.WriteString("it is no longer evidence for anything the letter says.\n\n")
	b.WriteString("Cut, in this order:\n")
	b.WriteString("1. Any sentence claiming the candidate did something that the listed achievements do not support. ")
	b.WriteString("This is the only cut you must make; make it without exception.\n")
	b.WriteString("2. Sentences that say nothing a recruiter can act on — enthusiasm without a subject, ")
	b.WriteString("restatements of the job title, filler openings.\n")
	fmt.Fprintf(&b, "3. Whatever still puts the letter over %d characters.\n\n", bounds.ceiling(band))
	b.WriteString("Keep: statements of interest in the role or the employer, the address and the closing. ")
	b.WriteString("Those assert nothing about the candidate's past and are not yours to cut.\n")
	b.WriteString("Do not add anything. Do not rewrite what survives beyond what a cut requires. ")
	b.WriteString("Return a real letter — if your cuts would leave less than a paragraph, cut less.\n")
	return b.String()
}

// auditUserPrompt carries BOTH the letter and the achievements it is meant to be checked
// against. Sending the letter alone would ask the skeptic to verify support against a list it
// was never given, which makes the one cut it must always make unenforceable — the stage
// would still shorten the letter and silently pass every invented claim.
func auditUserPrompt(drafted Letter, atoms []experience.Atom) (string, error) {
	blob, err := json.Marshal(atomsForPrompt(atoms))
	if err != nil {
		return "", fmt.Errorf("coverletter: marshal atoms: %w", err)
	}
	var b strings.Builder
	b.WriteString("ACHIEVEMENTS THE LETTER MAY REST ON:\n")
	b.Write(blob)
	b.WriteString("\n\nLETTER:\n")
	b.WriteString(drafted.Body)
	b.WriteString("\n")
	return b.String(), nil
}

// vacancyBlock is the posting as the model sees it: bounded, and the same clipped shape every
// other tailoring reader gets.
func vacancyBlock(in Input) string {
	var b strings.Builder
	b.WriteString("VACANCY:\n")
	b.WriteString("Title: " + in.Context.Job.Title + "\n")
	b.WriteString("Company: " + in.Context.Job.Company + "\n")
	b.WriteString("Description:\n")
	b.WriteString(llm.TruncateRunes(in.Context.Job.Description, maxPostingRunes))
	b.WriteString("\n")
	return b.String()
}

func requirementList(reqs []matchanalysis.Requirement) string {
	if len(reqs) == 0 {
		return "(none)\n"
	}
	var b strings.Builder
	for _, r := range reqs {
		b.WriteString("- " + r.Text)
		if r.Priority != "" {
			b.WriteString(" (" + r.Priority + ")")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// promptAtom is the shape an atom takes in a prompt. Provenance is deliberately absent: only
// publishable atoms ever reach here, so naming the label would invite the model to reason
// about a distinction that has already been decided.
type promptAtom struct {
	ID      string   `json:"id"`
	Claim   string   `json:"claim"`
	Context string   `json:"context,omitempty"`
	Metrics []string `json:"metrics,omitempty"`
	Skills  []string `json:"skills,omitempty"`
}

func atomsForPrompt(atoms []experience.Atom) []promptAtom {
	out := make([]promptAtom, 0, len(atoms))
	for _, a := range atoms {
		out = append(out, promptAtom{
			ID:      a.ID.String(),
			Claim:   a.Claim,
			Context: a.Context,
			Metrics: a.Metrics,
			Skills:  a.Skills,
		})
	}
	return out
}
