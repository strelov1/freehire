package atsapply

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBuildTask_IncludesEveryFieldsExactLabelAndValue(t *testing.T) {
	plan := Plan{Fields: []ResolvedField{
		{ID: "first_name", Value: "Ada"},
		{ID: "question_42", Value: "Yes"},
	}}
	merged := []MergedField{
		{ID: "first_name", Label: "First name"},
		{ID: "question_42", Label: "Are you authorized to work here?"},
	}

	task := buildTask(plan, merged, "https://example.com/apply")

	for _, want := range []string{
		"https://example.com/apply",
		`"First name" (id "first_name"): Ada`,
		`"Are you authorized to work here?" (id "question_42"): Yes`,
		"Do not touch",
		string(outcomeConfirmed) + ":",
		string(outcomeUnconfirmed),
		string(outcomeParked) + ":",
	} {
		if !strings.Contains(task, want) {
			t.Errorf("task missing %q\n---\n%s", want, task)
		}
	}
}

func TestBuildTask_FallsBackToIDWhenNoLabelKnown(t *testing.T) {
	plan := Plan{Fields: []ResolvedField{{ID: "custom_field", Value: "42"}}}
	task := buildTask(plan, nil, "https://example.com/apply")
	if !strings.Contains(task, `"custom_field" (id "custom_field"): 42`) {
		t.Errorf("task = %s, want it to fall back to the id as the label", task)
	}
}

// A file-kind field's Value is a local filesystem path (attachApprovedResume's own
// output), meaningless on browser-use's cloud VM and never to be printed. The task must
// instead point the agent at the file already attached to the run's own workspace.
func TestBuildTask_NeverPrintsAFileFieldsLocalPathValue(t *testing.T) {
	plan := Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file", Value: "/tmp/auto-apply-resume-12345.pdf"}}}
	merged := []MergedField{{ID: "resume", Label: "Resume/CV"}}

	task := buildTask(plan, merged, "https://example.com/apply")

	if strings.Contains(task, "/tmp/") {
		t.Errorf("task leaked the local temp file path:\n%s", task)
	}
	if !strings.Contains(task, `"Resume/CV" (id "resume")`) || !strings.Contains(task, "attached to this run's workspace") {
		t.Errorf("task = %s, want it to reference the attached file for the resume field", task)
	}
}

func TestResumeFileValue_NoFileFieldReturnsFalse(t *testing.T) {
	plan := Plan{Fields: []ResolvedField{{ID: "email", Kind: "text", Value: "ada@example.com"}}}
	if _, ok := resumeFileValue(plan); ok {
		t.Error("resumeFileValue ok = true, want false — no file field in this plan")
	}
}

func TestResumeFileValue_ReturnsTheFileFieldsValue(t *testing.T) {
	plan := Plan{Fields: []ResolvedField{
		{ID: "email", Kind: "text", Value: "ada@example.com"},
		{ID: "resume", Kind: "file", Value: "/tmp/rendered.pdf"},
	}}
	path, ok := resumeFileValue(plan)
	if !ok || path != "/tmp/rendered.pdf" {
		t.Errorf("resumeFileValue = (%q, %v), want (/tmp/rendered.pdf, true)", path, ok)
	}
}

func TestAttachResumeIfPresent_NoFileFieldIsANoOp(t *testing.T) {
	e := newTestBrowserUseExecutor("http://unused.example.test")
	plan := Plan{Fields: []ResolvedField{{ID: "email", Kind: "text", Value: "ada@example.com"}}}

	opts, cleanup, err := e.attachResumeIfPresent(context.Background(), plan)
	if err != nil || cleanup != nil || opts.WorkspaceID != "" || len(opts.AttachedFileIDs) != 0 {
		t.Fatalf("attachResumeIfPresent = (%+v, cleanup!=nil:%v, %v), want zero value, nil, nil", opts, cleanup != nil, err)
	}
}

func TestAttachResumeIfPresent_UnreadableFileIsAPlainError(t *testing.T) {
	e := newTestBrowserUseExecutor("http://unused.example.test")
	plan := Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file", Value: "/no/such/file.pdf"}}}

	if _, _, err := e.attachResumeIfPresent(context.Background(), plan); err == nil {
		t.Fatal("want an error for a résumé path that cannot be read")
	}
}

