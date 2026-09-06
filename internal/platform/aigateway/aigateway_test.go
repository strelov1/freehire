package aigateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// errUpstream stands in for a surface's own sentinel — the thing this package must wrap
// every failure in rather than substituting one of its own.
var errUpstream = errors.New("test gateway")

func testConfig(baseURL string) Config {
	return Config{
		BaseURL:     baseURL,
		APIKey:      "sk-test",
		Model:       "m-1",
		Timeout:     5 * time.Second,
		MaxResponse: 1 << 16,
		ErrUpstream: errUpstream,
	}
}

// gateway records what the fake endpoint saw and answers with status and body.
func gateway(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var (
		seen    http.Request
		seenRaw []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r
		seenRaw, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen, &seenRaw
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
			cfg := testConfig(tt.baseURL)
			cfg.APIKey, cfg.Model = tt.apiKey, tt.model
			if c := New(cfg); c != nil {
				t.Fatalf("New(%q, %q, %q) = %v, want nil", tt.baseURL, tt.apiKey, tt.model, c)
			}
		})
	}
}

func TestURLJoinsWithoutDoublingTheSlash(t *testing.T) {
	c := New(testConfig("https://gw.example/v1/"))
	if got := c.URL("/realtime/calls"); got != "https://gw.example/v1/realtime/calls" {
		t.Errorf("URL = %q, want https://gw.example/v1/realtime/calls", got)
	}
}

func TestPostSendsTheCredentialAndContentTypeAndReturnsTheBody(t *testing.T) {
	srv, seen, raw := gateway(t, http.StatusOK, `{"value":"ok"}`)
	c := New(testConfig(srv.URL + "/v1"))

	body, err := c.Post(context.Background(), "/realtime/client_secrets", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if string(body) != `{"value":"ok"}` {
		t.Errorf("body = %q, want the gateway's answer verbatim", body)
	}
	if seen.URL.Path != "/v1/realtime/client_secrets" {
		t.Errorf("path = %q, want /v1/realtime/client_secrets", seen.URL.Path)
	}
	if got := seen.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("authorization = %q, want Bearer sk-test", got)
	}
	if got := seen.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q, want application/json", got)
	}
	if string(*raw) != `{"a":1}` {
		t.Errorf("request body = %q, want it forwarded unchanged", *raw)
	}
}

// A refusal must carry the gateway's own sentence, because that sentence is the only
// thing distinguishing a bad key from a rate limit in a log — which is why the body is
// read BEFORE the status is switched on.
func TestPostReportsARefusalWithTheGatewaysOwnExplanation(t *testing.T) {
	srv, _, _ := gateway(t, http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`)
	c := New(testConfig(srv.URL + "/v1"))

	_, err := c.Post(context.Background(), "/x", "application/json", nil)
	if !errors.Is(err, errUpstream) {
		t.Fatalf("err = %v, want it to wrap the caller's own sentinel", err)
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "slow down") {
		t.Errorf("err = %q, want it to name the status and quote the body", err)
	}
}

func TestPostBoundsWhatItReadsBack(t *testing.T) {
	srv, _, _ := gateway(t, http.StatusOK, strings.Repeat("a", 4096))
	cfg := testConfig(srv.URL + "/v1")
	cfg.MaxResponse = 64
	c := New(cfg)

	body, err := c.Post(context.Background(), "/x", "application/json", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(body) != 64 {
		t.Fatalf("read %d bytes, want it truncated at MaxResponse (64)", len(body))
	}
}

func TestPostHonoursACancelledContext(t *testing.T) {
	srv, _, _ := gateway(t, http.StatusOK, `{}`)
	c := New(testConfig(srv.URL + "/v1"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Post(ctx, "/x", "application/json", nil); err == nil {
		t.Fatal("Post on a cancelled context returned no error")
	}
}
