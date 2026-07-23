package matchanalysis

import "strings"

import "testing"

func TestStage1Prompt_SendsDeIdentifiedStructuredNotRawCV(t *testing.T) {
	in := Input{JobTitle: "Go Engineer", CVText: "raw cv", StructuredResume: `{"full_name":"Jane","email":"jane@x.com","summary":"Go dev"}`}
	got := stage1UserPrompt(in, candidateContext(in.StructuredResume))
	if !strings.Contains(got, `"summary":"Go dev"`) {
		t.Errorf("stage1 prompt missing the structured candidate context:\n%s", got)
	}
	if strings.Contains(got, "Jane") || strings.Contains(got, "jane@x.com") {
		t.Errorf("contacts must be stripped from the candidate context:\n%s", got)
	}
	if strings.Contains(got, "raw cv") {
		t.Errorf("the raw CV must never be sent to the model:\n%s", got)
	}
}

func TestStage1Prompt_OmitsCandidateBlockWhenNoStructured(t *testing.T) {
	withEmpty := stage1UserPrompt(Input{JobTitle: "Go Engineer", CVText: "raw cv"}, candidateContext(""))
	if strings.Contains(withEmpty, "Candidate (structured résumé") {
		t.Errorf("stage1 prompt should omit the candidate block when there is no structured résumé:\n%s", withEmpty)
	}
	if strings.Contains(withEmpty, "raw cv") {
		t.Errorf("the raw CV must never be sent to the model:\n%s", withEmpty)
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

func TestStage2SystemPrompt_RemoteLocationRule(t *testing.T) {
	sp := stage2SystemPrompt()
	if !strings.Contains(sp, "remote reach") || !strings.Contains(sp, "Relocation matters only for onsite") {
		t.Errorf("stage2 system prompt must instruct remote-only location scoring:\n%s", sp)
	}
}

func TestStage3SystemPrompt_SynonymOnlyRequiredDiscipline(t *testing.T) {
	// The skeptic must not let thin evidence on a required requirement prop up
	// skills_coverage — an adjacent-exposure "synonym-only" match, or a "covered" match
	// backed only by a bare "keyword" mention, is not direct ownership. Guards against the
	// hard-negative the audit pass exists to catch (deploying Helm ≠ owning the skill).
	sp := stage3SystemPrompt()
	for _, want := range []string{"synonym-only", "keyword", "adjacent"} {
		if !strings.Contains(sp, want) {
			t.Errorf("stage3 system prompt must demote weak matches on required items (missing %q):\n%s", want, sp)
		}
	}
}

func TestStage1SystemPrompt_GradesEvidenceStrength(t *testing.T) {
	// Stage 1 must ask for evidence_strength on positive statuses and name the four tiers,
	// so the audit and served verdict can tell a metric-backed match from a bare keyword.
	sp := stage1SystemPrompt()
	for _, want := range []string{"evidence_strength", "metric", "scope", "responsibility", "keyword"} {
		if !strings.Contains(sp, want) {
			t.Errorf("stage1 system prompt must request graded evidence (missing %q):\n%s", want, sp)
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
