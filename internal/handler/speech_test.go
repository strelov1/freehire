package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/speech"
)

// fakeTranscriber records what it was handed and answers with a fixed result, so a
// test can assert both the response and — for every refusal path — that the metered
// upstream was never reached.
type fakeTranscriber struct {
	calls    int
	audio    []byte
	filename string
	text     string
	err      error
}

func (f *fakeTranscriber) Transcribe(_ context.Context, audio []byte, filename string) (string, error) {
	f.calls++
	f.audio = audio
	f.filename = filename
	return f.text, f.err
}

func speechApp(t *testing.T, stt transcriber) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/speech/transcriptions", auth.RequireAuth(iss, testVersions),
		(&speechHandlers{stt: stt}).PostTranscription)
	return app, token
}

// audioUpload builds a multipart body with the recording under the "file" part.
func audioUpload(t *testing.T, data []byte, filename string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func doTranscription(t *testing.T, app *fiber.App, body io.Reader, ctype, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/speech/transcriptions", body)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestTranscription_ReturnsTheText(t *testing.T) {
	stt := &fakeTranscriber{text: "compare the first two"}
	app, token := speechApp(t, stt)

	body, ctype := audioUpload(t, []byte("OpusOpus"), "dictation.webm")
	resp := doTranscription(t, app, body, ctype, token)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Text string `json:"text"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.Text != "compare the first two" {
		t.Errorf("text = %q, want %q", out.Data.Text, "compare the first two")
	}
	if string(stt.audio) != "OpusOpus" {
		t.Errorf("audio = %q, want OpusOpus", stt.audio)
	}
	// The container is identified upstream by extension, so the browser's own filename
	// has to reach the gateway rather than one invented here.
	if stt.filename != "dictation.webm" {
		t.Errorf("filename = %q, want dictation.webm", stt.filename)
	}
}

func TestTranscription_ForwardsASafeFilenameNotTheClientsOwn(t *testing.T) {
	// Go's multipart writer escapes quotes in a filename but NOT CRLF, so a crafted
	// name would inject header lines into the request WE send to the gateway. Only the
	// extension is worth trusting — the upstream identifies the container by it — so
	// that is all that survives.
	tests := []struct {
		name, upload, want string
	}{
		{"header injection", "a.webm\r\nX-Evil: 1", ""},
		{"path traversal", "../../etc/passwd.webm", "dictation.webm"},
		{"upper case extension", "Recording.WEBM", "dictation.webm"},
		{"mp4 from safari", "recording.mp4", "dictation.mp4"},
		{"no extension", "recording", ""},
		{"unknown container", "recording.exe", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stt := &fakeTranscriber{text: "ok"}
			app, token := speechApp(t, stt)

			body, ctype := audioUpload(t, []byte("OpusOpus"), tt.upload)
			resp := doTranscription(t, app, body, ctype, token)
			defer resp.Body.Close()

			if tt.want == "" {
				if resp.StatusCode != fiber.StatusBadRequest {
					t.Errorf("status = %d, want 400", resp.StatusCode)
				}
				if stt.calls != 0 {
					t.Errorf("the gateway was called %d time(s) for a refused filename", stt.calls)
				}
				return
			}
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if stt.filename != tt.want {
				t.Errorf("forwarded filename = %q, want %q", stt.filename, tt.want)
			}
		})
	}
}

func TestTranscription_EmptyRecordingIsRefusedBeforeTheGateway(t *testing.T) {
	stt := &fakeTranscriber{text: "never"}
	app, token := speechApp(t, stt)

	body, ctype := audioUpload(t, nil, "a.webm")
	resp := doTranscription(t, app, body, ctype, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if stt.calls != 0 {
		t.Errorf("the gateway was called %d time(s) for an empty recording", stt.calls)
	}
}

func TestTranscription_MissingFilePartIs400(t *testing.T) {
	stt := &fakeTranscriber{text: "never"}
	app, token := speechApp(t, stt)

	resp := doTranscription(t, app, bytes.NewReader(nil), "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if stt.calls != 0 {
		t.Errorf("the gateway was called %d time(s) for a request with no audio", stt.calls)
	}
}

func TestTranscription_RequiresAuthentication(t *testing.T) {
	stt := &fakeTranscriber{text: "never"}
	app, _ := speechApp(t, stt)

	body, ctype := audioUpload(t, []byte("OpusOpus"), "a.webm")
	resp := doTranscription(t, app, body, ctype, "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if stt.calls != 0 {
		t.Errorf("the gateway was called %d time(s) for an unauthenticated request", stt.calls)
	}
}

func TestTranscription_OversizeUploadIsRefusedBeforeTheGateway(t *testing.T) {
	stt := &fakeTranscriber{text: "never"}
	app, token := speechApp(t, stt)

	body, ctype := audioUpload(t, bytes.Repeat([]byte("x"), maxAudioUpload+1), "a.webm")
	resp := doTranscription(t, app, body, ctype, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if stt.calls != 0 {
		t.Errorf("the gateway was called %d time(s) for an oversize upload", stt.calls)
	}
}

func TestTranscription_UnconfiguredGatewayIs501(t *testing.T) {
	// nil, not a fake: this is the deployment with no speech gateway at all, and the
	// SPA reads 501 as "this surface does not exist here" rather than as a fault.
	app, token := speechApp(t, nil)

	body, ctype := audioUpload(t, []byte("OpusOpus"), "a.webm")
	resp := doTranscription(t, app, body, ctype, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Errorf("status = %d, want 501", resp.StatusCode)
	}
}

func TestTranscription_UpstreamFailureIs502(t *testing.T) {
	app, token := speechApp(t, &fakeTranscriber{err: fmt.Errorf("%w: status 500", speech.ErrUpstream)})

	body, ctype := audioUpload(t, []byte("OpusOpus"), "a.webm")
	resp := doTranscription(t, app, body, ctype, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestTranscription_ThrottlesOneCallerRatherThanOneAddress(t *testing.T) {
	stt := &fakeTranscriber{text: "ok"}
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/speech/transcriptions", auth.RequireAuth(iss, testVersions), perCallerLimiter(2),
		(&speechHandlers{stt: stt}).PostTranscription)

	for attempt := 1; attempt <= 3; attempt++ {
		body, ctype := audioUpload(t, []byte("OpusOpus"), "a.webm")
		resp := doTranscription(t, app, body, ctype, token)
		resp.Body.Close()
		want := fiber.StatusOK
		if attempt == 3 {
			want = fiber.StatusTooManyRequests
		}
		if resp.StatusCode != want {
			t.Errorf("attempt %d status = %d, want %d", attempt, resp.StatusCode, want)
		}
	}
	if stt.calls != 2 {
		t.Errorf("the gateway was called %d time(s), want 2 — the throttled request must not reach it", stt.calls)
	}
}

// The microphone sits in the composer that posts messages, so anything allowed to
// post a message is allowed to transcribe for it: the same gate as
// POST /assistant/sessions/:id/messages, and not the narrower cookie-only one.
func TestSpeechRegister_TakesTheMessageEndpointsGate(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&speechHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/speech/transcriptions", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "key" {
		t.Errorf("the transcription route is gated by %q, want %q", got, "key")
	}
}
