package handler

import (
	"context"
	"io"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// transcriber is internal/ai/speech seen from here. An interface so the handler's tests
// can assert that a refused request never reaches the metered upstream.
type transcriber interface {
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
}

// speechHandlers serve dictation and interview transcription. There is no
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

func (h *speechHandlers) register(api fiber.Router, mw middleware) {
	// The same gate as POST /assistant/sessions/:id/messages: the microphone lives in the
	// composer that posts messages, so a client allowed to send one may transcribe for it (a
	// narrower rule would leave the control dead in the extension's side panel).
	api.Post("/speech/transcriptions", mw.key, mw.paidAI(plan.FeatureDictation), h.PostTranscription)
	api.Post("/me/interviews/transcriptions", mw.key, mw.paidAI(unmeteredAI), interviewTranscriptionLimit(), h.postInterviewTranscription)
}

// maxAudioUpload bounds one recording: around eight minutes at the ~32 kbit/s the
// browser's opus encoder produces. The server's global body limit is what actually stops
// larger ones (fasthttp has read the whole body by the time FormFile runs); the client
// stops sooner still, so this is a backstop.
const maxAudioUpload = 2 << 20

// PostTranscription turns one uploaded recording into text. Nothing is stored. An empty
// result is a 200: that is what silence transcribes to, and the composer appends nothing.
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
	// A transcription is a model call that bypasses the user-call binding, so it makes the
	// binding's check itself: a route reaching here without the paid gate is our bug and must
	// not send audio upstream on anybody's account.
	if err := requirePaidCall(c.UserContext()); err != nil {
		return err
	}
	// Charged before the audio goes upstream, and given back below if nothing usable comes
	// out. This charge bounds how much the caller may transcribe in a day.
	charge, refused, err := h.chargeTranscription(c, userID)
	if refused {
		return err
	}

	text, err := h.stt.Transcribe(c.Context(), audio, filename)
	if err != nil {
		h.releaseTranscription(c, userID, charge)
		// Every failure from here is the gateway's (refusal, fault, unreadable answer), so a 502
		// rather than anything blaming the caller.
		return fiber.NewError(fiber.StatusBadGateway, "transcription failed")
	}
	// Silence transcribes to nothing, and nothing is not what the candidate was charged for:
	// the 200 stands, but the allowance goes back.
	if strings.TrimSpace(text) == "" {
		h.releaseTranscription(c, userID, charge)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"text": text}})
}

// chargeTranscription takes one dictation allowance, reporting what it charged and, when
// the plan says no, the refusal to write.
//
// The reference is a fresh id, not derived from the audio: two recordings of the same
// words are two dictations, and keying them alike would make the second free. A retry
// after a 502 is therefore charged again, which is correct since the release has returned
// the first.
//
// Fails open like every other request-path meter: an unreadable counter logs and lets the
// recording through uncharged.
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
// It runs on a DETACHED context, as the assistant's release does: a client that walks away
// mid-upload cancels the request context, and a release on it could not open its own
// transaction, leaving the candidate charged for nothing.
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

// safeAudioFilename derives the name to forward from the one the client sent. Only the
// extension survives, and only from a fixed list: the gateway identifies the container by
// extension, but Go's multipart writer escapes quotes in a filename and NOT CRLF, so a
// crafted name would inject header lines into the upstream request.
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
	// Capped on the read, not on the part's declared size: that is the client's claim, not a
	// promise about what the stream delivers.
	audio, err := io.ReadAll(io.LimitReader(f, maxAudioUpload+1))
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "cannot read audio file")
	}
	if len(audio) > maxAudioUpload {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "recording is too long")
	}
	// A recording with nothing in it is a client bug; spending a metered call to have the
	// gateway say so would answer 502 for something visible here.
	if len(audio) == 0 {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "the recording is empty")
	}
	return audio, filename, nil
}
