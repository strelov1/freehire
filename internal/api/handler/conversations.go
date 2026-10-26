package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/messaging"
)

// conversationHandlers serves the messaging core (internal/engage/messaging): every
// conversation of every kind, its messages, read state, blocks and reports. Starting one is
// each kind's own route (a buddy invitation is POST /me/buddies/invite), because who may
// start a conversation is the kind's rule. Every route is cookie-only, so a leaked API key
// cannot read or write a member's conversations.
type conversationHandlers struct {
	msg *messaging.Service
}

func newConversationHandlers(msg *messaging.Service) *conversationHandlers {
	return &conversationHandlers{msg: msg}
}

func (h *conversationHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/conversations", mw.cookie, h.List)
	api.Post("/me/conversations/:id/accept", mw.cookie, h.Accept)
	api.Post("/me/conversations/:id/decline", mw.cookie, h.Decline)
	api.Post("/me/conversations/:id/block", mw.cookie, h.Block)
	api.Post("/me/conversations/:id/report", mw.cookie, h.Report)
	api.Get("/me/conversations/:id/messages", mw.cookie, h.Messages)
	api.Post("/me/conversations/:id/messages", mw.cookie, h.Send)
	api.Post("/me/conversations/:id/read", mw.cookie, h.MarkRead)
	api.Get("/moderation/conversation-reports", mw.cookie, mw.moderator, h.ListReports)
	api.Post("/moderation/conversation-reports/:id/resolve", mw.cookie, mw.moderator, h.ResolveReport)
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

// conversationCounterpartResponse is the other side of a conversation, as the persona they
// speak as draws them. detail is that persona's own shape, told apart by persona, and null
// when reachable is false. Nothing here names the account behind the persona.
type conversationCounterpartResponse struct {
	Persona   string `json:"persona"`
	Reachable bool   `json:"reachable"`
	Detail    any    `json:"detail"`
}

type buddyCounterpartResponse struct {
	Handle      string                   `json:"handle"`
	CardSummary buddyCardSummaryResponse `json:"card_summary"`
}

func toConversationCounterpart(cp messaging.Counterpart) conversationCounterpartResponse {
	out := conversationCounterpartResponse{Persona: cp.Persona, Reachable: cp.Reachable}
	switch d := cp.Detail.(type) {
	case buddy.Counterpart:
		out.Detail = buddyCounterpartResponse{Handle: d.Handle, CardSummary: toBuddyCardSummary(d.Card)}
	}
	return out
}

// conversationResponse carries unread and last_message_at meaningfully on the list only; a
// single conversation answered by a start or an answer has 0 and null.
type conversationResponse struct {
	ID            int64                           `json:"id"`
	Kind          string                          `json:"kind"`
	StatusForMe   string                          `json:"status_for_me"`
	Counterpart   conversationCounterpartResponse `json:"counterpart"`
	Unread        int                             `json:"unread"`
	LastMessageAt *time.Time                      `json:"last_message_at"`
	CreatedAt     time.Time                       `json:"created_at"`
}

func toConversation(cv messaging.Conversation) conversationResponse {
	out := conversationResponse{
		ID: cv.ID, Kind: cv.Kind, StatusForMe: cv.StatusForMe, Counterpart: toConversationCounterpart(cv.Counterpart),
		Unread: cv.Unread, CreatedAt: cv.CreatedAt,
	}
	if !cv.LastMessageAt.IsZero() {
		out.LastMessageAt = &cv.LastMessageAt
	}
	return out
}

// List lists the caller's conversations of every kind, newest message first.
func (h *conversationHandlers) List(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	convs, err := h.msg.List(c.Context(), userID)
	if err != nil {
		return messagingError(err)
	}
	out := make([]conversationResponse, len(convs))
	for i, cv := range convs {
		out[i] = toConversation(cv)
	}
	return c.JSON(fiber.Map{"data": out})
}

// Accept opens a pending conversation the caller did not start.
func (h *conversationHandlers) Accept(c *fiber.Ctx) error {
	return h.answer(c, h.msg.Accept)
}

// Decline refuses a pending conversation.
func (h *conversationHandlers) Decline(c *fiber.Ctx) error {
	return h.answer(c, h.msg.Decline)
}

func (h *conversationHandlers) answer(c *fiber.Ctx, do func(ctx context.Context, userID, id int64) (messaging.Conversation, error)) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	conv, err := do(c.Context(), userID, id)
	if err != nil {
		return messagingError(err)
	}
	return c.JSON(fiber.Map{"data": toConversation(conv)})
}

