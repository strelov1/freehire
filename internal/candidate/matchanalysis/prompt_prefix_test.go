package matchanalysis

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// A provider prompt cache reuses only an identical prefix. The candidate context is what
// repeats when one candidate analyses several vacancies, so every stage's user prompt opens
// with it; anything job-specific ahead of it would make the whole block a cache miss.
func TestStageUserPrompts_OpenWithTheCandidateContext(t *testing.T) {
	resume := resumeextract.Professional{
		Summary:    "Go dev",
		Experience: []resumeextract.Experience{{Company: "Acme", Title: "Backend Engineer"}},
	}
	candidate := candidateContext(resume)
	var block strings.Builder
	writeCandidate(&block, candidate)

	reqs := []Requirement{{Text: "Go", Priority: "required", Status: "covered"}}
	for _, job := range []Input{
		{JobTitle: "Go Engineer", JobDescription: "Build APIs in Go.", StructuredResume: resume},
		{JobTitle: "Platform Engineer", JobDescription: "Run Kubernetes.", CompanyInfo: `{"size":"50"}`, StructuredResume: resume},
	} {
		prompts := map[string]string{
			"stage 1": stage1UserPrompt(job, candidate),
			"stage 2": stage2UserPrompt(job, reqs, candidate),
			"stage 3": stage3UserPrompt(job, reqs, recruiterVerdict{}, candidate),
		}
		for stage, got := range prompts {
			if !strings.HasPrefix(got, block.String()) {
				t.Errorf("%s prompt for %q does not open with the candidate context:\n%s", stage, job.JobTitle, got)
			}
			if strings.Count(got, "Candidate (structured résumé") != 1 {
				t.Errorf("%s prompt for %q must carry the candidate context exactly once:\n%s", stage, job.JobTitle, got)
			}
		}
	}
}
