package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/screeninganswers"
)

// screeningToolsFor builds the screening-answers tool over the given fake repo, and
// returns the registry so tool failures can be observed the way the model sees them.
func screeningToolsFor(repo *fakeScreeningAnswersRepo) *assistant.Registry {
	h := &assistantHandlers{screeningAnswers: screeninganswers.New(repo)}
	return assistant.NewRegistry(h.assistantScreeningTools()...)
}

func TestScreeningAnswersSetWritesOnlyWhatItIsGiven(t *testing.T) {
	repo := &fakeScreeningAnswersRepo{
		getErr:    screeninganswers.ErrNotFound,
		upsertRet: screeninganswers.Answers{},
	}
	reg := screeningToolsFor(repo)

	out := reg.Call(context.Background(), 1, "screening_answers_set", json.RawMessage(`{"notice_period_days":30}`))
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	if decoded.Error != "" {
		t.Fatalf("call failed: %s", decoded.Error)
	}
	if !repo.upsertCalled {
		t.Fatal("repo.Upsert was not called")
	}
	if repo.upserted.NoticePeriodDays == nil || *repo.upserted.NoticePeriodDays != 30 {
		t.Errorf("upserted NoticePeriodDays = %v, want 30", repo.upserted.NoticePeriodDays)
	}
	if repo.upserted.WillingToRelocate != nil {
		t.Errorf("upserted WillingToRelocate = %v, want nil (not stated)", repo.upserted.WillingToRelocate)
	}
}

func TestScreeningAnswersSetReportsAnInvalidFieldToTheModel(t *testing.T) {
	repo := &fakeScreeningAnswersRepo{getErr: screeninganswers.ErrNotFound}
	reg := screeningToolsFor(repo)

	out := reg.Call(context.Background(), 1, "screening_answers_set", json.RawMessage(`{"desired_salary_currency":"dollars"}`))
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	if decoded.Error == "" {
		t.Fatal("call succeeded, want an error result naming the bad currency")
	}
	if !strings.Contains(decoded.Error, "desired_salary_currency") {
		t.Errorf("error %q does not name the invalid field", decoded.Error)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called on invalid input")
	}
}
