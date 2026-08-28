//go:build integration

// Integration tests for the in-app assistant's HTTP surface against a real
// Postgres: the session lifecycle (create, list, read, delete), the owner checks
// on every one of them, the boundary at authentication (any signed-in user is
// served, a caller with no credential is not), and a full streamed turn driven by
// a scripted model — its events, its persisted transcript, and its resumption.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// turnModel replays a fixed script of model replies, so a turn is deterministic.
type turnModel struct{ replies []*llms.ContentChoice }

func (m *turnModel) Chat(_ context.Context, _ []llms.MessageContent, _ []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	if len(m.replies) == 0 {
		return &llms.ContentChoice{Content: "done"}, nil
	}
	reply := m.replies[0]
	m.replies = m.replies[1:]
	if s.OnText != nil && reply.Content != "" {
		s.OnText(reply.Content)
	}
	return reply, nil
}

// newAssistantApp wires the assistant routes over a real database, with the given
// scripted model behind the turn endpoint.
// mws are mounted before the routes so a test can install the sentryfiber middleware the
// server runs in production; existing callers pass none and are unaffected.
func newAssistantApp(pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model, mws ...fiber.Handler) (*fiber.App, *assistantHandlers) {
	queries := db.New(pool)
	bank := experience.NewStore(experience.NewQueriesRepository(queries))
	h := &assistantHandlers{
		store: assistant.NewStore(queries), queries: queries,
		maxPrompt: defaultAssistantMaxPrompt,
		// A rehearsal resolves its application through the same store the production
		// wiring gives it; without this the ownership check reports the assistant
		// unavailable and the 404 under test would never be reached.
		stages: queries,
		// The evidence gate answers from the bank. Without it the run could write an
		// unevidenced claim and the test that says it cannot would pass for the wrong
		// reason — which is why the editor below is CONSTRUCTED with it, exactly as the
		// production assembly does, rather than having it attached afterwards.
		experience: bank,
		// The run's plan, the tailoring context and the rehearsal all read the cached fit
		// analysis and the vacancy. They take both directly, exactly as the production
		// wiring hands them over — not through whichever HTTP surface happens to hold one.
		fit:  fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		jobs: queries,
		// The tailoring tools and the autopilot run reach the CV store, so the assistant
		// under test carries the same CV service the HTTP surface uses.
		cv: &cvHandlers{
			cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
			editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: bank}), queries: queries, jobReader: queries,
			fit: fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		},
	}
	if model != nil {
		h.runner = assistant.NewRunner(model, h.store, assistant.RunnerConfig{MaxSteps: 3})
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	for _, mw := range mws {
		app.Use(mw)
	}
	api := app.Group("/api/v1")
	// Both gates are supplied so the test exercises whichever one `register`
	// mounts: the extension reaches the assistant with a Bearer credential, which
	// only `key` resolves.
	mw := middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}
	h.register(api, mw)
	// The CV routes ride along: the autopilot's undo and the CV read that carries a run's
	// report are the other half of the tailoring surface these tests exercise.
	h.cv.register(api, mw)
	return app, h
}

// assistantUser inserts a user and returns its id plus a session cookie.
func assistantUser(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, email string, beta bool) (int64, string) {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, beta_tester) VALUES ($1, $2) RETURNING id`, email, beta).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	token, err := iss.Issue(id, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return id, token
}

func assistantRequest(t *testing.T, app *fiber.App, method, path, cookie string, body any) *http.Response {
	t.Helper()
	return assistantDo(t, app, method, path, body, func(r *http.Request) {
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
	})
}

// assistantBearerRequest issues the same request as assistantRequest, but carries
// the credential as `Authorization: Bearer` and sends no cookie — what a browser
// extension does, hire's httpOnly cookie being invisible to it across origins.
func assistantBearerRequest(t *testing.T, app *fiber.App, method, path, token string, body any) *http.Response {
	t.Helper()
	return assistantDo(t, app, method, path, body, func(r *http.Request) {
		r.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	})
}

// assistantDo issues one JSON request, letting the caller attach whichever
// credential it is testing. The two carriers are the only difference between an
// extension's request and the web app's, so everything else lives here once.
func assistantDo(t *testing.T, app *fiber.App, method, path string, body any, credential func(*http.Request)) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		blob, _ := json.Marshal(body)
		reader = bytes.NewReader(blob)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Content-Type", "application/json")
	credential(r)
	// A streamed turn has no body length; give the test client room to read it all.
	resp, err := app.Test(r, 10_000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

// createSession creates a session and returns its id.
func createSession(t *testing.T, app *fiber.App, cookie string) string {
	t.Helper()
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions", cookie, map[string]any{})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create session: status %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.ID == "" {
		t.Fatal("create session returned no id")
	}
	return body.Data.ID
}

func TestAssistantSessionLifecycle(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, cookie := assistantUser(t, pool, iss, "beta@example.test", true)

	id := createSession(t, app, cookie)

	// The rail lists it.
	resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions", cookie, nil)
	var list struct {
		Data []struct {
			ID     string `json:"id"`
			Preset string `json:"preset"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Data) != 1 || list.Data[0].ID != id || list.Data[0].Preset != assistant.PresetChat {
		t.Fatalf("list = %+v, want the one chat session just created", list.Data)
	}

	// Deleting it removes it from the list and from reads.
	if resp := assistantRequest(t, app, fiber.MethodDelete, "/api/v1/assistant/sessions/"+id, cookie, nil); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete: status %d", resp.StatusCode)
	}
	if resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions/"+id, cookie, nil); resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("read after delete: status %d, want 404", resp.StatusCode)
	}
}

