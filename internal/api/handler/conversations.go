package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/messaging"
)

// conversationHandlers serves the messaging core (internal/engage/messaging): every
// conversation of every kind, its messages, read state, blocks and reports. Starting one is
// each kind's own route (a buddy invitation is POST /me/buddies/invite), because who may
// start a conversation is the kind's rule. Every route is cookie-only, so a leaked API key
// cannot read or write a member's conversations.
type conversationHandlers struct {
	msg   *messaging.Service
	mocks *buddy.MockService
}

func newConversationHandlers(msg *messaging.Service, mocks *buddy.MockService) *conversationHandlers {
	return &conversationHandlers{msg: msg, mocks: mocks}
}

func (h *conversationHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/conversations", mw.cookie, contract)
	api.Post("/me/conversations/:id/accept", mw.cookie, contract)
	api.Post("/me/conversations/:id/decline", mw.cookie, contract)
	api.Post("/me/conversations/:id/block", mw.cookie, contract)
	api.Post("/me/conversations/:id/report", mw.cookie, contract)
	api.Get("/me/conversations/:id/messages", mw.cookie, contract)
	api.Post("/me/conversations/:id/messages", mw.cookie, contract)
	api.Post("/me/conversations/:id/read", mw.cookie, contract)
	api.Get("/moderation/conversation-reports", mw.cookie, mw.moderator, contract)
	api.Post("/moderation/conversation-reports/:id/resolve", mw.cookie, mw.moderator, contract)
}

// messagingError renders a messaging sentinel as the status its declaration documents.
// Anything else passes through to the error renderer, which answers 500.
func messagingError(err error) error {
	switch {
	case errors.Is(err, messaging.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, messaging.ErrInvalidMessage):
		return fiber.NewError(fiber.StatusBadRequest, "message is empty or too long")
	case errors.Is(err, messaging.ErrInvalidReport):
		return fiber.NewError(fiber.StatusBadRequest, "invalid report reason or note")
	case errors.Is(err, messaging.ErrUnavailable):
		return fiber.NewError(fiber.StatusForbidden, "this conversation cannot be started")
	case errors.Is(err, messaging.ErrExists):
		return fiber.NewError(fiber.StatusConflict, "a conversation already exists")
	case errors.Is(err, messaging.ErrState):
		return fiber.NewError(fiber.StatusConflict, "this conversation cannot be answered or written to")
	case errors.Is(err, messaging.ErrAlreadyReported):
		return fiber.NewError(fiber.StatusConflict, "you already reported this conversation")
	}
	return err
}

// toContractConversation is a conversation in its wire shape.
func toContractConversation(cv messaging.Conversation) apigen.Conversation {
	out := apigen.Conversation{
		ID: cv.ID, Kind: apigen.ConversationKind(cv.Kind), StatusForMe: apigen.ConversationStatusForMe(cv.StatusForMe), Unread: int64(cv.Unread), CreatedAt: cv.CreatedAt,
		Counterpart: apigen.ConversationCounterpart{Persona: apigen.ConversationCounterpartPersona(cv.Counterpart.Persona), Reachable: cv.Counterpart.Reachable},
	}
	out.Counterpart.Detail.SetToNull()
	if d, ok := cv.Counterpart.Detail.(buddy.Counterpart); ok {
		out.Counterpart.Detail.SetTo(apigen.BuddyCounterpart{Handle: d.Handle, CardSummary: buddyCardSummaryOut(d.Card)})
	}
	if cv.LastMessageAt.IsZero() {
		out.LastMessageAt.SetToNull()
	} else {
		out.LastMessageAt.SetTo(cv.LastMessageAt)
	}
	return out
}

// ListConversations lists the caller's conversations of every kind, newest message first.
func (h *conversationHandlers) ListConversations(ctx context.Context) (*apigen.ListConversationsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convs, err := h.msg.List(ctx, userID)
	if err != nil {
		return nil, messagingError(err)
	}
	out := make([]apigen.Conversation, len(convs))
	for i, cv := range convs {
		out[i] = toContractConversation(cv)
	}
	return &apigen.ListConversationsOK{Data: out}, nil
}

// AcceptConversation opens a pending conversation the caller did not start.
func (h *conversationHandlers) AcceptConversation(ctx context.Context, params apigen.AcceptConversationParams) (*apigen.AcceptConversationOK, error) {
	conv, err := h.answer(ctx, params.ID, h.msg.Accept)
	if err != nil {
		return nil, err
	}
	return &apigen.AcceptConversationOK{Data: conv}, nil
}

// DeclineConversation refuses a pending conversation.
func (h *conversationHandlers) DeclineConversation(ctx context.Context, params apigen.DeclineConversationParams) (*apigen.DeclineConversationOK, error) {
	conv, err := h.answer(ctx, params.ID, h.msg.Decline)
	if err != nil {
		return nil, err
	}
	return &apigen.DeclineConversationOK{Data: conv}, nil
}

func (h *conversationHandlers) answer(ctx context.Context, rawID string, do func(ctx context.Context, userID, id int64) (messaging.Conversation, error)) (apigen.Conversation, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return apigen.Conversation{}, err
	}
	id, err := contractPathID(rawID)
	if err != nil {
		return apigen.Conversation{}, err
	}
	conv, err := do(ctx, userID, id)
	if err != nil {
		return apigen.Conversation{}, messagingError(err)
	}
	return toContractConversation(conv), nil
}

