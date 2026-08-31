package handler

import (
	"context"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// transcriber is internal/ai/speech seen from here. An interface rather than the
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
	// plans meters a transcription against the caller's plan. Nil leaves dictation
	// unmetered, which is what a fixture gets; production always wires one.
	plans *plan.Store
}

func newSpeechHandlers(stt transcriber, plans *plan.Store) *speechHandlers {
	return &speechHandlers{stt: stt, plans: plans}
}

// transcriptionsPerHour bounds how many recordings one caller may transcribe per
// hour. It bounds the RATE; the daily dictation allowance bounds the volume, and the two
// are not interchangeable — a burst of sixty in a minute and sixty spread over a day are
// different problems. Sixty is far past anyone dictating into a chat and far below
// anything worth doing with a stolen session.
const transcriptionsPerHour = 60

// perCallerLimiter throttles per authenticated caller rather than per address — an IP
// key is lifted by any rotating proxy pool, and the cost being defended here is a
// metered upstream. Mounted AFTER the auth gate so the user id is resolved. Shared
// with voice mode's token-minting endpoint (assistant_interview_voice.go), which
// meters a different upstream behind the same per-caller shape.
func perCallerLimiter(max int) fiber.Handler {
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
	api.Post("/speech/transcriptions", mw.key, perCallerLimiter(transcriptionsPerHour), h.PostTranscription)
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
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	// Charged before the audio goes upstream, and given back below if nothing usable comes
	// out. The rate limiter above bounds how FAST a caller may ask; this bounds how much
	// they may have in a day — and the two answer differently on purpose, 429 against 402,
	// because one clears in seconds and the other clears tomorrow.
	charge, refused, err := h.chargeTranscription(c, userID)
	if refused {
		return err
	}

	text, err := h.stt.Transcribe(c.Context(), audio, filename)
	if err != nil {
		h.releaseTranscription(c, userID, charge)
		// Every failure from here is the gateway's: a refusal, a fault, an answer we
		// could not read. The caller did nothing wrong and has no remedy, so it is a
		// 502 rather than anything that blames them.
		return fiber.NewError(fiber.StatusBadGateway, "transcription failed")
	}
	// Silence transcribes to nothing, and nothing is not what the candidate was charged
	// for. The 200 stands — an empty result is a real answer, and the composer appends
	// nothing — but the allowance goes back.
	if strings.TrimSpace(text) == "" {
		h.releaseTranscription(c, userID, charge)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"text": text}})
}

// chargeTranscription takes one dictation allowance, reporting what it charged and — when
// the plan says no — the refusal to write.
//
// The reference is a fresh id rather than anything derived from the audio: two recordings
// of the same words are two dictations, and keying them alike would make the second free.
// That also means a retry after a 502 is charged again, which is correct — the release
// below has already given the first one back.
//
// Fails open, like every other meter on a request path: a counter that cannot be read logs
// and lets the recording through uncharged.
func (h *speechHandlers) chargeTranscription(c *fiber.Ctx, userID int64) (ref string, refused bool, err error) {
	if h.plans == nil {
		return "", false, nil
	}
	ref = uuid.NewString()
	d, err := h.plans.Consume(c.Context(), userID, plan.FeatureDictation, ref)
	switch {
	case err == nil:
		return ref, false, nil
	case isRefusal(err):
		return "", true, refuse(c, d)
	default:
		log.Printf("plan: charging a transcription for user %d: %v", userID, err)
		return "", false, nil
	}
}

// releaseTranscription gives the allowance back for a recording that produced nothing.
// Safe to call blind: an empty reference releases nothing.
//
// It runs on a DETACHED context, for the reason the assistant's release does: a client
// that walks away mid-upload cancels the request context, and a release on that same
// context could not open its own transaction — leaving the candidate charged for a
// transcription they never received, in exactly the case this exists for.
func (h *speechHandlers) releaseTranscription(_ *fiber.Ctx, userID int64, ref string) {
	if h.plans == nil || ref == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), turnReleaseTimeout)
	defer cancel()
	if err := h.plans.Release(ctx, userID, plan.FeatureDictation, ref); err != nil {
		log.Printf("plan: releasing a transcription for user %d: %v", userID, err)
	}
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
