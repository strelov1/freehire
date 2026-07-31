package speech

import (
	"context"
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
	path      string
	authz     string
	model     string
	format    string
	filename  string
	audio     []byte
	multipart bool
}

// gateway stands in for the OpenAI-compatible endpoint. It records the request and
// answers with body and status.
func gateway(t *testing.T, status int, body string) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.authz = r.Header.Get("Authorization")
		got.multipart = strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			got.model = r.FormValue("model")
			got.format = r.FormValue("response_format")
			if f, hdr, err := r.FormFile("file"); err == nil {
				defer f.Close()
				got.filename = hdr.Filename
				got.audio, _ = io.ReadAll(f)
			}
		}
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

func TestTranscribeSendsTheAudioAndReturnsTheText(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"text":"find me remote go jobs"}`)
	c := New(srv.URL+"/v1", "sk-test", "whisper-1")

	text, err := c.Transcribe(context.Background(), []byte("OpusOpusOpus"), "dictation.webm")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "find me remote go jobs" {
		t.Errorf("text = %q, want %q", text, "find me remote go jobs")
	}
	if got.path != "/v1/audio/transcriptions" {
		t.Errorf("path = %q, want /v1/audio/transcriptions", got.path)
	}
	if got.authz != "Bearer sk-test" {
		t.Errorf("authorization = %q, want Bearer sk-test", got.authz)
	}
	if !got.multipart {
		t.Error("request was not multipart/form-data")
	}
	if got.model != "whisper-1" {
		t.Errorf("model = %q, want whisper-1", got.model)
	}
	if got.format != "json" {
		t.Errorf("response_format = %q, want json", got.format)
	}
	// The upstream sniffs the container by extension, so the caller's filename has to
	// survive the round trip rather than being replaced with one of ours.
	if got.filename != "dictation.webm" {
		t.Errorf("filename = %q, want dictation.webm", got.filename)
	}
	if string(got.audio) != "OpusOpusOpus" {
		t.Errorf("audio = %q, want OpusOpusOpus", got.audio)
	}
}

func TestTranscribeJoinsTheBaseURLWithoutDoublingTheSlash(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"text":"ok"}`)
	c := New(srv.URL+"/v1/", "sk-test", "whisper-1")

	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.webm"); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got.path != "/v1/audio/transcriptions" {
		t.Errorf("path = %q, want /v1/audio/transcriptions", got.path)
	}
}

func TestTranscribeReportsAnUpstreamRefusal(t *testing.T) {
	srv, _ := gateway(t, http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`)
	c := New(srv.URL+"/v1", "sk-test", "whisper-1")

	_, err := c.Transcribe(context.Background(), []byte("a"), "a.webm")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want it to wrap ErrUpstream", err)
	}
	// The upstream's own status belongs in the message: it is the only clue a log
	// reader has about whether the gateway refused us or fell over.
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("err = %q, want it to name the status", err)
	}
}

func TestTranscribeReportsAnUnreadableAnswer(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `not json at all`)
	c := New(srv.URL+"/v1", "sk-test", "whisper-1")

	if _, err := c.Transcribe(context.Background(), []byte("a"), "a.webm"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want it to wrap ErrUpstream", err)
	}
}

func TestTranscribeAcceptsSilenceAsAnEmptyString(t *testing.T) {
	// Whisper answers an empty string for audio with no speech in it. That is a
	// successful transcription of nothing, not a failure: the composer decides what
	// to do with it, and appending nothing is the right answer there.
	srv, _ := gateway(t, http.StatusOK, `{"text":""}`)
	c := New(srv.URL+"/v1", "sk-test", "whisper-1")

	text, err := c.Transcribe(context.Background(), []byte("silence"), "a.webm")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}

func TestTranscribeHonoursACancelledContext(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `{"text":"ok"}`)
	c := New(srv.URL+"/v1", "sk-test", "whisper-1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Transcribe(ctx, []byte("a"), "a.webm"); err == nil {
		t.Fatal("Transcribe on a cancelled context returned no error")
	}
}
