package autofillagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// jevServer replays the given answer for any /v1/systemone call.
func jevServer(t *testing.T, answer map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{jevAnswerKey: answer},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestJevChooserPicksTheOfferedOption(t *testing.T) {
	srv := jevServer(t, map[string]any{"choice": "Germany"})
	chooser := newJevChooser("test-key", withJevBaseURL(srv.URL))

	got, err := chooser.Choose(context.Background(), Field{Label: "Which country are you located in?"},
		[]string{"Germany", "France", "Poland"}, Profile{"location": "Berlin, Germany"})
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "Germany" {
		t.Errorf("Choose = %q, want %q", got, "Germany")
	}
}

func TestJevChooserDeclineMapsToEmptyString(t *testing.T) {
	srv := jevServer(t, map[string]any{"choice": jevDeclineKey})
	chooser := newJevChooser("test-key", withJevBaseURL(srv.URL))

	got, err := chooser.Choose(context.Background(), Field{Label: "What is your notice period?"},
		[]string{"Immediately", "2 weeks", "1 month"}, Profile{})
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "" {
		t.Errorf("Choose = %q, want empty (decline)", got)
	}
}

func TestJevChooserErrorsOnUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // connection refused on every attempt — one shot, no retries

	chooser := newJevChooser("test-key", withJevBaseURL(srv.URL))
	if _, err := chooser.Choose(context.Background(), Field{Label: "Q"}, []string{"A", "B"}, Profile{}); err == nil {
		t.Fatal("Choose against an unreachable server returned no error")
	}
}

func TestJevChooserErrorsOnMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers": not valid json`))
	}))
	t.Cleanup(srv.Close)

	chooser := newJevChooser("test-key", withJevBaseURL(srv.URL))
	if _, err := chooser.Choose(context.Background(), Field{Label: "Q"}, []string{"A", "B"}, Profile{}); err == nil {
		t.Fatal("Choose against a malformed JSON body returned no error")
	}
}

func TestJevChooserErrorsWhenAnswerKeyIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{}, // valid JSON, but no "answer" entry
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)

	chooser := newJevChooser("test-key", withJevBaseURL(srv.URL))
	if _, err := chooser.Choose(context.Background(), Field{Label: "Q"}, []string{"A", "B"}, Profile{}); err == nil {
		t.Fatal("Choose against a response with no answer entry returned no error")
	}
}

func TestJevChooserErrorsOnNon200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	chooser := newJevChooser("bad-key", withJevBaseURL(srv.URL))
	if _, err := chooser.Choose(context.Background(), Field{Label: "Q"}, []string{"A", "B"}, Profile{}); err == nil {
		t.Fatal("Choose against a 401 response returned no error")
	}
}
