package handler

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/strelov1/freehire/internal/identity/auth"
)

// The desktop client sends successive WAV segments. Bound their total cost per
// account independently of the daily allowance for short composer dictations.
func interviewTranscriptionLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        120,
		Expiration: time.Hour,
		KeyGenerator: func(c *fiber.Ctx) string {
			userID, _ := auth.UserID(c)
			return strconv.FormatInt(userID, 10)
		},
		LimitReached: func(_ *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusTooManyRequests, "interview transcription limit reached")
		},
	})
}

func (h *speechHandlers) postInterviewTranscription(c *fiber.Ctx) error {
	if h.stt == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "transcription is not configured")
	}
	if err := requirePaidCall(c.UserContext()); err != nil {
		return err
	}
	if err := refuseEncodedBody(c); err != nil {
		return err
	}
	audio, filename, err := readAudioUpload(c)
	if err != nil {
		return err
	}
	if filename != "dictation.wav" || !validInterviewAudio(audio) {
		return fiber.NewError(fiber.StatusBadRequest, "expected up to 60 seconds of mono 16 kHz PCM WAV")
	}
	text, err := h.stt.Transcribe(c.Context(), audio, filename)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "transcription failed")
	}
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return c.JSON(fiber.Map{"data": fiber.Map{"text": text}})
}

// The desktop client writes a canonical PCM WAV header. Checking the actual
// sample bytes bounds inference time even if the header overstates its length.
func validInterviewAudio(audio []byte) bool {
	const (
		headerBytes    = 44
		sampleRate     = 16000
		bytesPerSample = 2
		bytesPerSecond = sampleRate * bytesPerSample
		segmentSeconds = 60
	)
	size := len(audio) - headerBytes
	if size <= 0 || size > segmentSeconds*bytesPerSecond || size%bytesPerSample != 0 {
		return false
	}
	return bytes.Equal(audio[:4], []byte("RIFF")) &&
		binary.LittleEndian.Uint32(audio[4:]) == uint32(len(audio)-8) &&
		bytes.Equal(audio[8:16], []byte("WAVEfmt ")) &&
		binary.LittleEndian.Uint32(audio[16:]) == 16 &&
		binary.LittleEndian.Uint16(audio[20:]) == 1 &&
		binary.LittleEndian.Uint16(audio[22:]) == 1 &&
		binary.LittleEndian.Uint32(audio[24:]) == sampleRate &&
		binary.LittleEndian.Uint32(audio[28:]) == bytesPerSecond &&
		binary.LittleEndian.Uint16(audio[32:]) == bytesPerSample &&
		binary.LittleEndian.Uint16(audio[34:]) == 8*bytesPerSample &&
		bytes.Equal(audio[36:40], []byte("data")) &&
		binary.LittleEndian.Uint32(audio[40:]) == uint32(size)
}
