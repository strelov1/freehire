//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
)

type interviewFeedbackModel struct {
	llms.Model
	calls int
}

func (m *interviewFeedbackModel) GenerateContent(
	_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption,
) (*llms.ContentResponse, error) {
	m.calls++
	if !strings.Contains(messages[1].Parts[0].(llms.TextContent).Text, "I measured p95 latency") {
		return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: "{}"}}}, nil
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{
		Content: `{"strength":"You identified a metric.","improvement":"Say what p95 changed by.","quote":"invented","question":"invented"}`,
	}}}, nil
}

func TestInterviewReportPersistsAndRepeatsOriginalQuestion(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	issuer := auth.NewIssuer("interview-test-secret", time.Hour)
	app, h := newAssistantApp(pool, issuer, nil)
	model := &interviewFeedbackModel{}
	h.llm = llm.NewWithModel(model)

	userID, cookie := assistantUser(t, pool, issuer, "report-owner@example.test", true)
	_, stranger := assistantUser(t, pool, issuer, "report-stranger@example.test", true)
	job := seedApplication(t, pool, userID, "interview-report", "interview")
	session, err := h.store.CreateSession(ctx, userID, assistant.PresetInterview, nil, &job)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/assistant/sessions/" + session.ID.String()
	appendMessage := func(message assistant.Message, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.store.Append(ctx, session.ID, message); err != nil {
			t.Fatal(err)
		}
	}
	const question = "How did you measure the result?"
	const answer = "I measured p95 latency."
	appendMessage(assistant.EncodeUser("Start the rehearsal."))
	appendMessage(assistant.EncodeAssistant(question, nil))

	request := func(method, path, token string, status int) []byte {
		t.Helper()
		response := assistantRequest(t, app, method, path, token, nil)
		defer response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("%s %s: got %d, want %d", method, path, response.StatusCode, status)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	request(http.MethodPost, base+"/interview-report", stranger, http.StatusNotFound)
	request(http.MethodPost, base+"/interview-report", cookie, http.StatusConflict)

	appendMessage(assistant.EncodeUser(answer))
	appendMessage(assistant.EncodeAssistant("What would you do differently?", nil))
	for range 2 {
		response := request(http.MethodPost, base+"/interview-report", cookie, http.StatusOK)
		report := decodeReportResponse[apigen.PostAssistantInterviewReportOK](t, response)
		if report.Data.Question != question || report.Data.Quote != answer || report.Data.Strength == "" {
			t.Fatalf("unexpected report: %+v", report.Data)
		}
	}
	if model.calls != 1 {
		t.Fatalf("generated feedback %d times", model.calls)
	}
	detail := decodeReportResponse[apigen.GetAssistantSessionOK](t, request(http.MethodGet, base, cookie, http.StatusOK))
	if detail.Data.InterviewReport.Value.Quote != answer {
		t.Fatal("saved report was missing from session detail")
	}

	request(http.MethodPost, base+"/interview-repeat", stranger, http.StatusNotFound)
	repeat := decodeReportResponse[apigen.PostAssistantInterviewRepeatOK](t, request(http.MethodPost, base+"/interview-repeat", cookie, http.StatusOK))
	if repeat.Data.SessionID == session.ID.String() {
		t.Fatal("repeat reused the finished session")
	}
	newDetail := decodeReportResponse[apigen.GetAssistantSessionOK](t, request(http.MethodGet, "/api/v1/assistant/sessions/"+repeat.Data.SessionID, cookie, http.StatusOK))
	var transcript []assistant.Message
	if err := json.Unmarshal(newDetail.Data.Messages, &transcript); err != nil || len(transcript) != 1 {
		t.Fatalf("unexpected repeat transcript: %s (%v)", newDetail.Data.Messages, err)
	}
	repeated, err := transcript[0].Decode()
	if err != nil || transcript[0].Role != assistant.RoleAssistant || repeated.Parts[0] != (llms.TextContent{Text: question}) {
		t.Fatalf("repeat changed the question: %s (%v)", transcript[0].Content, err)
	}
}

func decodeReportResponse[T any](t *testing.T, body []byte) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
