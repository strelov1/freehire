package handler

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/resume"
)

// everythingSet is an account that has done every step.
func everythingSet() setupFacts {
	return setupFacts{
		cvStorage: true, cvPresent: true, cvReading: resume.ExtractStatusOK,
		specializations: true, skills: true, seniorities: true, workModes: true,
		alertsKnown: true, activeAlerts: 1,
		tierKnown: true, tier: plan.TierPro,
		agentsKnown: true, agentsMayApply: true,
	}
}

func stepNamed(t *testing.T, s setupStatus, name string) setupStep {
	t.Helper()
	for _, st := range s.Steps {
		if st.Name == name {
			return st
		}
	}
	t.Fatalf("no step %q in %+v", name, s.Steps)
	return setupStep{}
}

// next_step is the first core step not done, in the order a person gets value from them: a
// CV, then the preferences it suggests, then an alert.
func TestSetupStatus_NextStepIsTheFirstNotDone(t *testing.T) {
	cases := []struct {
		name     string
		edit     func(*setupFacts)
		wantStep string
		wantTool string
		wantURL  string
	}{
		{"no CV", func(f *setupFacts) { *f = setupFacts{cvStorage: true} }, "cv", "add_cv", "/my/profile"},
		{"no CV, storage off", func(f *setupFacts) { *f = setupFacts{} }, "cv", "", "/my/profile"},
		{"CV unreadable", func(f *setupFacts) { f.cvReading = resume.ExtractStatusFailed }, "cv", "", "/my/profile"},
		{"no specializations", func(f *setupFacts) { f.specializations = false }, "preferences", "update_profile", ""},
		{"no skills", func(f *setupFacts) { f.skills = false }, "preferences", "update_profile", ""},
		{"no active alert", func(f *setupFacts) { f.activeAlerts = 0 }, "alerts", "subscribe", ""},
		// The optional auto-apply steps are never the next step: once the core is done, search.
		{"free plan", func(f *setupFacts) { f.tier = plan.TierFree }, "done", "search_jobs", ""},
		{"agents may not apply", func(f *setupFacts) { f.agentsMayApply = false }, "done", "search_jobs", ""},
		{"done", func(*setupFacts) {}, "done", "search_jobs", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := everythingSet()
			tc.edit(&f)
			got := buildSetupStatus(f, testMCPOrigin).NextStep
			if got.Step != tc.wantStep || got.Tool != tc.wantTool {
				t.Errorf("next_step = %+v, want step %q tool %q", got, tc.wantStep, tc.wantTool)
			}
			if tc.wantURL == "" && got.URL != "" || tc.wantURL != "" && got.URL != testMCPOrigin+tc.wantURL {
				t.Errorf("next_step url = %q, want %q", got.URL, tc.wantURL)
			}
			if got.Do == "" {
				t.Error("next_step says nothing to do")
			}
		})
	}
}

// A CV still being read is reported but does not hold the person up: the suggestions they
// confirm come from the CV text at once.
func TestSetupStatus_ReadingCVDoesNotBlock(t *testing.T) {
	f := everythingSet()
	f.cvReading, f.specializations = resume.ExtractStatusPending, false
	s := buildSetupStatus(f, testMCPOrigin)
	if cv := stepNamed(t, s, "cv"); !cv.Done || cv.Detail != "reading" {
		t.Errorf("cv step = %+v, want done and reading", cv)
	}
	if s.NextStep.Step != "preferences" {
		t.Errorf("next_step = %+v, want preferences", s.NextStep)
	}
}

// complete means the core setup — CV, preferences, an alert — is done; auto-apply is optional.
func TestSetupStatus_CompleteIgnoresOptionalSteps(t *testing.T) {
	f := everythingSet()
	f.tier, f.agentsMayApply = plan.TierFree, false
	s := buildSetupStatus(f, testMCPOrigin)
	if !s.Complete {
		t.Error("complete = false with only optional steps left")
	}
	if paid := stepNamed(t, s, "paid_plan"); !paid.Optional || paid.Done {
		t.Errorf("paid_plan = %+v, want visible as optional and not done", paid)
	}
	if !stepNamed(t, s, "agents_may_apply").Optional {
		t.Error("agents_may_apply is not marked optional")
	}
	if s.NextStep.Tool != "search_jobs" {
		t.Errorf("next_step = %+v, want search_jobs once the core is done", s.NextStep)
	}
	f.activeAlerts = 0
	if buildSetupStatus(f, testMCPOrigin).Complete {
		t.Error("complete = true with no alert")
	}
}

// The preferences step names what is missing, so the agent asks about those and no more.
func TestSetupStatus_PreferencesNameTheGaps(t *testing.T) {
	f := everythingSet()
	f.skills, f.workModes = false, false
	got := stepNamed(t, buildSetupStatus(f, testMCPOrigin), "preferences").Missing
	if !slices.Equal(got, []string{"skills", "work_modes"}) {
		t.Errorf("missing = %v, want [skills work_modes]", got)
	}
}

// A dependency the deployment does not wire leaves its step out rather than reporting a
// state nobody read.
func TestSetupStatus_UnknownStepsAreLeftOut(t *testing.T) {
	f := everythingSet()
	f.alertsKnown, f.tierKnown, f.agentsKnown = false, false, false
	s := buildSetupStatus(f, testMCPOrigin)
	for _, st := range s.Steps {
		if st.Name == "alerts" || st.Name == "paid_plan" || st.Name == "agents_may_apply" {
			t.Errorf("unread step %q reported", st.Name)
		}
	}
}

// Over the tool: an account with nothing is sent to add a CV.
func TestSetupStatusTool_NewAccount(t *testing.T) {
	h := addCVHandlers(newFakeResumeBlobs(), &fakeResumeRepo{})
	h.profile = assistantWithProfile(&fakeProfileRepo{}, nil).profile
	out, err := h.setupStatusTool(testMCPOrigin).Run(context.Background(), 1, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_setup_status: %v", err)
	}
	blob, _ := json.Marshal(out)
	if !strings.Contains(string(blob), `"tool":"add_cv"`) {
		t.Errorf("result = %s, want next_step naming add_cv", blob)
	}
}
