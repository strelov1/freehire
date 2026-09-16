package ojcp

import (
	"testing"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// greenhouseOnly is what this deployment can submit to unattended today: the chromedp fill
// path covers Greenhouse alone (fillProviders in internal/api/atsapply). Everything else
// tailors, gets reviewed, and then parks with a named reason.
var greenhouseOnly = map[string]bool{"greenhouse": true}

func projector() Projector {
	return Projector{Origin: testOrigin, Submittable: greenhouseOnly}
}

func capturedForm(provider string) *applyform.Form {
	return &applyform.Form{
		Provider: provider,
		Fields: []applyform.Field{
			{ID: "question_67165648", Label: "Why do you want to work here?", Required: true},
			{ID: "question_67165649", Label: "Years of Go experience", Required: true},
			{ID: "question_67165650", Label: "How did you hear about us?", Required: false},
			{ID: "gender", Label: "Gender", Required: true, Demographic: true},
		},
	}
}

func TestApplyPathsNameTheATSAndItsRequiredQuestions(t *testing.T) {
	j := openPosting()

	posting := projector().JobPosting(j, capturedForm("greenhouse"))

	if len(posting.ApplyPaths) != 1 {
		t.Fatalf("apply_paths = %d entries, want 1", len(posting.ApplyPaths))
	}
	path := posting.ApplyPaths[0]
	if path.Type != "ats_direct" {
		t.Errorf("type = %q, want ats_direct", path.Type)
	}
	if path.ATSProvider != "greenhouse" {
		t.Errorf("ats_provider = %q", path.ATSProvider)
	}
	if len(path.RequiredFields) != 2 {
		t.Errorf("required_fields = %v, want only the two required non-demographic questions", path.RequiredFields)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting with apply_paths rejected: %v", err)
	}
}

func TestApplyPathsLeaveOutTheEqualOpportunityBlock(t *testing.T) {
	// The platform itself files these separately from the employer's own questions, and
	// OJCP has its own eeo-data schema for them. Folding a demographic question into
	// required_fields would tell an agent that answering it decides the application.
	posting := projector().JobPosting(openPosting(), capturedForm("greenhouse"))

	for _, field := range posting.ApplyPaths[0].RequiredFields {
		if field == "Gender" {
			t.Fatalf("required_fields carries a demographic question: %v", posting.ApplyPaths[0].RequiredFields)
		}
	}
}

func TestSupportsAgentSubmissionIsTrueOnlyWhereWeCanActuallySubmit(t *testing.T) {
	// The flag an agent plans around. An optimistic value here is worse than no value:
	// it sends the agent down a path that parks, and the candidate spent a tailoring turn.
	for _, tc := range []struct {
		provider string
		want     bool
	}{
		{"greenhouse", true},
		{"lever", false},    // has a fill path that a captcha defeats ~7 times in 8
		{"ashby", false},    // fetches a schema, cannot submit
		{"workable", false}, // same
		{"recruitee", false},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			j := openPosting()
			j.Source = tc.provider

			posting := projector().JobPosting(j, capturedForm(tc.provider))

			if got := posting.ApplyPaths[0].SupportsAgentSubmission; got != tc.want {
				t.Errorf("supports_agent_submission = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSupportsAgentSubmissionFollowsThisDeploymentNotAConstant(t *testing.T) {
	// Whether Ashby and Workable can be submitted to depends on whether the cloud-agent
	// fallback is enabled and funded on THIS deployment — configuration a pure projection
	// cannot discover, so it is handed in rather than baked into a package-level map.
	withCloudAgent := Projector{
		Origin:      testOrigin,
		Submittable: map[string]bool{"greenhouse": true, "ashby": true, "workable": true},
	}
	j := openPosting()
	j.Source = "ashby"

	posting := withCloudAgent.JobPosting(j, capturedForm("ashby"))

	if !posting.ApplyPaths[0].SupportsAgentSubmission {
		t.Error("supports_agent_submission = false on a deployment that can submit to ashby")
	}
}

func TestPostingWithNoCapturedFormStillOffersAWayToApply(t *testing.T) {
	// Omitting apply_paths would read as "there is no way to apply", which is never true —
	// the source's own page always is one. An agent hands the candidate the link.
	posting := projector().JobPosting(openPosting(), nil)

	if len(posting.ApplyPaths) != 1 {
		t.Fatalf("apply_paths = %d entries, want one redirect", len(posting.ApplyPaths))
	}
	path := posting.ApplyPaths[0]
	if path.Type != "external_redirect" {
		t.Errorf("type = %q, want external_redirect", path.Type)
	}
	if path.URL != "https://boards.greenhouse.io/acme/jobs/4012" {
		t.Errorf("url = %q, want the source's own link", path.URL)
	}
	if path.SupportsAgentSubmission {
		t.Error("supports_agent_submission = true for a path we never captured a form for")
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("redirect-only posting rejected: %v", err)
	}
}

func TestApplyPathRequiredFieldsFallBackToTheOpaqueIdentifier(t *testing.T) {
	// A field the platform gave no label to still has to be named, or an agent counts
	// one fewer question than the form will actually refuse to submit without.
	form := &applyform.Form{
		Provider: "greenhouse",
		Fields:   []applyform.Field{{ID: "question_991", Required: true}},
	}

	posting := projector().JobPosting(openPosting(), form)

	fields := posting.ApplyPaths[0].RequiredFields
	if len(fields) != 1 || fields[0] != "question_991" {
		t.Errorf("required_fields = %v, want the identifier as the fallback name", fields)
	}
}