// Block closes every conversation with the other participant and stops either from
// starting another, of any kind.
func (h *conversationHandlers) Block(c *fiber.Ctx) error {
	return h.byID(c, h.msg.Block)
}

// byID runs an action on the caller and the ":id" path param and answers 204.
func (h *conversationHandlers) byID(c *fiber.Ctx, do func(ctx context.Context, userID, id int64) error) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := do(c.Context(), userID, id); err != nil {
		return messagingError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type conversationReportBody struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// Report files a complaint about a conversation the caller takes part in.
func (h *conversationHandlers) Report(c *fiber.Ctx) error {
	var in conversationReportBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	return h.byID(c, func(ctx context.Context, userID, id int64) error {
		return h.msg.Report(ctx, userID, id, in.Reason, in.Note)
	})
}

type conversationMessageResponse struct {
	ID        int64     `json:"id"`
	Mine      bool      `json:"mine"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func toConversationMessage(m messaging.Message) conversationMessageResponse {
	return conversationMessageResponse{ID: m.ID, Mine: m.Mine, Body: m.Body, CreatedAt: m.CreatedAt}
}

// Messages reads a conversation's messages after the given id, oldest first.
func (h *conversationHandlers) Messages(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	msgs, err := h.msg.Messages(c.Context(), userID, id, int64(c.QueryInt("after")), c.QueryInt("limit"))
	if err != nil {
		return messagingError(err)
	}
	out := make([]conversationMessageResponse, len(msgs))
	for i, m := range msgs {
		out[i] = toConversationMessage(m)
	}
	return c.JSON(fiber.Map{"data": out})
}

type conversationSendBody struct {
	Body string `json:"body"`
}

// Send posts a message into an open conversation.
func (h *conversationHandlers) Send(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in conversationSendBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	msg, err := h.msg.Send(c.Context(), userID, id, in.Body)
	if err != nil {
		return messagingError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toConversationMessage(msg)})
}

type conversationReadBody struct {
	UpTo int64 `json:"up_to"`
}

// MarkRead records that the caller has read up to a message.
func (h *conversationHandlers) MarkRead(c *fiber.Ctx) error {
	var in conversationReadBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	return h.byID(c, func(ctx context.Context, userID, id int64) error {
		return h.msg.MarkRead(ctx, userID, id, in.UpTo)
	})
}

type conversationReportMessageResponse struct {
	Sender    string    `json:"sender"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// conversationReportResponse names each account the way the conversation's kind draws it
// (for buddy, by handle): a moderator reads what the participants saw.
type conversationReportResponse struct {
	ID             int64                               `json:"id"`
	ConversationID int64                               `json:"conversation_id"`
	Kind           string                              `json:"kind"`
	Reason         string                              `json:"reason"`
	Note           string                              `json:"note"`
	CreatedAt      time.Time                           `json:"created_at"`
	ResolvedAt     *time.Time                          `json:"resolved_at"`
	Reporter       string                              `json:"reporter"`
	Reported       string                              `json:"reported"`
	Messages       []conversationReportMessageResponse `json:"messages"`
}

// ListReports is the moderator queue of reported conversations. Role-gated.
func (h *conversationHandlers) ListReports(c *fiber.Ctx) error {
	limit, offset, err := pageParamsWindowed(c, messaging.DefaultReportsLimit, messaging.MaxReportsLimit)
	if err != nil {
		return err
	}
	reports, err := h.msg.ListReports(c.Context(), limit, offset, c.QueryBool("include_resolved"))
	if err != nil {
		return err
	}
	out := make([]conversationReportResponse, len(reports))
	for i, r := range reports {
		msgs := make([]conversationReportMessageResponse, len(r.Messages))
		for j, m := range r.Messages {
			msgs[j] = conversationReportMessageResponse{Sender: m.Sender, Body: m.Body, CreatedAt: m.CreatedAt}
		}
		out[i] = conversationReportResponse{
			ID: r.ID, ConversationID: r.ConversationID, Kind: r.Kind, Reason: r.Reason, Note: r.Note,
			CreatedAt: r.CreatedAt, ResolvedAt: r.ResolvedAt,
			Reporter: r.Reporter, Reported: r.Reported, Messages: msgs,
		}
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"limit": limit, "offset": offset}})
}

// ResolveReport marks a report handled. Role-gated; idempotent; 404 for an unknown id.
func (h *conversationHandlers) ResolveReport(c *fiber.Ctx) error {
	return h.byID(c, h.msg.ResolveReport)
}