func TestAttachResumeIfPresent_UploadFailureCleansUpTheWorkspace(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "resume-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString("%PDF-1.4 fake"); err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()

	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workspaces":
			_, _ = w.Write([]byte(`{"id":"ws-1","archived":false,"createdAt":"2026-09-07T00:00:00Z","updatedAt":"2026-09-07T00:00:00Z"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/workspaces/ws-1":
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/workspaces/ws-1/files/upload":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	e := newTestBrowserUseExecutor(srv.URL)
	plan := Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file", Value: tmp.Name()}}}

	if _, _, err := e.attachResumeIfPresent(context.Background(), plan); err == nil {
		t.Fatal("want an error when the upload-reservation request fails")
	}
	if !deleted {
		t.Error("workspace was never deleted after the upload failed — a résumé must not be left behind")
	}
}

func TestParseOutcome_Confirmed(t *testing.T) {
	report := "I filled everything.\nCONFIRMED: Application received!"
	outcome, detail := parseOutcome(report)
	if outcome != outcomeConfirmed || detail != "Application received!" {
		t.Errorf("outcome=%v detail=%q, want confirmed/\"Application received!\"", outcome, detail)
	}
}

func TestParseOutcome_Unconfirmed(t *testing.T) {
	report := "I clicked submit but nothing happened.\nUNCONFIRMED"
	outcome, _ := parseOutcome(report)
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed", outcome)
	}
}

func TestParseOutcome_Parked(t *testing.T) {
	report := "The relocation question was required and unanswered.\nPARKED: relocation question unanswered"
	outcome, detail := parseOutcome(report)
	if outcome != outcomeParked || detail != "relocation question unanswered" {
		t.Errorf("outcome=%v detail=%q, want parked/\"relocation question unanswered\"", outcome, detail)
	}
}

func TestParseOutcome_NoMarkerDefaultsToUnconfirmed(t *testing.T) {
	report := "I think it probably worked out fine in the end."
	outcome, _ := parseOutcome(report)
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed for a report with no explicit marker", outcome)
	}
}

func TestParseOutcome_EmptyReportDefaultsToUnconfirmed(t *testing.T) {
	outcome, _ := parseOutcome("")
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed for an empty report", outcome)
	}
}

func TestParseOutcome_MarkerNotOnTheLastLineDoesNotCount(t *testing.T) {
	// The instruction asks for the marker as the LAST line — one buried earlier followed
	// by more narrative must not be picked up as confirmation.
	report := "CONFIRMED: looked right at the time\nActually wait, I'm not sure that went through."
	outcome, _ := parseOutcome(report)
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed when the marker isn't the last line", outcome)
	}
}

func TestBrowserUseEligible(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		plan     Plan
		want     bool
	}{
		{"ashby with no file field", "ashby", Plan{Fields: []ResolvedField{{ID: "email", Kind: "text"}}}, true},
		{"workable with no file field", "workable", Plan{}, true},
		{"greenhouse is never eligible", "greenhouse", Plan{}, false},
		// Lever moved here on 2026-09-10: it has a working fill path that loses the
		// invisible-hCaptcha coin toss seven attempts in eight, and the cloud browser
		// solves supported captchas itself. See browserUseProviders.
		{"lever, whose own fill path the captcha mostly blocks", "lever", Plan{}, true},
		{"unknown provider is never eligible", "smartrecruiters", Plan{}, false},
		// Recruitee has no registered applyform.Fetcher at all (found during
		// implementation — see browserUseProviders' own doc comment), so Client.Submit
		// never even reaches browserUseEligible for it; excluded here for the same reason.
		{"recruitee is never eligible", "recruitee", Plan{}, false},
		{"a resume file field no longer disqualifies the plan", "ashby", Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file"}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := browserUseEligible(tc.provider, tc.plan); got != tc.want {
				t.Errorf("browserUseEligible(%q, ...) = %v, want %v", tc.provider, got, tc.want)
			}
		})
	}
}

func TestRunSpendGuard_AllowsUntilLimitThenShadowLogsInsteadOfRefusing(t *testing.T) {
	g := &runSpendGuard{limitUSD: 1.0}
	g.record(0.5)
	if !g.allow(false) {
		t.Fatal("want allowed below the limit")
	}
	g.record(0.6) // now over the 1.0 limit
	if !g.allow(false) {
		t.Error("want allowed in shadow mode even over the limit — it only logs")
	}
	if g.allow(true) {
		t.Error("want refused once enforced and over the limit")
	}
}

func TestRunSpendGuard_UnlimitedWhenZero(t *testing.T) {
	g := &runSpendGuard{limitUSD: 0}
	g.record(1000)
	if !g.allow(true) {
		t.Error("want a zero limit to mean unlimited, even enforced")
	}
}
