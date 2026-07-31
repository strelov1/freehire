package handler

import (
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/strelov1/freehire/internal/auth"
)

// transcriber is internal/speech seen from here. An interface rather than the
// concrete client so the handler's tests can assert that a refused request never
// reaches the metered upstream — which is the point of most of them.
type transcriber interface {
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
}

// speechHandlers serve dictation: one recording in, its text out. There is no
// storage and no state — the audio is read, forwarded, and dropped.
type speechHandlers struct {
	// stt is nil where no speech gateway is configured, and every request then
	// answers 501. Whoever wires this must pass an untyped nil rather than a nil
	// *speech.Client: a nil pointer in an interface is not a nil interface, and that
	// mistake turns "the feature is absent" into a panic on the first recording.
	stt transcriber
}

func newSpeechHandlers(stt transcriber) *speechHandlers {
	return &speechHandlers{stt: stt}
}

// transcriptionsPerHour bounds how many recordings one caller may transcribe per
// hour. Transcription is billed per minute of audio and the assistant is not metered,
// so this limit and the upload cap are the only things standing between a script and
// our bill. Sixty is far past anyone dictating into a chat and far below anything
// worth doing with a stolen session.
const transcriptionsPerHour = 60

// transcriptionLimiter throttles per authenticated caller rather than per address —
// an IP key is lifted by any rotating proxy pool, and the cost being defended here is
// a metered upstream. Mounted AFTER the auth gate so the user id is resolved.
func transcriptionLimiter(max int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: time.Hour,
		KeyGenerator: func(c *fiber.Ctx) string {
			if id, ok := auth.UserID(c); ok {
				return "user:" + strconv.FormatInt(id, 10)
			}
			return "ip:" + c.IP()
		},
	})
}

func (h *speechHandlers) register(api fiber.Router, mw middleware) {
	// The same gate as POST /assistant/sessions/:id/messages. The microphone lives in
	// the composer that posts messages, so a client allowed to send one is allowed to
	// transcribe for it; a narrower rule here would leave the control dead in the
	// extension's side panel for no gain.
	api.Post("/speech/transcriptions", mw.key, transcriptionLimiter(transcriptionsPerHour), h.PostTranscription)
}

// maxAudioUpload bounds one recording. At the ~32 kbit/s the browser's opus encoder
// produces this is around eight minutes of speech — well past a dictated message and
// well under the server's global body limit, which is what actually stops anything
// larger (fasthttp has read the whole body by the time FormFile runs). The client
// stops recording sooner still, so this is the backstop rather than the common path.
const maxAudioUpload = 2 << 20

// PostTranscription turns one uploaded recording into text.
//
// Nothing is stored and nothing is sent: the caller gets the words back and decides
// what to do with them. An empty result is a 200 — that is what silence transcribes
// to, and the composer appends nothing.
func (h *speechHandlers) PostTranscription(c *fiber.Ctx) error {
	if h.stt == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "transcription is not configured")
	}
	audio, filename, err := readAudioUpload(c)
	if err != nil {
		return err
	}
	text, err := h.stt.Transcribe(c.Context(), audio, filename)
	if err != nil {
		// Every failure from here is the gateway's: a refusal, a fault, an answer we
		// could not read. The caller did nothing wrong and has no remedy, so it is a
		// 502 rather than anything that blames them.
		return fiber.NewError(fiber.StatusBadGateway, "transcription failed")
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"text": text}})
}

// audioContainers is what a browser's MediaRecorder actually emits: webm/opus in
// Chrome and Firefox, mp4 or m4a in Safari, ogg on some Linux builds. The list is an
// allowlist rather than a hint — see safeAudioFilename.
var audioContainers = map[string]bool{
	"webm": true,
	"mp4":  true,
	"m4a":  true,
	"ogg":  true,
	"wav":  true,
	"mp3":  true,
}

// safeAudioFilename derives the name to forward from the one the client sent.
//
// Only the extension survives, and only from a fixed list. The gateway identifies the
// container by extension, so that much has to travel — but the rest of the client's
// string must not, because Go's multipart writer escapes quotes in a filename and NOT
// CRLF: a crafted name would inject header lines into the request this server makes.
// Rebuilding the name from an allowlisted extension closes that without needing to
// reason about what else a filename might carry.
func safeAudioFilename(name string) (string, bool) {
	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return "", false
	}
	ext := strings.ToLower(name[dot+1:])
	if !audioContainers[ext] {
		return "", false
	}
	return "dictation." + ext, true
}

// readAudioUpload reads the "file" part into memory, along with a filename safe to
// forward.
func readAudioUpload(c *fiber.Ctx) ([]byte, string, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "missing audio file")
	}
	filename, ok := safeAudioFilename(fh.Filename)
	if !ok {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "unsupported audio format")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "cannot read audio file")
	}
	defer f.Close()
	// Capped on the read, not on the part's declared size: that number is the client's
	// claim, not a promise about what the stream delivers.
	audio, err := io.ReadAll(io.LimitReader(f, maxAudioUpload+1))
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "cannot read audio file")
	}
	if len(audio) > maxAudioUpload {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "recording is too long")
	}
	// A recording with nothing in it is a client bug, and spending a metered call to
	// have the gateway tell us so would answer 502 for something we can see here.
	if len(audio) == 0 {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "the recording is empty")
	}
	return audio, filename, nil
}
