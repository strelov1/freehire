package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/directapply"
)

func (h *employerHandlers) applicationMessageIDs(ctx context.Context, rawID string) (int64, int64, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return 0, 0, err
	}
	id, err := contractPathID(rawID)
	if err != nil {
		return 0, 0, err
	}
	if _, err := h.employer.ActiveAccount(ctx, userID); err != nil {
		return 0, 0, employerError(err)
	}
	return userID, id, nil
}

func employerApplicationMessageOut(m directapply.ApplicationMessage) apigen.EmployerApplicationMessage {
	return apigen.EmployerApplicationMessage{
		ID: m.ID, Body: m.Body, Status: apigen.EmployerApplicationMessageStatus(m.Status),
		CreatedAt: m.CreatedAt, RequestKey: optString(m.RequestKey),
	}
}

func (h *employerHandlers) GetEmployerApplicationConversation(ctx context.Context, params apigen.GetEmployerApplicationConversationParams) (*apigen.GetEmployerApplicationConversationOK, error) {
	userID, id, err := h.applicationMessageIDs(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	history, err := h.messages.History(ctx, userID, id, params.Before.Value)
	if err != nil {
		return nil, inboxError(err)
	}
	out := apigen.EmployerApplicationConversation{
		Messages: make([]apigen.EmployerApplicationMessage, 0, len(history.Messages)),
		HasMore:  history.HasMore,
		CanSend:  history.CanSend,
	}
	for _, msg := range history.Messages {
		out.Messages = append(out.Messages, employerApplicationMessageOut(msg))
	}
	return &apigen.GetEmployerApplicationConversationOK{Data: out}, nil
}

func (h *employerHandlers) SendEmployerApplicationMessage(ctx context.Context, req *apigen.EmployerApplicationMessageSend, params apigen.SendEmployerApplicationMessageParams) (*apigen.SendEmployerApplicationMessageOK, error) {
	userID, id, err := h.applicationMessageIDs(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	msg, err := h.messages.Send(ctx, userID, id, req.RequestKey, req.Body)
	switch {
	case errors.Is(err, directapply.ErrMessageUnavailable):
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	case errors.Is(err, directapply.ErrMessageConflict):
		return nil, fiber.NewError(fiber.StatusConflict, err.Error())
	case err != nil:
		return nil, inboxError(err)
	}
	return &apigen.SendEmployerApplicationMessageOK{Data: employerApplicationMessageOut(msg)}, nil
}

func (h *employerHandlers) MarkEmployerApplicationRepliesRead(ctx context.Context, req *apigen.EmployerApplicationRepliesRead, params apigen.MarkEmployerApplicationRepliesReadParams) (*apigen.MarkEmployerApplicationRepliesReadOK, error) {
	userID, id, err := h.applicationMessageIDs(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	unread, err := h.messages.MarkRead(ctx, userID, id, req.ThroughID)
	if err != nil {
		return nil, inboxError(err)
	}
	return &apigen.MarkEmployerApplicationRepliesReadOK{Data: apigen.EmployerApplicationReplyReadState{HasUnreadReply: unread}}, nil
}
