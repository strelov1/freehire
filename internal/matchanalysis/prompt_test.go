package matchanalysis

import "strings"

import "testing"

func TestStage1Prompt_IncludesStructuredResumeWhenPresent(t *testing.T) {
	in := Input{JobTitle: "Go Engineer", CVText: "raw cv", StructuredResume: `{"full_name":"Jane"}`}
	got := stage1UserPrompt(in)
	if !strings.Contains(got, `{"full_name":"Jane"}`) {
		t.Errorf("stage1 prompt missing structured résumé block:\n%s", got)
	}
	if !strings.Contains(got, "raw cv") {
		t.Error("stage1 prompt must still include the raw CV text (structure is additive, not a replacement)")
	}
}

func TestStage1Prompt_OmitsStructuredBlockWhenEmpty(t *testing.T) {
	withEmpty := stage1UserPrompt(Input{JobTitle: "Go Engineer", CVText: "raw cv"})
	// The structured header must not appear at all when there is no structured résumé,
	// so an un-extracted CV produces exactly today's prompt.
	if strings.Contains(withEmpty, "Structured résumé") {
		t.Errorf("stage1 prompt should omit the structured block when empty:\n%s", withEmpty)
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
	got := stage2UserPrompt(in, nil)
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
	got := stage2UserPrompt(in, nil)
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
