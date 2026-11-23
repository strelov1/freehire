package speech

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestElevenLabsAudio(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("xi-api-key") != "fixture" {
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/speech-to-text":
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Errorf("read uploaded recording: %v", err)
				http.Error(w, "missing recording", http.StatusBadRequest)
				return
			}
			defer file.Close()
			audio, err := io.ReadAll(file)
			if err != nil || string(audio) != "test recording" {
				t.Errorf("provider audio = %q, %v", audio, err)
			}
			if header.Filename != "recording.webm" || r.FormValue("model_id") != "scribe_v2" {
				t.Errorf("unexpected transcription form: %s, %s", header.Filename, r.FormValue("model_id"))
			}
			_, _ = w.Write([]byte(`{"text":" I measured error rates. "}`))
		case "/text-to-speech/TestVoice123456":
			var body struct {
				Text  string `json:"text"`
				Model string `json:"model_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode speech body: %v", err)
			}
			if body.Text != "How did you measure the result?" || body.Model != "eleven_multilingual_v2" {
				t.Errorf("unexpected synthesis request: %+v", body)
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("mp3 audio"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	client := NewElevenLabs("fixture", "TestVoice123456")
	client.url = upstream.URL
	text, err := client.Transcribe(context.Background(), []byte("test recording"), "recording.webm")
	if err != nil || text != "I measured error rates." {
		t.Fatalf("Transcribe() = %q, %v", text, err)
	}
	audio, err := client.Speak(context.Background(), "How did you measure the result?")
	if err != nil || string(audio) != "mp3 audio" {
		t.Fatalf("Speak() = %q, %v", audio, err)
	}
}

func TestElevenLabsAudioThroughBifrost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Errorf("unexpected gateway request: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/v1/audio/transcriptions":
			if model := r.FormValue("model"); model != "elevenlabs/scribe_v2" {
				t.Errorf("unexpected transcription model: %q", model)
			}
			_, _ = io.WriteString(w, `{"text":"I measured error rates."}`)
		case "/v1/audio/speech":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["model"] != "elevenlabs/eleven_multilingual_v2" || body["voice"] != "TestVoice123456" ||
				body["input"] != "How did you measure the result?" || body["response_format"] != "mp3" {
				t.Errorf("unexpected synthesis body: %+v", body)
			}
			_, _ = io.WriteString(w, "ID3audio")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	client := NewElevenLabsGateway(upstream.URL+"/v1", "fixture", "TestVoice123456")
	text, err := client.Transcribe(context.Background(), []byte("test recording"), "recording.webm")
	if err != nil || text != "I measured error rates." {
		t.Fatalf("Transcribe() = %q, %v", text, err)
	}
	audio, err := client.Speak(context.Background(), "How did you measure the result?")
	if err != nil || string(audio) != "ID3audio" {
		t.Fatalf("Speak() = %q, %v", audio, err)
	}
}