func TestAssistantSessionsAreOwnerScoped(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, owner := assistantUser(t, pool, iss, "owner@example.test", true)
	_, other := assistantUser(t, pool, iss, "other@example.test", true)

	id := createSession(t, app, owner)

	// Another beta user must not see, read, delete or post to it — and must not be
	// able to tell it exists.
	resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions", other, nil)
	var list struct {
		Data []json.RawMessage `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Data) != 0 {
		t.Errorf("another user's list contains %d sessions, want none", len(list.Data))
	}
	for _, tc := range []struct{ method, path string }{
		{fiber.MethodGet, "/api/v1/assistant/sessions/" + id},
		{fiber.MethodDelete, "/api/v1/assistant/sessions/" + id},
		{fiber.MethodPost, "/api/v1/assistant/sessions/" + id + "/messages"},
	} {
		resp := assistantRequest(t, app, tc.method, tc.path, other, map[string]string{"text": "hi"})
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("%s %s as a non-owner: status %d, want 404", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// The assistant applies no membership test beyond authentication: a signed-in user in
// no group at all is served. Authentication itself did not loosen with the gate — a
// caller presenting no credential is still refused, and this test guards both halves,
// because "open to everyone" is one edit away from "open to anyone".
func TestAssistantServesAUserInNoGroup(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, plain := assistantUser(t, pool, iss, "plain@example.test", false)

	if resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions", plain, map[string]any{}); resp.StatusCode != fiber.StatusCreated {
		t.Errorf("create as a user who is neither moderator nor beta tester: status %d, want 201", resp.StatusCode)
	}
	if resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions", "", nil); resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("unauthenticated list: status %d, want 401", resp.StatusCode)
	}
}

// A browser extension cannot send hire's httpOnly cookie across origins, so it
// presents the session JWT the connect flow minted as a Bearer credential. It must
// reach the same conversations, under the same gate, as the cookie would.
func TestAssistantServesABearerSessionJWT(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, token := assistantUser(t, pool, iss, "extension@example.test", true)

	id := createSession(t, app, token)

	resp := assistantBearerRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions", token, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("list with a Bearer session JWT: status %d, want 200", resp.StatusCode)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Data) != 1 || list.Data[0].ID != id {
		t.Errorf("Bearer list = %+v, want the caller's one session %s", list.Data, id)
	}
}

// The side panel holds browsing conversations, so it has to be able to ask for one.
// A tailoring session is not on offer here: it is bound to a CV and a vacancy, and
// one minted without that binding would register no CV tools at all.
func TestCreateAssistantSessionTakesThePresetTheClientAsksFor(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, cookie := assistantUser(t, pool, iss, "presets@example.test", true)

	// The preset rides the query string, as `?preset=profile` already does.
	preset := func(t *testing.T, query string) (int, string) {
		t.Helper()
		resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions"+query, cookie, map[string]any{})
		var out struct {
			Data struct {
				Preset string `json:"preset"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out.Data.Preset
	}

	if status, got := preset(t, "?preset=browse"); status != fiber.StatusCreated || got != assistant.PresetBrowse {
		t.Errorf("asking for browse: status %d preset %q, want 201 and %q", status, got, assistant.PresetBrowse)
	}
	if status, got := preset(t, ""); status != fiber.StatusCreated || got != assistant.PresetChat {
		t.Errorf("asking for nothing: status %d preset %q, want 201 and a chat", status, got)
	}
	if status, _ := preset(t, "?preset=tailor"); status != fiber.StatusBadRequest {
		t.Errorf("asking for tailor: status %d, want 400 — a tailoring session needs its CV binding", status)
	}
}

