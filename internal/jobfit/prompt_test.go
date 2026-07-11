package jobfit

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
