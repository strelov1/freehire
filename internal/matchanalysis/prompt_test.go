package matchanalysis

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/resumeextract"
)

// freehire#1837: the fit analysis is the candidate's own reading of their match, not
// text that goes onto a CV, so every free-text field follows their profile language.
func TestStageSystemPrompts_CarryTheLanguageDirective(t *testing.T) {
	for name, sp := range map[string]string{
		"stage1": stage1SystemPrompt("ru"),
		"stage2": stage2SystemPrompt("ru"),
		"stage3": stage3SystemPrompt("ru"),
	} {
		if !strings.Contains(sp, "Write every free-text value in Russian") {
			t.Errorf("%s system prompt does not name the requested language:\n%s", name, sp)
		}
	}
}

// Stage 1's "quote" field is defined as "a short verbatim excerpt from the job
// description" — a blanket translation instruction would break that contract, so the
// directive must carve it out explicitly rather than leave the model to notice the
// conflict on its own.
func TestStage1SystemPrompt_LanguageDirectiveExemptsQuote(t *testing.T) {
	sp := stage1SystemPrompt("ru")
	if !strings.Contains(sp, `Exception: "quote" must stay a verbatim excerpt`) {
		t.Errorf("stage1 system prompt does not exempt \"quote\" from the language directive:\n%s", sp)
	}
	if !strings.Contains(sp, "never translate it") {
		t.Errorf("stage1 system prompt does not explicitly forbid translating \"quote\":\n%s", sp)
	}
}

// Stage 2 and Stage 3 have no verbatim-excerpt field to protect — every value they
// return is the model's own prose — so their directive must stay the plain one rather
// than carrying stage1's exception where it names nothing that exists in their output.
func TestStage2And3SystemPrompt_LanguageDirectiveHasNoQuoteException(t *testing.T) {
	for name, sp := range map[string]string{
		"stage2": stage2SystemPrompt("ru"),
		"stage3": stage3SystemPrompt("ru"),
	} {
		if strings.Contains(sp, "Exception:") {
			t.Errorf("%s system prompt carries stage1's quote exception, which names a field it does not return:\n%s", name, sp)
		}
	}
}

func TestStage1Prompt_SendsDeIdentifiedStructured(t *testing.T) {
	// The caller projects; the chain cannot be handed the contact-bearing structure at all.
	structured := resumeextract.Structured{
		FullName:   "Jane",
		Email:      "jane@x.com",
		Summary:    "Go dev",
		Experience: []resumeextract.Experience{{Company: "Acme", Title: "Backend Engineer"}},
	}
	in := Input{JobTitle: "Go Engineer", StructuredResume: structured.Professional()}
	got := stage1UserPrompt(in, candidateContext(in.StructuredResume))
	if !strings.Contains(got, `"summary":"Go dev"`) {
		t.Errorf("stage1 prompt missing the structured candidate context:\n%s", got)
	}
	if strings.Contains(got, "Jane") || strings.Contains(got, "jane@x.com") {
		t.Errorf("contacts reached the candidate context:\n%s", got)
	}
}

// The field nobody has added yet can no longer reach this package at all: the argument's type
// names what a model may see. resumeextract's TestProfessional_WithholdsOnlyDeclaredFields is
// what now fails when somebody adds a field to the structured résumé without deciding.
//
// What remains testable here is the chain's own rule: a candidate with no banked experience
// yields no context, and the chain then produces no analysis rather than scoring against a
// work history nothing owns.
func TestCandidateContext_EmptyWithoutBankedExperience(t *testing.T) {
	if got := candidateContext(resumeextract.Professional{Summary: "Go dev"}); got != "" {
		t.Errorf("candidate context = %q, want empty without experience", got)
	}
}

func TestStage1Prompt_OmitsCandidateBlockWhenNoStructured(t *testing.T) {
	withEmpty := stage1UserPrompt(Input{JobTitle: "Go Engineer"}, candidateContext(resumeextract.Professional{}))
	if strings.Contains(withEmpty, "Candidate (structured résumé") {
		t.Errorf("stage1 prompt should omit the candidate block when there is no structured résumé:\n%s", withEmpty)
	}
}

func TestWriteLocation_RemoteWithinReachAddsNote(t *testing.T) {
	// A LATAM-remote role whose posted office happens to sit in one country (DR) — the
	// candidate's remote reach names `latam`, so they can take it without relocating. The
	// prompt must say so deterministically, never leaving the model to read the office city
	// as a relocation requirement (the false LATAM location-mismatch this guards against).
	in := Input{
		JobRemote:           true,
		JobLocation:         "Santo Domingo, Dominican Republic",
		JobRegions:          []string{"latam"},
		JobCountries:        []string{"do"},
		LocationPreferences: `{"base":{"country":"br"},"remote":{"regions":["global","latam","cis"]},"relocation":{"open":false}}`,
	}
	got := stage2UserPrompt(in, nil, candidateContext(in.StructuredResume))
	if !strings.Contains(got, "within the candidate's stated remote reach") {
		t.Errorf("expected remote-reach NOTE for a LATAM-remote job matching the candidate's reach:\n%s", got)
	}
}