// BlockConversation closes every conversation with the other participant and stops either
// from starting another, of any kind.
//
// A block also cancels every future buddy mock between the two, inside the block's own
// statement (a trigger the buddy migration puts on user_blocks, because the core never
// imports a feature). This layer, the only one that sees both, tells the other side.
func (h *conversationHandlers) BlockConversation(ctx context.Context, params apigen.BlockConversationParams) error {
	return h.byID(ctx, params.ID, func(ctx context.Context, userID, id int64) error {
		if err := h.msg.Block(ctx, userID, id); err != nil {
			return err
		}
		h.mocks.AnnounceCancellations(ctx, userID)
		return nil
	})
}

// byID runs an action on the caller and the ":id" path param.
func (h *conversationHandlers) byID(ctx context.Context, rawID string, do func(ctx context.Context, userID, id int64) error) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(rawID)
	if err != nil {
		return err
	}
	if err := do(ctx, userID, id); err != nil {
		return messagingError(err)
	}
	return nil
}

// ReportConversation files a complaint about a conversation the caller takes part in.
func (h *conversationHandlers) ReportConversation(ctx context.Context, req *apigen.ConversationReportRequest, params apigen.ReportConversationParams) error {
	return h.byID(ctx, params.ID, func(ctx context.Context, userID, id int64) error {
		return h.msg.Report(ctx, userID, id, req.Reason.Value, req.Note.Value)
	})
}

func toContractMessage(m messaging.Message) apigen.ConversationMessage {
	return apigen.ConversationMessage{ID: m.ID, Mine: m.Mine, Body: m.Body, CreatedAt: m.CreatedAt}
}

// ListConversationMessages reads a conversation's messages after the given id, oldest first.
func (h *conversationHandlers) ListConversationMessages(ctx context.Context, params apigen.ListConversationMessagesParams) (*apigen.ListConversationMessagesOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	query := contractQuery(ctx)
	msgs, err := h.msg.Messages(ctx, userID, id, int64(queryIntOr(query, "after", 0)), queryIntOr(query, "limit", 0))
	if err != nil {
		return nil, messagingError(err)
	}
	out := make([]apigen.ConversationMessage, len(msgs))
	for i, m := range msgs {
		out[i] = toContractMessage(m)
	}
	return &apigen.ListConversationMessagesOK{Data: out}, nil
}

// SendConversationMessage posts a message into an open conversation.
func (h *conversationHandlers) SendConversationMessage(ctx context.Context, req *apigen.ConversationSendRequest, params apigen.SendConversationMessageParams) (*apigen.SendConversationMessageCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	msg, err := h.msg.Send(ctx, userID, id, req.Body.Value)
	if err != nil {
		return nil, messagingError(err)
	}
	return &apigen.SendConversationMessageCreated{Data: toContractMessage(msg)}, nil
}

// MarkConversationRead records that the caller has read up to a message.
func (h *conversationHandlers) MarkConversationRead(ctx context.Context, req *apigen.ConversationReadRequest, params apigen.MarkConversationReadParams) error {
	return h.byID(ctx, params.ID, func(ctx context.Context, userID, id int64) error {
		return h.msg.MarkRead(ctx, userID, id, req.UpTo.Value)
	})
}

// ListConversationReports is the moderator queue of reported conversations. Role-gated.
func (h *conversationHandlers) ListConversationReports(ctx context.Context, _ apigen.ListConversationReportsParams) (*apigen.ConversationReportsPage, error) {
	limit, offset, err := contractPageWindowed(ctx, messaging.DefaultReportsLimit, messaging.MaxReportsLimit)
	if err != nil {
		return nil, err
	}
	resolved, _ := strconv.ParseBool(contractQuery(ctx).Get("include_resolved"))
	reports, err := h.msg.ListReports(ctx, limit, offset, resolved)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.ConversationReport, len(reports))
	for i, r := range reports {
		msgs := make([]apigen.ConversationReportMessage, len(r.Messages))
		for j, m := range r.Messages {
			msgs[j] = apigen.ConversationReportMessage{Sender: m.Sender, Body: m.Body, CreatedAt: m.CreatedAt}
		}
		out[i] = apigen.ConversationReport{
			ID: r.ID, ConversationID: r.ConversationID, Kind: apigen.ConversationReportKind(r.Kind), Reason: apigen.ConversationReportReason(r.Reason), Note: r.Note,
			CreatedAt: r.CreatedAt, Reporter: r.Reporter, Reported: r.Reported, Messages: msgs,
		}
		if r.ResolvedAt != nil {
			out[i].ResolvedAt.SetTo(*r.ResolvedAt)
		} else {
			out[i].ResolvedAt.SetToNull()
		}
	}
	return &apigen.ConversationReportsPage{Data: out, Meta: apigen.ConversationReportsMeta{Limit: int64(limit), Offset: int64(offset)}}, nil
}

// ResolveConversationReport marks a report handled. Role-gated; idempotent; 404 for an unknown id.
func (h *conversationHandlers) ResolveConversationReport(ctx context.Context, params apigen.ResolveConversationReportParams) error {
	return h.byID(ctx, params.ID, h.msg.ResolveReport)
}
