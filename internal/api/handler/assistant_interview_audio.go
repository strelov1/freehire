package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
)

type interviewAudioProvider interface {
	transcriber
	Speak(context.Context, string) ([]byte, error)
}

func (h *assistantHandlers) interviewAudioSession(ctx context.Context, id string) (assistant.Session, error) {
	session, err := h.ownedSessionByID(ctx, id)
	if err != nil {
		return assistant.Session{}, err
	}
	if session.Preset != assistant.PresetInterview || session.JobID == nil {
		return assistant.Session{}, fiber.NewError(fiber.StatusConflict, "interview audio needs a rehearsal")
	}
	if h.interviewAudio == nil {
		return assistant.Session{}, fiber.NewError(fiber.StatusNotImplemented, "interview audio is not configured")
	}
	return session, nil
}

func (h *assistantHandlers) PostInterviewTranscription(c *fiber.Ctx) error {
	if _, err := h.interviewAudioSession(c.Context(), c.Params("id")); err != nil {
		return err
	}
	return newSpeechHandlers(h.interviewAudio, h.plans).PostTranscription(c)
}

func (h *assistantHandlers) PostAssistantInterviewSpeech(
	ctx context.Context,
	params apigen.PostAssistantInterviewSpeechParams,
) (apigen.PostAssistantInterviewSpeechRes, error) {
	session, err := h.interviewAudioSession(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	if err := requirePaidCall(ctx); err != nil {
		return nil, err
	}

	messages, err := h.store.RecentTranscript(ctx, session.ID, 40)
	if err != nil {
		return nil, err
	}
	text := latestInterviewReply(messages)
	if text == "" {
		return nil, fiber.NewError(fiber.StatusConflict, "there is no interviewer reply to read")
	}

	charge := turnCharge{feature: plan.FeatureDictation}
	if h.plans != nil {
		charge.ref = uuid.NewString()
		decision, err := h.plans.Consume(
			ctx,
			session.UserID,
			charge.feature,
			charge.ref,
		)
		if isRefusal(err) {
			return decisionRefusal(decision).assistant()
		}
		if err != nil {
			return nil, err
		}
	}

	audio, err := h.interviewAudio.Speak(ctx, text)
	if err != nil {
		h.releaseTurn(session, charge)
		return nil, fiber.NewError(fiber.StatusBadGateway, "could not read the interviewer reply")
	}
	return &apigen.PostAssistantInterviewSpeechOKHeaders{
		CacheControl: "private, no-store",
		Response:     apigen.PostAssistantInterviewSpeechOK{Data: bytes.NewReader(audio)},
	}, nil
}

func latestInterviewReply(messages []assistant.Message) string {
	const maxRunes = 2_000
	for _, message := range slices.Backward(messages) {
		switch message.Role {
		case assistant.RoleUser:
			return ""
		case assistant.RoleAssistant:
			var content struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(message.Content, &content); err != nil {
				continue
			}
			text := strings.TrimSpace(content.Text)
			if text == "" {
				continue
			}
			runes := []rune(text)
			if len(runes) > maxRunes {
				return string(runes[:maxRunes])
			}
			return text
		}
	}
	return ""
}
