package atsapply

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/platform/llm"
)

func stubModel(t *testing.T, responseJSON string) *llm.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, responseJSON)
	}))
	t.Cleanup(srv.Close)
	c, err := llm.New(srv.URL, "sk-test", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return c
}

func TestLLMDrafter_ReturnsTheModelsGroundedAnswer(t *testing.T) {
	client := stubModel(t, `{"answer":"I found this role on the freehire job board.","grounded":true}`)
	d := NewLLMDrafter(client)

	answer, ok, err := d.Draft(context.Background(),
		MergedField{ID: "q1", Label: "Where did you hear about us?", Required: true, Kind: "text"},
		GroundingContext{})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !ok || answer != "I found this role on the freehire job board." {
		t.Errorf("got (%q, %v), want the model's grounded answer", answer, ok)
	}
}

func TestLLMDrafter_UngroundedResponseIsNotAnAnswer(t *testing.T) {
	client := stubModel(t, `{"answer":"","grounded":false}`)
	d := NewLLMDrafter(client)

	_, ok, err := d.Draft(context.Background(), MergedField{ID: "q1", Label: "Describe a niche hobby", Required: true, Kind: "text"}, GroundingContext{})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if ok {
		t.Error("ok = true, want false — the model reported no grounded basis")
	}
}

// Grounded=true with a blank answer is treated the same as ungrounded — an empty string
// must never be mistaken for a real, if terse, answer.
func TestLLMDrafter_GroundedTrueButBlankAnswerIsStillNotAnAnswer(t *testing.T) {
	client := stubModel(t, `{"answer":"   ","grounded":true}`)
	d := NewLLMDrafter(client)

	_, ok, err := d.Draft(context.Background(), MergedField{ID: "q1", Label: "x", Required: true, Kind: "text"}, GroundingContext{})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if ok {
		t.Error("ok = true, want false for a blank answer despite grounded=true")
	}
}

func TestLLMDrafter_NilClientNeverCallsOutAndReportsNoDraft(t *testing.T) {
	d := NewLLMDrafter(nil)
	answer, ok, err := d.Draft(context.Background(), MergedField{ID: "q1", Label: "x", Required: true, Kind: "text"}, GroundingContext{})
	if err != nil || ok || answer != "" {
		t.Errorf("Draft(nil client) = (%q, %v, %v), want (\"\", false, nil)", answer, ok, err)
	}
}

func TestDraftUserPrompt_ListsOfferedOptionsAndPublishableAtoms(t *testing.T) {
	grounding := GroundingContext{Atoms: []experience.Atom{
		{ID: uuid.New(), Claim: "Led the payments migration", Context: "at Acme", Skills: []string{"go", "postgresql"}},
	}}
	question := MergedField{
		Label:   "Are you willing to relocate?",
		Options: []applyform.Option{{Label: "Yes", Value: "1"}, {Label: "No", Value: "0"}},
	}

	prompt := draftUserPrompt(question, grounding)
	if !strings.Contains(prompt, "Led the payments migration") {
		t.Errorf("prompt missing the atom's claim:\n%s", prompt)
	}
	if !strings.Contains(prompt, "at Acme") {
		t.Errorf("prompt missing the atom's context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "go, postgresql") {
		t.Errorf("prompt missing the atom's skills:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Yes") || !strings.Contains(prompt, "No") {
		t.Errorf("prompt missing the field's offered options:\n%s", prompt)
	}
}