// A conversation begun on a vacancy in the side panel is one the candidate can
// pick up at their desk, so it belongs in the same rail as their chats.
func TestSessionListSpansRehearsalsButExcludesBrowsing(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "rail@example.test", true)

	// A browsing conversation's one distinguishing tool, read_current_page, only works
	// over the extension's own connection — see the confine-browse-preset-to-extension
	// change — so it stays out of the rail this endpoint serves, the same as tailoring.
	browsing, err := h.store.CreateSession(context.Background(), userID, assistant.PresetBrowse, nil, nil)
	if err != nil {
		t.Fatalf("create browsing session: %v", err)
	}
	// The other half of the same predicate: excluding browse and tailor must not have
	// swept a rehearsal out too. Only a real query can prove that — the store's own unit
	// tests run against a fake that never sees the WHERE clause. The binding is left
	// unset because what is on trial here is the preset filter, not the CV it points at.
	tailoring, err := h.store.CreateSession(context.Background(), userID, assistant.PresetTailor, nil, nil)
	if err != nil {
		t.Fatalf("create tailoring session: %v", err)
	}
	// A rehearsal is bound to a vacancy, but unlike a tailoring session it needs nothing
	// but itself to continue: the interview is days away, the candidate closes the tab,
	// and the conversation has to be somewhere they can find it.
	jobID := seedApplication(t, pool, userID, "rail-rehearsal", "interview")
	rehearsal, err := h.store.CreateSession(context.Background(), userID, assistant.PresetInterview, nil, &jobID)
	if err != nil {
		t.Fatalf("create rehearsal session: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions", cookie, nil)
	var list struct {
		Data []struct {
			ID     string `json:"id"`
			Preset string `json:"preset"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var sawRehearsal bool
	for _, s := range list.Data {
		if s.ID == rehearsal.ID.String() {
			sawRehearsal = true
		}
		if s.ID == tailoring.ID.String() {
			t.Errorf("the rail contains the tailoring session %s; it is reached through its CV, not here", tailoring.ID)
		}
		if s.ID == browsing.ID.String() {
			t.Errorf("the rail contains the browsing session %s; read_current_page only works from the extension", browsing.ID)
		}
	}
	if !sawRehearsal {
		t.Errorf("the rail = %+v, want it to contain the rehearsal %s", list.Data, rehearsal.ID)
	}
}

// The carrier decides nothing about standing: a user in no group is served over Bearer
// exactly as the cookie serves them. Paired with the unauthenticated case above, this
// pins the boundary at authentication and nowhere else.
func TestAssistantServesABearerCallerInNoGroup(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, token := assistantUser(t, pool, iss, "plain-extension@example.test", false)

	resp := assistantBearerRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions", token, map[string]any{})
	if resp.StatusCode != fiber.StatusCreated {
		t.Errorf("create over Bearer as a user in no group: status %d, want 201", resp.StatusCode)
	}
}

func TestAssistantTurnStreamsAndPersists(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Here is an answer."}}}
	app, _ := newAssistantApp(pool, iss, model)
	_, cookie := assistantUser(t, pool, iss, "turn@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "find me go jobs"})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("turn: status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content type = %q, want an SSE stream", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	stream := string(body)
	for _, want := range []string{"event: user_prompt", "event: assistant_text", "event: result"} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream is missing %q:\n%s", want, stream)
		}
	}
	if !strings.Contains(stream, assistant.StopEndTurn) {
		t.Errorf("stream does not end with a clean stop reason:\n%s", stream)
	}

	// The transcript is persisted and replays: prompt then answer.
	resp = assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions/"+id, cookie, nil)
	var read struct {
		Data struct {
			Session struct {
				Label string `json:"label"`
			} `json:"session"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&read); err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
	if len(read.Data.Messages) != 2 {
		t.Fatalf("transcript has %d messages, want the prompt and the answer", len(read.Data.Messages))
	}
	if read.Data.Messages[0].Role != assistant.RoleUser || read.Data.Messages[1].Role != assistant.RoleAssistant {
		t.Errorf("transcript roles = %+v, want user then assistant", read.Data.Messages)
	}
	// The rail names a session after its first message.
	if read.Data.Session.Label != "find me go jobs" {
		t.Errorf("label = %q, want the first user message", read.Data.Session.Label)
	}
}

func TestAssistantTurnRejectsAnEmptyMessage(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	_, cookie := assistantUser(t, pool, iss, "empty@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "   "})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("blank message: status %d, want 400", resp.StatusCode)
	}
}

func TestAssistantTurnWithoutAModelIsUnavailable(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil) // no runner: LLM unconfigured
	_, cookie := assistantUser(t, pool, iss, "nollm@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "hi"})
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("turn with no model: status %d, want 503", resp.StatusCode)
	}
}