func TestWriteLocation_RemoteOutsideReachNoNote(t *testing.T) {
	// The candidate's reach is Europe-only; a LATAM-remote role is genuinely out of reach,
	// so the deterministic vouch must NOT fire — the model judges it (and may score it low).
	in := Input{
		JobRemote:           true,
		JobRegions:          []string{"latam"},
		LocationPreferences: `{"remote":{"regions":["europe"]}}`,
	}
	got := stage2UserPrompt(in, nil, candidateContext(in.StructuredResume))
	if strings.Contains(got, "within the candidate's stated remote reach") {
		t.Errorf("must not vouch for a remote job outside the candidate's reach:\n%s", got)
	}
}

// The profile editor lets a candidate scope their remote reach by country independently
// of region (ProfileForm's "Remote reach" block: region pills + a country search, each
// toggled on its own). A candidate who picked only countries, no regions, must still get
// the deterministic vouch for a job in one of those countries.
func TestWriteLocation_RemoteWithinReachByCountryAddsNote(t *testing.T) {
	in := Input{
		JobRemote:           true,
		JobLocation:         "Berlin, Germany",
		JobCountries:        []string{"de"},
		LocationPreferences: `{"remote":{"countries":["de","pl"]}}`,
	}
	got := stage2UserPrompt(in, nil, candidateContext(in.StructuredResume))
	if !strings.Contains(got, "within the candidate's stated remote reach") {
		t.Errorf("expected remote-reach NOTE for a job in a country the candidate's reach names:\n%s", got)
	}
}

func TestStage2SystemPrompt_RemoteLocationRule(t *testing.T) {
	sp := stage2SystemPrompt("en")
	if !strings.Contains(sp, "remote reach") || !strings.Contains(sp, "Relocation matters only for onsite") {
		t.Errorf("stage2 system prompt must instruct remote-only location scoring:\n%s", sp)
	}
}

func TestStage2And3SystemPrompt_RecommendationBudget(t *testing.T) {
	// Both stages rewrite recommendation; each must state the same length/shape contract so
	// the model's target and the sanitizer ceiling describe the same thing.
	for name, sp := range map[string]string{
		"stage2": stage2SystemPrompt("en"),
		"stage3": stage3SystemPrompt("en"),
	} {
		for _, want := range []string{
			"two or three short prose paragraphs",
			"no headings, no lists",
			"do not recap per-requirement",
		} {
			if !strings.Contains(strings.ToLower(sp), strings.ToLower(want)) {
				t.Errorf("%s system prompt must state the recommendation budget (missing %q):\n%s", name, want, sp)
			}
		}
	}
}

func TestStage3SystemPrompt_SynonymOnlyRequiredDiscipline(t *testing.T) {
	// The skeptic must not let thin evidence on a required requirement prop up
	// skills_coverage — an adjacent-exposure "synonym-only" match, or a "covered" match
	// backed only by a bare "keyword" mention, is not direct ownership. Guards against the
	// hard-negative the audit pass exists to catch (deploying Helm ≠ owning the skill).
	sp := stage3SystemPrompt("en")
	for _, want := range []string{"synonym-only", "keyword", "adjacent"} {
		if !strings.Contains(sp, want) {
			t.Errorf("stage3 system prompt must demote weak matches on required items (missing %q):\n%s", want, sp)
		}
	}
}

func TestStage1SystemPrompt_GradesEvidenceStrength(t *testing.T) {
	// Stage 1 must ask for evidence_strength on positive statuses and name the four tiers,
	// so the audit and served verdict can tell a metric-backed match from a bare keyword.
	sp := stage1SystemPrompt("en")
	for _, want := range []string{"evidence_strength", "metric", "scope", "responsibility", "keyword"} {
		if !strings.Contains(sp, want) {
			t.Errorf("stage1 system prompt must request graded evidence (missing %q):\n%s", want, sp)
		}
	}
}

func TestStage1SystemPrompt_RequestsHiddenSignals(t *testing.T) {
	// Stage 1 must ask for hidden_signals (quote + insight, max 5) alongside the requirement
	// table, and must not force one on a generic posting.
	sp := stage1SystemPrompt("en")
	for _, want := range []string{"hidden_signals", "quote", "insight"} {
		if !strings.Contains(sp, want) {
			t.Errorf("stage1 system prompt must request hidden signals (missing %q):\n%s", want, sp)
		}
	}
}

func TestWriteRequirements_RendersStrengthForPositiveOnly(t *testing.T) {
	var b strings.Builder
	writeRequirements(&b, []Requirement{
		{Text: "Go", Priority: PriorityRequired, Status: StatusCovered, EvidenceStrength: StrengthMetric},
		{Text: "Kafka", Priority: PriorityRequired, Status: StatusMissingGap},
	})
	got := b.String()
	if !strings.Contains(got, "[required/covered/metric] Go") {
		t.Errorf("positive requirement must render its strength:\n%s", got)
	}
	if !strings.Contains(got, "[required/missing-gap] Kafka") {
		t.Errorf("missing requirement must render status-only (no trailing strength):\n%s", got)
	}
}
