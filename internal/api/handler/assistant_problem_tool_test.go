package handler

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/problemreport"
)

func TestProblemToolFailsClosedAndIsOAuthOnly(t *testing.T) {
	h := &assistantHandlers{}
	tool := h.problemMCPTools()[0]
	if tool.Name != "report_problem" || tool.Effect != assistant.EffectAdds || tool.Idempotent {
		t.Fatalf("incorrect effect metadata: %+v", tool)
	}
	for _, candidate := range mcpKeyTools(h, testMCPOrigin) {
		if candidate.Name == tool.Name {
			t.Fatal("report_problem exposed to API keys")
		}
	}
	for _, args := range []string{
		`{"category":"other","goal":"test","expected":"success","actual":"failure"}`,
		`{"category":"other","goal":"test","expected":"success","actual":"failure","user_id":99}`,
	} {
		if result, err := tool.Run(t.Context(), 42, json.RawMessage(args)); err == nil || result != nil {
			t.Fatalf("unavailable tool claimed success: %+v %v", result, err)
		}
	}
	h.problems = problemreport.New(nil, nil)
	if _, err := tool.Run(t.Context(), 0, json.RawMessage(`{"category":"other","goal":"test","expected":"success","actual":"failure"}`)); err == nil {
		t.Fatal("tool accepted missing identity")
	}
}