func TestSlugAddressedToolsReadRealRows(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('acme', 'Acme Inc')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, description)
		 VALUES ('greenhouse', 'ext-1', 'https://example.test/j/1', 'Go Developer', 'go-developer-acme', 'acme', '<p>Build things</p>')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	a := &assistantHandlers{queries: db.New(pool)}
	tools := a.assistantDiscoveryTools()

	out, err := toolByName(t, tools, "get_job").Run(ctx, 1, json.RawMessage(`{"slug":"go-developer-acme"}`))
	if err != nil {
		t.Fatalf("get_job: %v", err)
	}
	payload, _ := json.Marshal(out)
	if !strings.Contains(string(payload), "Go Developer") {
		t.Errorf("get_job = %s, want the seeded vacancy", payload)
	}
	// The description reaches the model as markdown, not as the stored HTML.
	if strings.Contains(string(payload), "<p>") {
		t.Errorf("get_job = %s, want markdown rather than raw HTML", payload)
	}

	out, err = toolByName(t, tools, "get_company").Run(ctx, 1, json.RawMessage(`{"slug":"acme"}`))
	if err != nil {
		t.Fatalf("get_company: %v", err)
	}
	payload, _ = json.Marshal(out)
	if !strings.Contains(string(payload), "Acme Inc") || !strings.Contains(string(payload), "go-developer-acme") {
		t.Errorf("get_company = %s, want the company and its open vacancy", payload)
	}

	// An unknown slug is a tool error naming what was wrong, not a crash.
	if _, err := toolByName(t, tools, "get_job").Run(ctx, 1, json.RawMessage(`{"slug":"nope"}`)); err == nil {
		t.Error("get_job on an unknown slug returned no error")
	}
}

func TestASessionIdIsNotGuessable(t *testing.T) {
	// The id is a random UUID, so it publishes nothing about how many conversations
	// exist — and a malformed one is reported as missing, exactly like a foreign id,
	// so probing cannot tell the two apart.
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, cookie := assistantUser(t, pool, iss, "opaque@example.test", true)

	first := createSession(t, app, cookie)
	second := createSession(t, app, cookie)
	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("session id %q is not a UUID: %v", first, err)
	}
	if first == second {
		t.Fatal("two sessions share an id")
	}
	// Sequential ids would differ by one; random ones cannot be walked.
	if len(first) != len(second) || first[:8] == second[:8] {
		t.Errorf("ids %q and %q look related; they must be independently random", first, second)
	}

	for _, bad := range []string{"9", "0", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions/"+bad, cookie, nil)
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("GET session %q: status %d, want 404", bad, resp.StatusCode)
		}
	}
}

// failingTurnModel stands in for an upstream that is down — a 502 from the proxy is an
// ordinary event here, and the turn dies with it.
type failingTurnModel struct{}

func (failingTurnModel) Chat(context.Context, []llms.MessageContent, []llms.Tool, llm.ChatStream) (*llms.ContentChoice, error) {
	return nil, errors.New("upstream llm exploded")
}

// The assistant stream has the fit stream's shape: the handler returns nil before the body
// writer runs, so a turn that dies never reaches RenderError and the access log keeps the
// 200 the stream opened with. Logging it is not enough — nothing watches the journal, and
// it keeps ~12 hours.
func TestAssistantTurnFailureReachesSentry(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	tr := &recordingTransport{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://public@o0.ingest.sentry.io/0",
		Transport: tr,
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}
	app, _ := newAssistantApp(pool, iss, failingTurnModel{},
		sentryfiber.New(sentryfiber.Options{Repanic: true, WaitForDelivery: true}))
	_, cookie := assistantUser(t, pool, iss, "turnfail@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "find me go jobs"})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("turn: status %d, want the 200 the stream opens with", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if got := tr.count(); got != 1 {
		t.Errorf("sentry events = %d, want exactly 1 for a failed turn", got)
	}
}
