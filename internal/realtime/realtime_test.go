package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captured is what the fake gateway saw, so a test can assert on the request the
// client built rather than only on what it returned.
type captured struct {
	path         string
	authz        string
	contentType  string
	sessionType  string
	model        string
	instructions string
}

// gateway stands in for the OpenAI-compatible /realtime/client_secrets endpoint. It
// records the request and answers with body and status.
func gateway(t *testing.T, status int, body string) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.authz = r.Header.Get("Authorization")
		got.contentType = r.Header.Get("Content-Type")
		var payload struct {
			Session struct {
				Type         string `json:"type"`
				Model        string `json:"model"`
				Instructions string `json:"instructions"`
			} `json:"session"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		got.sessionType = payload.Session.Type
		got.model = payload.Session.Model
		got.instructions = payload.Session.Instructions

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestNewReportsUnconfiguredAsNil(t *testing.T) {
	tests := []struct {
		name                   string
		baseURL, apiKey, model string
	}{
		{"all empty", "", "", ""},
		{"no base url", "", "k", "m"},
		{"no key", "https://gw.example/v1", "", "m"},
		{"no model", "https://gw.example/v1", "k", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if c := New(tt.baseURL, tt.apiKey, tt.model); c != nil {
				t.Fatalf("New(%q, %q, %q) = %v, want nil", tt.baseURL, tt.apiKey, tt.model, c)
			}
		})
	}
}

// The WebRTC SDP exchange names the model on its URL, and the browser holds only the
// ephemeral secret — never the deployment's model choice — so it has to learn the
// name from the same place it got the secret, rather than a value baked into the
// frontend that could drift from REALTIME_MODEL.
func TestModelReturnsWhatTheClientWasConfiguredWith(t *testing.T) {
	c := New("https://gw.example/v1", "sk-test", "gpt-realtime-2.1")
	if got := c.Model(); got != "gpt-realtime-2.1" {
		t.Errorf("Model() = %q, want gpt-realtime-2.1", got)
	}
}

func TestMintClientSecretSendsInstructionsAndReturnsTheValue(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"value":"ek_abc123","expires_at":1234}`)
	c := New(srv.URL+"/v1", "sk-test", "gpt-realtime-2.1")

	value, err := c.MintClientSecret(context.Background(), "You are the interviewer.")
	if err != nil {
		t.Fatalf("MintClientSecret: %v", err)
	}
	if value != "ek_abc123" {
		t.Errorf("value = %q, want ek_abc123", value)
	}
	if got.path != "/v1/realtime/client_secrets" {
		t.Errorf("path = %q, want /v1/realtime/client_secrets", got.path)
	}
	if got.authz != "Bearer sk-test" {
		t.Errorf("authorization = %q, want Bearer sk-test", got.authz)
	}
	if !strings.HasPrefix(got.contentType, "application/json") {
		t.Errorf("content-type = %q, want application/json", got.contentType)
	}
	if got.sessionType != "realtime" {
		t.Errorf("session.type = %q, want realtime", got.sessionType)
	}
	if got.model != "gpt-realtime-2.1" {
		t.Errorf("session.model = %q, want gpt-realtime-2.1", got.model)
	}
	if got.instructions != "You are the interviewer." {
		t.Errorf("session.instructions = %q, want %q", got.instructions, "You are the interviewer.")
	}
}

func TestMintClientSecretJoinsTheBaseURLWithoutDoublingTheSlash(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"value":"ek_abc123"}`)
	c := New(srv.URL+"/v1/", "sk-test", "gpt-realtime-2.1")

	if _, err := c.MintClientSecret(context.Background(), "hi"); err != nil {
		t.Fatalf("MintClientSecret: %v", err)
	}
	if got.path != "/v1/realtime/client_secrets" {
		t.Errorf("path = %q, want /v1/realtime/client_secrets", got.path)
	}
}

func TestMintClientSecretReportsAnUpstreamRefusal(t *testing.T) {
	srv, _ := gateway(t, http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`)
	c := New(srv.URL+"/v1", "sk-test", "gpt-realtime-2.1")

	_, err := c.MintClientSecret(context.Background(), "hi")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want it to wrap ErrUpstream", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("err = %q, want it to name the status", err)
	}
}

func TestMintClientSecretReportsAnUnreadableAnswer(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `not json at all`)
	c := New(srv.URL+"/v1", "sk-test", "gpt-realtime-2.1")

	if _, err := c.MintClientSecret(context.Background(), "hi"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want it to wrap ErrUpstream", err)
	}
}

func TestMintClientSecretReportsAMissingValue(t *testing.T) {
	// The gateway answered 200 but without the one field that matters — treat it the
	// same as an unreadable answer rather than handing the caller an empty secret.
	srv, _ := gateway(t, http.StatusOK, `{"expires_at":1234}`)
	c := New(srv.URL+"/v1", "sk-test", "gpt-realtime-2.1")

	if _, err := c.MintClientSecret(context.Background(), "hi"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want it to wrap ErrUpstream", err)
	}
}

func TestMintClientSecretHonoursACancelledContext(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `{"value":"ek_abc123"}`)
	c := New(srv.URL+"/v1", "sk-test", "gpt-realtime-2.1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.MintClientSecret(ctx, "hi"); err == nil {
		t.Fatal("MintClientSecret on a cancelled context returned no error")
	}
}
