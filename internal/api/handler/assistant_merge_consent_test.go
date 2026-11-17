package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

func TestExperienceMergeWithoutConsentDoesNotDeleteAnAchievement(t *testing.T) {
	bank := newStubBank()
	a := bank.add(1, experience.Atom{Claim: "Taught undergraduate students"})
	b := bank.add(1, experience.Atom{Claim: "Trained undergraduate students"})
	reg, _ := experienceToolsFor(t, bank)
	raw, _ := json.Marshal(map[string]any{"ids": []string{a.ID.String(), b.ID.String()}})
	result := reg.Call(context.Background(), 1, "experience_merge", raw)
	if result.Failed {
		t.Fatalf("preview: %s", result.Content)
	}
	if len(bank.atoms) != 2 {
		t.Fatal("merge deleted an achievement without candidate consent")
	}
	var preview struct {
		Status string `json:"status"`
		Claim  string `json:"claim"`
	}
	if err := json.Unmarshal([]byte(result.Content), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Status != "awaiting_candidate_response" || preview.Claim == "" {
		t.Fatalf("missing confirmation preview: %s", result.Content)
	}
}

func TestCVEditExplainsUnchangedAndEmptyEdits(t *testing.T) {
	bank := newStubBank()
	evidence := bank.add(3, experience.Atom{Claim: "Backend engineer", Provenance: experience.ProvenanceManual})
	h, _ := cvToolsAPIWithBank(t, oneExperienceCV, bank)
	tool := h.cvEditTool(testCVID, uuid.New())
	args := json.RawMessage(fmt.Sprintf(`{"ops":[{"kind":"set","path":"summary","value":"Backend engineer","evidence_id":%q}]}`, evidence.ID.String()))
	if _, err := tool.Run(context.Background(), 3, args); err != nil {
		t.Fatal(err)
	}
	result, err := tool.Run(context.Background(), 3, args)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	var receipt struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(raw, &receipt)
	if receipt.Status != "unchanged" {
		t.Fatalf("receipt = %s, want unchanged", raw)
	}
	_, err = tool.Run(context.Background(), 3, json.RawMessage(`{"note":"No evidence; leave this open"}`))
	if err == nil || !strings.Contains(err.Error(), "tailor_report") {
		t.Fatalf("error = %v, want guidance to record gaps without editing", err)
	}
}
