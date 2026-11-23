package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/aigateway"
)

const elevenLabsURL = "https://api.elevenlabs.io/v1"
const elevenLabsAudioLimit = 4 << 20

var elevenLabsVoiceID = regexp.MustCompile(`^[a-zA-Z0-9]{10,40}$`)

// ElevenLabs supplies recognition and playback for the interview rehearsal.
type ElevenLabs struct {
	apiKey string
	voice  string
	url    string
	client *http.Client
	stt    *Client
	tts    *aigateway.Client
}

func NewElevenLabs(apiKey, voice string) *ElevenLabs {
	if strings.TrimSpace(apiKey) == "" || !elevenLabsVoiceID.MatchString(voice) {
		return nil
	}
	return &ElevenLabs{
		apiKey: strings.TrimSpace(apiKey),
		voice:  voice,
		url:    elevenLabsURL,
		client: &http.Client{
			Timeout: 25 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// NewElevenLabsGateway routes both audio operations through Bifrost.
func NewElevenLabsGateway(baseURL, apiKey, voice string) *ElevenLabs {
	if !elevenLabsVoiceID.MatchString(voice) {
		return nil
	}
	stt := New(baseURL, apiKey, "elevenlabs/scribe_v2")
	tts := aigateway.New(aigateway.Config{
		BaseURL: baseURL, APIKey: apiKey, Model: "elevenlabs/eleven_multilingual_v2",
		Timeout: 25 * time.Second, MaxResponse: elevenLabsAudioLimit + 1, ErrUpstream: ErrUpstream,
	})
	if stt == nil || tts == nil {
		return nil
	}
	return &ElevenLabs{voice: voice, stt: stt, tts: tts}
}

// Transcribe accepts the filename normalized by the upload handler.
func (c *ElevenLabs) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	if c.stt != nil {
		return c.stt.Transcribe(ctx, audio, filename)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("model_id", "scribe_v2"); err != nil {
		return "", err
	}
	if err := form.WriteField("tag_audio_events", "false"); err != nil {
		return "", err
	}
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}
	response, err := c.post(
		ctx,
		"/speech-to-text",
		form.FormDataContentType(),
		&body,
	)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, elevenLabsAudioLimit)).Decode(&result); err != nil {
		return "", fmt.Errorf("%w: decode ElevenLabs transcript: %w", ErrUpstream, err)
	}
	return strings.TrimSpace(result.Text), nil
}

func (c *ElevenLabs) Speak(ctx context.Context, text string) ([]byte, error) {
	if c.tts != nil {
		return c.speakThroughGateway(ctx, text)
	}
	body, err := json.Marshal(struct {
		Text  string `json:"text"`
		Model string `json:"model_id"`
	}{Text: text, Model: "eleven_multilingual_v2"})
	if err != nil {
		return nil, err
	}
	response, err := c.post(
		ctx,
		"/text-to-speech/"+c.voice+"?output_format=mp3_44100_128",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "audio/") {
		return nil, fmt.Errorf("%w: ElevenLabs returned a non-audio response", ErrUpstream)
	}
	audio, err := io.ReadAll(io.LimitReader(response.Body, elevenLabsAudioLimit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read ElevenLabs audio: %w", ErrUpstream, err)
	}
	if len(audio) == 0 || len(audio) > elevenLabsAudioLimit {
		return nil, fmt.Errorf("%w: unexpected ElevenLabs audio size", ErrUpstream)
	}
	return audio, nil
}

func (c *ElevenLabs) speakThroughGateway(ctx context.Context, text string) ([]byte, error) {
	body, err := json.Marshal(struct {
		Model  string `json:"model"`
		Voice  string `json:"voice"`
		Input  string `json:"input"`
		Format string `json:"response_format"`
	}{Model: c.tts.Model(), Voice: c.voice, Input: text, Format: "mp3"})
	if err != nil {
		return nil, err
	}
	audio, err := c.tts.Post(ctx, "/audio/speech", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(audio) < 3 || len(audio) > elevenLabsAudioLimit ||
		(!bytes.HasPrefix(audio, []byte("ID3")) && (audio[0] != 0xff || audio[1]&0xe0 != 0xe0)) {
		return nil, fmt.Errorf("%w: ElevenLabs did not return MP3 audio", ErrUpstream)
	}
	return audio, nil
}

func (c *ElevenLabs) post(
	ctx context.Context,
	path string,
	contentType string,
	body io.Reader,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.url+path,
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: build ElevenLabs request: %w", ErrUpstream, err)
	}
	request.Header.Set("xi-api-key", c.apiKey)
	request.Header.Set("Content-Type", contentType)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: send ElevenLabs request: %w", ErrUpstream, err)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("%w: ElevenLabs returned HTTP %d", ErrUpstream, response.StatusCode)
	}
	return response, nil
}
