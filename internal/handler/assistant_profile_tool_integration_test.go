//go:build integration

// Integration test for the get_profile tool against a real Postgres: a whole turn in
// which the model calls the tool, and the transcript that turn leaves behind.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/userprofile"
)

// callReplyChoice is a model reply that calls one tool. (runner_test.go has the same
// shape, but in package assistant — not reachable from here.)
func callReplyChoice(name, args string) *llms.ContentChoice {
	return &llms.ContentChoice{ToolCalls: []llms.ToolCall{{
		ID: "call_" + name, Type: "function",
		FunctionCall: &llms.FunctionCall{Name: name, Arguments: args},
	}}}
}

// seedProfileAndCV gives a user the two things get_profile reads: a saved profile and
// a structured résumé current with their stored CV. The structure carries contacts,
// because the point of the test is that they do not come back out.
func seedProfileAndCV(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_profiles (user_id, specializations, skills, excluded_skills)
		 VALUES ($1, ARRAY['backend'], ARRAY['go','kubernetes'], ARRAY['php'])`, userID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	structured := `{
		"full_name":"Ada Lovelace","email":"ada@example.test","phone":"+351900000000",
		"links":["https://github.com/ada-example"],
		"headline":"Staff Backend Engineer","total_years":11,
		"skills":["Go","Kafka"],
		"experience":[{"title":"Staff Engineer","company":"Analytical Engines","stack":["Go"]}]
	}`
	// The structure serves only while its stamp equals the résumé upload time, so both
	// columns take the same instant — passed in, not `now()`, because within one UPDATE
	// the right-hand side of an assignment reads the column's OLD value.
	uploadedAt := time.Now().UTC()
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'cv/ada.pdf', resume_uploaded_at = $3,
		        resume_structured = $2::jsonb, resume_structured_uploaded_at = $3,
		        resume_structured_model = 'test-model'
		 WHERE id = $1`, userID, structured, uploadedAt); err != nil {
		t.Fatalf("seed structured résumé: %v", err)
	}
}

// newProfileAssistantApp is newAssistantApp with the profile handlers wired, which is
// what get_profile is built on.
func newProfileAssistantApp(pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model) *fiber.App {
	queries := db.New(pool)
	profileH := newProfileHandlers(
		userprofile.New(userprofile.NewQueriesRepository(queries)),
		resume.New(nil, resume.NewQueriesRepository(queries)),
	)
	h := &assistantHandlers{store: assistant.NewStore(queries), queries: queries, profile: profileH}
	h.runner = assistant.NewRunner(model, h.store, assistant.RunnerConfig{MaxSteps: 3})

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(iss, testVersions)})
	return app
}

// TestGetProfileToolGroundsATurnWithoutLeakingContacts drives the whole chain the
// feature rests on: the model calls get_profile, the tool reads real rows, and the
// result is written into the stored transcript.
//
// The transcript is the reason this is worth an integration test. It is replayed into
// the model's context on every later turn, so a contact that reaches it is not a
// one-off disclosure — it is a permanent passenger in the conversation.
func TestGetProfileToolGroundsATurnWithoutLeakingContacts(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{
		callReplyChoice("get_profile", `{}`),
		{Content: "You are a backend engineer working in Go — searching on that."},
	}}
	app := newProfileAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "grounded@example.test", true)
	seedProfileAndCV(t, pool, userID)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "help me find work"})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("turn: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	assertGroundedWithoutContacts(t, "stream", string(body))

	// And again from the stored transcript, which is what later turns actually read.
	resp = assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions/"+id, cookie, nil)
	transcript, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	assertGroundedWithoutContacts(t, "transcript", string(transcript))
}

// assertGroundedWithoutContacts checks one rendering of the turn: the profile the
// agent was grounded in is present, and none of the four contact fields are.
func assertGroundedWithoutContacts(t *testing.T, where, payload string) {
	t.Helper()
	for _, want := range []string{"backend", "kubernetes", "Staff Backend Engineer"} {
		if !strings.Contains(payload, want) {
			t.Errorf("%s does not carry %q from the profile:\n%s", where, want, payload)
		}
	}
	for _, leaked := range []string{"Ada Lovelace", "ada@example.test", "+351900000000", "github.com/ada-example"} {
		if strings.Contains(payload, leaked) {
			t.Errorf("%s leaks the contact %q — it would be replayed into the model's context on every later turn:\n%s", where, leaked, payload)
		}
	}
}

// TestGetProfileToolSendsAProfilelessUserToTheProfilePage covers the other half: a
// user who never saved a profile is directed to the page that persists one, rather
// than interviewed in a conversation whose answers evaporate.
func TestGetProfileToolSendsAProfilelessUserToTheProfilePage(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	userID, _ := assistantUser(t, pool, iss, "blank@example.test", true)

	h := &assistantHandlers{profile: newProfileHandlers(
		userprofile.New(userprofile.NewQueriesRepository(queries)),
		resume.New(nil, resume.NewQueriesRepository(queries)),
	)}

	out, err := toolByName(t, h.assistantDiscoveryTools(), "get_profile").
		Run(context.Background(), userID, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("a missing profile is an answer, not a tool failure: %v", err)
	}
	payload, _ := json.Marshal(out)
	if !strings.Contains(string(payload), "/my/profile") {
		t.Errorf("result should point the agent at the profile page:\n%s", payload)
	}
}
