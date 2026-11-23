//go:build integration

package handler

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/identity/auth"
)

type interviewAudioStub struct {
	spoken     string
	recorded   string
	recordName string
}

func (s *interviewAudioStub) Speak(_ context.Context, text string) ([]byte, error) {
	s.spoken = text
	return []byte("mp3 audio"), nil
}

func (s *interviewAudioStub) Transcribe(_ context.Context, audio []byte, filename string) (string, error) {
	s.recorded = string(audio)
	s.recordName = filename
	return "I would compare error rates.", nil
}

func TestInterviewAudioUsesOwnedRehearsal(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	issuer := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, issuer, nil)
	stub := &interviewAudioStub{}
	h.interviewAudio = stub

	owner, cookie := assistantUser(t, pool, issuer, "interview-audio-owner@example.test", true)
	_, stranger := assistantUser(t, pool, issuer, "interview-audio-stranger@example.test", true)
	job := seedApplication(t, pool, owner, "interview-audio", "interview")
	session, err := h.store.CreateSession(
		ctx,
		owner,
		assistant.PresetInterview,
		nil,
		&job,
	)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := assistant.EncodeAssistant("How did you measure the result?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Append(ctx, session.ID, reply); err != nil {
		t.Fatal(err)
	}

	base := "/api/v1/assistant/sessions/" + session.ID.String()
	detail := assistantRequest(t, app, fiber.MethodGet, base, cookie, nil)
	defer detail.Body.Close()
	json, err := io.ReadAll(detail.Body)
	if err != nil || !bytes.Contains(json, []byte(`"elevenlabs_voice":true`)) {
		t.Fatalf("missing voice capability: %s (%v)", json, err)
	}

	speech := assistantRequest(
		t,
		app,
		fiber.MethodPost,
		base+"/interview-speech",
		stranger,
		nil,
	)
	defer speech.Body.Close()
	if speech.StatusCode != http.StatusNotFound || stub.spoken != "" {
		t.Fatalf("speech for someone else's session = %d", speech.StatusCode)
	}

	speech = assistantRequest(
		t,
		app,
		fiber.MethodPost,
		base+"/interview-speech",
		cookie,
		nil,
	)
	defer speech.Body.Close()
	audio, err := io.ReadAll(speech.Body)
	if err != nil || speech.StatusCode != http.StatusOK || string(audio) != "mp3 audio" || speech.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("speech response: status %d, body %q, error %v", speech.StatusCode, audio, err)
	}
	if stub.spoken != "How did you measure the result?" {
		t.Fatalf("unexpected spoken text = %q", stub.spoken)
	}

	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	part, err := form.CreateFormFile("file", "candidate.webm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "candidate's audio"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(ctx, fiber.MethodPost, base+"/interview-transcriptions", &upload)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Cookie", auth.CookieName+"="+cookie)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("transcription: status %d, body %q, error %v", response.StatusCode, body, err)
	}
	if !strings.Contains(string(body), "I would compare error rates.") || stub.recorded != "candidate's audio" {
		t.Fatalf("transcription did not reach the browser: %s", body)
	}
	if stub.recordName != "dictation.webm" {
		t.Fatalf("provider received an unexpected filename: %q", stub.recordName)
	}
	stored, err := h.store.Transcript(ctx, session.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("transcription wrote a message before the candidate sent it: %v, %v", stored, err)
	}
}
