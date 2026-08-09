package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/experience"
)

// The tool's whole point: hand the agent back the atom's OWN words, so it can compare what
// it just wrote against what the evidence actually supports.
func TestEvidenceFidelityToolReturnsTheCitedAtomsOwnText(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(1, experience.Atom{
		Claim:   "Cut message-posting latency from 20s to 1s",
		Context: "Rewrote the outbound queue to batch writes instead of one call per message.",
		Metrics: []string{"20s -> 1s"},
	})
	h := &assistantHandlers{experience: bank}

	tool := toolByName(t, h.assistantCVTools(testCVID, 9, uuid.New()), "check_evidence_fidelity")
	out, err := tool.Run(context.Background(), 1,
		json.RawMessage(`{"evidence_id":"`+atom.ID.String()+`"}`))
	if err != nil {
		t.Fatalf("check_evidence_fidelity: %v", err)
	}

	payload, _ := json.Marshal(out)
	var got struct {
		Claim   string   `json:"claim"`
		Context string   `json:"context"`
		Metrics []string `json:"metrics"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v (payload=%s)", err, payload)
	}
	if got.Claim != atom.Claim {
		t.Errorf("claim = %q, want %q", got.Claim, atom.Claim)
	}
	if got.Context != atom.Context {
		t.Errorf("context = %q, want %q", got.Context, atom.Context)
	}
	if len(got.Metrics) != 1 || got.Metrics[0] != "20s -> 1s" {
		t.Errorf("metrics = %v, want [20s -> 1s]", got.Metrics)
	}
}

// A stray or foreign id must fail the same way an unresolved evidence_id fails everywhere
// else in this tool surface — named, so the model knows what to go check.
func TestEvidenceFidelityToolRefusesAnUnknownOrForeignId(t *testing.T) {
	bank := newStubBank()
	foreign := bank.add(2, experience.Atom{Claim: "Someone else's achievement"})
	h := &assistantHandlers{experience: bank}
	tool := toolByName(t, h.assistantCVTools(testCVID, 9, uuid.New()), "check_evidence_fidelity")
	ctx := context.Background()

	if _, err := tool.Run(ctx, 1, json.RawMessage(`{"evidence_id":"`+uuid.New().String()+`"}`)); err == nil {
		t.Error("an id matching no achievement was accepted")
	}
	if _, err := tool.Run(ctx, 1, json.RawMessage(`{"evidence_id":"not-a-uuid"}`)); err == nil {
		t.Error("a malformed id was accepted")
	}
	if _, err := tool.Run(ctx, 1, json.RawMessage(`{"evidence_id":"`+foreign.ID.String()+`"}`)); err == nil {
		t.Error("another user's evidence was accepted")
	}
}
