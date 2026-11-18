//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// TestReferralMessageStream checks delivery and the allowance at the stream boundary.
func TestReferralMessageStream(t *testing.T) {
	for _, tt := range []struct {
		name     string
		fail     bool
		wantUsed int
		wantSSE  string
	}{
		{name: "successful draft keeps its charge", wantUsed: 1, wantSSE: "message"},
		{name: "failed draft releases its charge", fail: true, wantUsed: 0, wantSSE: "stream_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool := startPostgres(t)
			fixture := newCoverLetterFixture(t, pool)
			ctx := context.Background()
			atom, err := fixture.bank.AddAtom(ctx, fixture.userID, experience.Atom{
				Claim: "I cut p95 latency from 800ms to 120ms",
			}, experience.AuthorCandidate)
			if err != nil {
				t.Fatalf("bank atom: %v", err)
			}
			makePro(t, pool, fixture.userID)
			plans := plan.NewStore(db.New(pool), pool,
				plan.DefaultConfig().WithProCeiling(plan.FeatureCoverLetter, 1).Enforcing())

			message := "Hello Sam, I saw your engineering work at Acme. I am applying for the backend role, and I cut p95 latency from 800ms to 120ms in a previous project. If you feel comfortable, would you consider referring me? Thanks for considering it."
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.fail {
					http.Error(w, "provider unavailable", http.StatusInternalServerError)
					return
				}
				draft, _ := json.Marshal(map[string]any{
					"body": message, "cited_atom_ids": []string{atom.ID.String()},
				})
				w.Header().Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
				_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, draft)
			}))
			t.Cleanup(gateway.Close)
			client, err := llm.New(gateway.URL, "test-key", "test-model")
			if err != nil {
				t.Fatalf("model client: %v", err)
			}
			h := newReferralMessageHandlers(fixture.h.queries, fixture.bank, fixture.h.resume,
				fixture.h.fit, coverletter.NewAnalyzer(client), llmBinding{client: client}, plans)
			app := fiber.New(fiber.Config{ErrorHandler: RenderError})
			app.Post("/api/v1/jobs/:slug/referral-message/stream", func(c *fiber.Ctx) error {
				c.Locals(auth.LocalsUserID, fixture.userID)
				c.SetUserContext(plan.WithPaidAI(c.UserContext()))
				return c.Next()
			}, h.Stream)
			req := httptest.NewRequestWithContext(ctx, fiber.MethodPost,
				"/api/v1/jobs/backend-kafka-letter/referral-message/stream",
				strings.NewReader(`{"channel":"linkedin","profile":"Sam, engineer at Acme"}`))
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			resp, err := app.Test(req, 10_000)
			if err != nil {
				t.Fatalf("stream request: %v", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read stream: %v", err)
			}
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
			}
			data, ok := referralSSEData(string(body), tt.wantSSE)
			if !ok {
				t.Fatalf("missing %s event in %q", tt.wantSSE, body)
			}
			if tt.fail {
				if !strings.Contains(data, referralDraftFailure) || strings.Contains(data, "provider unavailable") {
					t.Errorf("stream error = %q, want only generic failure text", data)
				}
			} else {
				var delivered struct {
					Body  string `json:"body"`
					Cited []struct {
						ID    string `json:"id"`
						Claim string `json:"claim"`
					} `json:"cited"`
				}
				if err := json.Unmarshal([]byte(data), &delivered); err != nil {
					t.Fatalf("message event: %v", err)
				}
				if delivered.Body != message || len(delivered.Cited) != 1 ||
					delivered.Cited[0].ID != atom.ID.String() || delivered.Cited[0].Claim != atom.Claim {
					t.Errorf("message event = %+v, want the body and cited bank atom", delivered)
				}
			}
			standing, err := plans.Standing(ctx, fixture.userID, plan.FeatureCoverLetter)
			if err != nil {
				t.Fatalf("allowance: %v", err)
			}
			if standing.Used != tt.wantUsed {
				t.Errorf("used = %d, want %d", standing.Used, tt.wantUsed)
			}
		})
	}
}

// referralSSEData reads the JSON payload of the named event from a completed test stream.
func referralSSEData(stream, name string) (string, bool) {
	for _, frame := range strings.Split(strings.ReplaceAll(stream, "\r\n", "\n"), "\n\n") {
		if !strings.HasPrefix(frame, "event: "+name+"\n") {
			continue
		}
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "data: ") {
				return strings.TrimPrefix(line, "data: "), true
			}
		}
	}
	return "", false
}
