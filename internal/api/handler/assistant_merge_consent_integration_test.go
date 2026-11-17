//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/tmc/langchaingo/llms"
)

func TestMergeRequiresApprovalInItsOwnSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	_, h := newAssistantApp(pool, iss, nil)
	owner, _ := assistantUser(t, pool, iss, "merge-consent@example.test", true)
	other, _ := assistantUser(t, pool, iss, "merge-foreign@example.test", true)
	ctx := context.Background()
	sess, err := h.store.CreateSession(ctx, owner, assistant.PresetProfile, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := h.experience.AddAtom(ctx, owner, experience.Atom{Claim: "Taught undergraduate students"}, experience.AuthorCandidate)
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.experience.AddAtom(ctx, owner, experience.Atom{Claim: "Trained undergraduate students"}, experience.AuthorCandidate)
	if err != nil {
		t.Fatal(err)
	}
	tool := h.experienceMergeTool(sess.ID)
	args, _ := json.Marshal(map[string]any{"ids": []uuid.UUID{a.ID, b.ID}})
	initial, err := tool.Run(ctx, owner, args)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(initial)
	var preview struct {
		Claim string `json:"claim"`
	}
	if err := json.Unmarshal(raw, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Claim == "" {
		t.Fatal("merge did not offer a confirmation")
	}
	appendMessage := func(m assistant.Message) {
		t.Helper()
		if _, err := h.store.Append(ctx, sess.ID, m); err != nil {
			t.Fatal(err)
		}
	}
	question, _ := json.Marshal(map[string]string{"claim": preview.Claim, "question": "Merge these?"})
	ask, _ := assistant.EncodeAssistant("", []llms.ToolCall{{FunctionCall: &llms.FunctionCall{Name: "request_confirmation", Arguments: string(question)}}})
	appendMessage(ask)
	unrelated, _ := assistant.EncodeUser("Where does my CV fit most?")
	appendMessage(unrelated)
	if _, err := tool.Run(ctx, owner, args); err != nil {
		t.Fatal(err)
	}
	if _, err := h.experience.GetAtom(ctx, b.ID, owner); err != nil {
		t.Fatal("topic change authorized a merge")
	}
	appendMessage(ask)
	yes, _ := assistant.EncodeUser("yes")
	appendMessage(yes)
	if _, err := tool.Run(ctx, other, args); err == nil {
		t.Fatal("foreign caller reached the merge")
	}
	if _, err := tool.Run(ctx, owner, args); err != nil {
		t.Fatal(err)
	}
	atoms, err := h.experience.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(atoms) != 1 {
		t.Fatalf("approved merge kept %d atoms, want 1", len(atoms))
	}
}
