package handler

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/application/autoapply"
)

// The apply conversation is about one attempt that already exists: it reads, answers,
// retries, reviews and cancels that attempt, and starts or lists nothing. It is not a
// rehearsal, so it does not inherit the interview's context tool by binding to a vacancy.
func TestApplyPresetCarriesTheAttemptsToolsAndNothingElseOfItsKind(t *testing.T) {
	jobID := int64(9)
	names := presetAPI().registry(
		assistant.Session{UserID: 3, Preset: assistant.PresetApply, JobID: &jobID}, uuid.New()).Names()

	for _, want := range []string{"auto_apply_get", "auto_apply_review", "auto_apply_retry", "auto_apply_cancel",
		"answer_bank_save", "answer_bank_list", "screening_answers_get", "screening_answers_set"} {
		if !slices.Contains(names, want) {
			t.Errorf("the apply preset does not register %q: %v", want, names)
		}
	}
	for _, forbidden := range []string{"auto_apply_start", "auto_apply_list", "interview_context", "cv_edit"} {
		if slices.Contains(names, forbidden) {
			t.Errorf("the apply preset offers %q, which belongs to another kind of session", forbidden)
		}
	}
}

// Every other preset keeps what it had: the auto-apply tools are this conversation's and the
// MCP server's, and nobody else's.
func TestOtherPresetsDoNotGainTheAutoApplyTools(t *testing.T) {
	for _, preset := range []string{assistant.PresetChat, assistant.PresetProfile, assistant.PresetInterview} {
		jobID := int64(9)
		names := presetAPI().registry(assistant.Session{UserID: 3, Preset: preset, JobID: &jobID}, uuid.New()).Names()
		for _, name := range names {
			if slices.Contains([]string{"auto_apply_get", "auto_apply_review", "auto_apply_retry", "auto_apply_cancel", "answer_bank_save"}, name) {
				t.Errorf("the %q preset offers %q", preset, name)
			}
		}
	}
}

// The session names the attempt; the model does not. A tool that took a slug would let the
// model act on any application the candidate holds from inside this one's chat.
func TestApplyToolsTakeNoJobFromTheModel(t *testing.T) {
	jobID := int64(9)
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetApply, JobID: &jobID}, uuid.New())
	for _, def := range reg.Definitions() {
		if def.Function == nil {
			continue
		}
		raw, _ := json.Marshal(def.Function.Parameters)
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		_ = json.Unmarshal(raw, &schema)
		if _, ok := schema.Properties["job_slug"]; ok {
			t.Errorf("%s asks the model for a job_slug", def.Function.Name)
		}
	}
}

// The binding injects the session's job over anything the model sends.
func TestBindToJobOverridesTheModelsSlug(t *testing.T) {
	var got string
	tool := assistant.Tool{Name: "t", Schema: jobSlugSchema, Run: func(_ context.Context, _ int64, raw json.RawMessage) (any, error) {
		var in jobSlugArgs
		_ = json.Unmarshal(raw, &in)
		got = in.JobSlug
		return nil, nil
	}}
	bound := bindToJob(tool, func(context.Context) (string, error) { return "the-session-job", nil })

	if _, err := bound.Run(context.Background(), 3, json.RawMessage(`{"job_slug":"someone-elses-job"}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "the-session-job" {
		t.Errorf("the tool ran on %q, want the session's job", got)
	}
}

// An agent asked to relay fourteen questions one at a time gets one answer and a tired
// person. One numbered list is what lets the person reply "1 …, 3 …" — and the form is in
// the employer's language whatever language the person answers in.
func TestBlockedNextStepAsksForOneNumberedList(t *testing.T) {
	step := autoApplyNextStep(string(autoapply.StatusBlocked), false)
	for _, want := range []string{"one numbered list", "language of the application form"} {
		if !strings.Contains(step, want) {
			t.Errorf("blocked next_step = %q, missing %q", step, want)
		}
	}
}

// A tool with no arguments left is often called with "null"; that must not crash the turn.
func TestBindToJobAcceptsNullArguments(t *testing.T) {
	var got string
	tool := assistant.Tool{Name: "t", Schema: jobSlugSchema, Run: func(_ context.Context, _ int64, raw json.RawMessage) (any, error) {
		var in jobSlugArgs
		_ = json.Unmarshal(raw, &in)
		got = in.JobSlug
		return nil, nil
	}}
	bound := bindToJob(tool, func(context.Context) (string, error) { return "the-session-job", nil })

	if _, err := bound.Run(context.Background(), 3, json.RawMessage(`null`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "the-session-job" {
		t.Errorf("the tool ran on %q", got)
	}
}

// Binding a tool to the session's job does not lift agents_may_apply: an API-key caller in an
// apply session is an agent outside the browser, and the switch must hold for it. Only a turn
// the person sent from the website, marked by streamSSE from the session cookie, lifts it.
func TestTheSwitchIsLiftedByTheBrowserNotByTheBinding(t *testing.T) {
	var waived bool
	tool := assistant.Tool{Name: "t", Schema: jobSlugSchema, Run: func(ctx context.Context, _ int64, _ json.RawMessage) (any, error) {
		waived = agentsMayApplyWaived(ctx)
		return nil, nil
	}}
	bound := bindToJob(tool, func(context.Context) (string, error) { return "job", nil })

	_, _ = bound.Run(context.Background(), 3, json.RawMessage(`{}`))
	if waived {
		t.Error("a bound tool lifted agents_may_apply on its own, so an API key would bypass it")
	}
	_, _ = bound.Run(withPersonInBrowser(context.Background()), 3, json.RawMessage(`{}`))
	if !waived {
		t.Error("a turn from the person's own browser still meets agents_may_apply")
	}
}
