package handler

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

func TestInterviewTranscription(t *testing.T) {
	apiKey, hash, _, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	issuer := auth.NewIssuer("test-secret", time.Hour)
	gateway := &fakeTranscriber{text: "the interview transcript"}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	newSpeechHandlers(gateway, nil).register(app.Group("/api/v1"), middleware{
		key:    auth.RequireAuthOrKey(issuer, testVersions, stubValidKey{hash: hash, userID: 7}),
		paidAI: markPaidAI,
	})

	for _, tt := range []struct {
		name  string
		audio []byte
		key   bool
		want  int
	}{
		{"anonymous", interviewTestWAV(32000), false, fiber.StatusUnauthorized},
		{"malformed", []byte("not a WAV"), true, fiber.StatusBadRequest},
		{"over 60 seconds", interviewTestWAV(60*32000 + 2), true, fiber.StatusBadRequest},
		{"60 seconds", interviewTestWAV(60 * 32000), true, fiber.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body, contentType := audioUpload(t, tt.audio, "interview.wav")
			req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/api/v1/me/interviews/transcriptions", body)
			req.Header.Set(fiber.HeaderContentType, contentType)
			if tt.key {
				req.Header.Set(fiber.HeaderAuthorization, "Bearer "+apiKey)
			}
			before := gateway.calls
			res, err := app.Test(req, 5000)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.want)
			}
			if tt.want != fiber.StatusOK {
				if gateway.calls != before {
					t.Fatal("rejected audio reached the inference gateway")
				}
				return
			}
			var result struct {
				Data struct {
					Text string `json:"text"`
				} `json:"data"`
			}
			if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result.Data.Text != gateway.text || res.Header.Get(fiber.HeaderCacheControl) != "private, no-store" {
				t.Fatalf("text = %q, cache control = %q", result.Data.Text, res.Header.Get(fiber.HeaderCacheControl))
			}
			if gateway.calls != before+1 || gateway.filename != "dictation.wav" || !bytes.Equal(gateway.audio, tt.audio) {
				t.Fatal("gateway did not receive the WAV with its sanitized filename")
			}
		})
	}
}

func interviewTestWAV(sampleBytes uint32) []byte {
	audio := make([]byte, 44+sampleBytes)
	copy(audio, "RIFF")
	binary.LittleEndian.PutUint32(audio[4:], sampleBytes+36)
	copy(audio[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(audio[16:], 16)
	binary.LittleEndian.PutUint16(audio[20:], 1)
	binary.LittleEndian.PutUint16(audio[22:], 1)
	binary.LittleEndian.PutUint32(audio[24:], 16000)
	binary.LittleEndian.PutUint32(audio[28:], 32000)
	binary.LittleEndian.PutUint16(audio[32:], 2)
	binary.LittleEndian.PutUint16(audio[34:], 16)
	copy(audio[36:], "data")
	binary.LittleEndian.PutUint32(audio[40:], sampleBytes)
	return audio
}
