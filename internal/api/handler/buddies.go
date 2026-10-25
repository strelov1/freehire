package handler

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/engage/buddy"
)

// buddyHandlers serves the interview-buddy network (internal/engage/buddy): membership
// under a generated handle, similar members, the feed of other members' search progress,
// and one-to-one chat behind an invitation. Every route is cookie-only, so a leaked API
// key cannot read or write a member's conversations.
type buddyHandlers struct {
	buddy *buddy.Service
}

func newBuddyHandlers(svc *buddy.Service) *buddyHandlers {
	return &buddyHandlers{buddy: svc}
}

func (h *buddyHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/buddies", mw.cookie, h.Status)
	api.Post("/me/buddies", mw.cookie, h.Join)
	api.Delete("/me/buddies", mw.cookie, h.Leave)
	api.Get("/me/buddies/similar", mw.cookie, h.Similar)
	api.Get("/me/buddies/feed", mw.cookie, h.Feed)
	api.Get("/me/buddies/profiles/:handle", mw.cookie, h.Profile)
	api.Get("/me/buddies/hidden-applications", mw.cookie, h.HiddenApplications)
	api.Put("/me/buddies/hidden-applications/:id", mw.cookie, h.HideApplication)
	api.Delete("/me/buddies/hidden-applications/:id", mw.cookie, h.ShowApplication)
	api.Get("/me/buddies/conversations", mw.cookie, h.ListConversations)
	api.Post("/me/buddies/conversations", mw.cookie, h.Invite)
	api.Post("/me/buddies/conversations/:id/accept", mw.cookie, h.Accept)
	api.Post("/me/buddies/conversations/:id/decline", mw.cookie, h.Decline)
	api.Post("/me/buddies/conversations/:id/block", mw.cookie, h.Block)
	api.Post("/me/buddies/conversations/:id/report", mw.cookie, h.Report)
	api.Get("/me/buddies/conversations/:id/messages", mw.cookie, h.Messages)
	api.Post("/me/buddies/conversations/:id/messages", mw.cookie, h.Send)
	api.Post("/me/buddies/conversations/:id/read", mw.cookie, h.MarkRead)
	api.Get("/moderation/buddy-reports", mw.cookie, mw.moderator, h.ListReports)
	api.Post("/moderation/buddy-reports/:id/resolve", mw.cookie, mw.moderator, h.ResolveReport)
}

// buddyError renders a buddy sentinel as the status its declaration documents. Anything
// else passes through to the error renderer, which answers 500.
func buddyError(err error) error {
	switch {
	case errors.Is(err, buddy.ErrNotMember):
		return fiber.NewError(fiber.StatusForbidden, "join the buddy network first")
	case errors.Is(err, buddy.ErrRecipientUnavailable):
		return fiber.NewError(fiber.StatusForbidden, "this member cannot be invited")
	case errors.Is(err, buddy.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, buddy.ErrInvalidMessage):
		return fiber.NewError(fiber.StatusBadRequest, "message is empty or too long")
	case errors.Is(err, buddy.ErrInvalidReport):
		return fiber.NewError(fiber.StatusBadRequest, "invalid report reason or note")
	case errors.Is(err, buddy.ErrBadFeedRequest):
		return fiber.NewError(fiber.StatusBadRequest, "invalid feed mode or cursor")
	case errors.Is(err, buddy.ErrConversationExists):
		return fiber.NewError(fiber.StatusConflict, "a conversation with this member already exists")
	case errors.Is(err, buddy.ErrConversationState):
		return fiber.NewError(fiber.StatusConflict, "this conversation cannot be answered or written to")
	case errors.Is(err, buddy.ErrAlreadyReported):
		return fiber.NewError(fiber.StatusConflict, "you already reported this conversation")
	}
	return err
}

type buddyCardSummaryResponse struct {
	Category  string `json:"category"`
	Seniority string `json:"seniority"`
	Country   string `json:"country"`
}

func toBuddyCardSummary(c buddy.CardSummary) buddyCardSummaryResponse {
	return buddyCardSummaryResponse{Category: c.Category, Seniority: c.Seniority, Country: c.Country}
}

type buddyCompanyResponse struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type buddyStatusResponse struct {
	Member    bool   `json:"member"`
	Handle    string `json:"handle,omitempty"`
	CardEmpty bool   `json:"card_empty"`
}

func (h *buddyHandlers) statusResponse(c *fiber.Ctx, userID int64) error {
	st, err := h.buddy.Status(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": buddyStatusResponse{Member: st.Member, Handle: st.Handle, CardEmpty: st.EmptyCard}})
}

// Status reports the caller's standing. A member's visit also refreshes their ranking
// keys; no other route does, because the chat polls and a write per poll would be wrong.
func (h *buddyHandlers) Status(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.buddy.Touch(c.Context(), userID); err != nil && !errors.Is(err, buddy.ErrNotMember) {
		return err
	}
	return h.statusResponse(c, userID)
}

// Join opts the caller in, or restores a returning member under their old handle.
func (h *buddyHandlers) Join(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if _, err := h.buddy.Join(c.Context(), userID); err != nil {
		return err
	}
	return h.statusResponse(c, userID)
}

// Leave opts the caller out and closes their conversations.
func (h *buddyHandlers) Leave(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.buddy.Leave(c.Context(), userID); err != nil {
		return buddyError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddySimilarResponse struct {
	Handle          string                   `json:"handle"`
	CardSummary     buddyCardSummaryResponse `json:"card_summary"`
	Score           int                      `json:"score"`
	SharedCompanies []buddyCompanyResponse   `json:"shared_companies"`
}

// Similar lists the members most like the caller.
func (h *buddyHandlers) Similar(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	limit, offset, err := pageParamsWindowed(c, buddy.DefaultSimilarLimit, buddy.MaxSimilarLimit)
	if err != nil {
		return err
	}
	members, err := h.buddy.Similar(c.Context(), userID, limit, offset)
	if err != nil {
		return buddyError(err)
	}
	out := make([]buddySimilarResponse, len(members))
	for i, m := range members {
		shared := make([]buddyCompanyResponse, len(m.Shared))
		for j, co := range m.Shared {
			shared[j] = buddyCompanyResponse{Slug: co.Slug, Name: co.Name}
		}
		out[i] = buddySimilarResponse{Handle: m.Handle, CardSummary: toBuddyCardSummary(m.Card), Score: m.Score, SharedCompanies: shared}
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"limit": limit, "offset": offset}})
}

type buddyFeedEntryResponse struct {
	ID          int64                    `json:"id"`
	Handle      string                   `json:"handle"`
	CardSummary buddyCardSummaryResponse `json:"card_summary"`
	Company     buddyCompanyResponse     `json:"company"`
	JobTitle    *string                  `json:"job_title"`
	Type        string                   `json:"type"`
	Day         string                   `json:"day"`
}

// Feed reads one page of other members' search progress. With ?handle= it reads that one
// member's events instead (their page's activity), and mode is not read.
func (h *buddyHandlers) Feed(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var page buddy.FeedPage
	if handle := c.Query("handle"); handle != "" {
		page, err = h.buddy.MemberFeed(c.Context(), userID, handle, c.Query("cursor"), c.QueryInt("limit"))
	} else {
		mode := buddy.FeedMode(c.Query("mode", string(buddy.FeedSimilar)))
		page, err = h.buddy.Feed(c.Context(), userID, mode, c.Query("cursor"), c.QueryInt("limit"))
	}
	if err != nil {
		return buddyError(err)
	}
	out := make([]buddyFeedEntryResponse, len(page.Entries))
	for i, e := range page.Entries {
		out[i] = buddyFeedEntryResponse{
			ID: e.ID, Handle: e.Handle, CardSummary: toBuddyCardSummary(e.Card),
			Company:  buddyCompanyResponse{Slug: e.Company.Slug, Name: e.Company.Name},
			JobTitle: e.JobTitle, Type: e.Type, Day: e.Day,
		}
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"next_cursor": next}})
}

type buddyProfileResponse struct {
	Handle    string                      `json:"handle"`
	Card      talentnetwork.CandidateCard `json:"card"`
	CardEmpty bool                        `json:"card_empty"`
}

// Profile shows one member's anonymous card.
func (h *buddyHandlers) Profile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	p, err := h.buddy.Profile(c.Context(), userID, c.Params("handle"))
	if err != nil {
		return buddyError(err)
	}
	return c.JSON(fiber.Map{"data": buddyProfileResponse{Handle: p.Handle, Card: p.Card, CardEmpty: p.EmptyCard}})
}

// HiddenApplications lists the caller's applications kept out of the feed.
func (h *buddyHandlers) HiddenApplications(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	ids, err := h.buddy.HiddenApplications(c.Context(), userID)
	if err != nil {
		return buddyError(err)
	}
	if ids == nil {
		ids = []int64{}
	}
	return c.JSON(fiber.Map{"data": ids})
}

// HideApplication keeps one of the caller's applications out of the feed.
func (h *buddyHandlers) HideApplication(c *fiber.Ctx) error {
	return h.byID(c, h.buddy.HideApplication)
}

// ShowApplication lifts a hide.
func (h *buddyHandlers) ShowApplication(c *fiber.Ctx) error {
	return h.byID(c, h.buddy.ShowApplication)
}

// Block closes a conversation and hides the pair from each other.
func (h *buddyHandlers) Block(c *fiber.Ctx) error {
	return h.byID(c, h.buddy.Block)
}

// byID runs an action on the caller and the ":id" path param and answers 204.
func (h *buddyHandlers) byID(c *fiber.Ctx, do func(ctx context.Context, userID, id int64) error) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := do(c.Context(), userID, id); err != nil {
		return buddyError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddyOtherResponse struct {
	Handle      string                   `json:"handle"`
	CardSummary buddyCardSummaryResponse `json:"card_summary"`
}

// buddyConversationResponse carries unread and last_message_at meaningfully on the list
// only; a single conversation answered by invite or accept has 0 and null.
type buddyConversationResponse struct {
	ID            int64              `json:"id"`
	Other         buddyOtherResponse `json:"other"`
	FormerMember  bool               `json:"former_member"`
	StatusForMe   string             `json:"status_for_me"`
	Unread        int                `json:"unread"`
	LastMessageAt *time.Time         `json:"last_message_at"`
	CreatedAt     time.Time          `json:"created_at"`
}

func toBuddyConversation(cv buddy.Conversation) buddyConversationResponse {
	out := buddyConversationResponse{
		ID:           cv.ID,
		Other:        buddyOtherResponse{Handle: cv.OtherHandle, CardSummary: toBuddyCardSummary(cv.Card)},
		FormerMember: cv.FormerMember, StatusForMe: cv.StatusForMe, Unread: cv.Unread, CreatedAt: cv.CreatedAt,
	}
	if !cv.LastMessageAt.IsZero() {
		out.LastMessageAt = &cv.LastMessageAt
	}
	return out
}

// ListConversations lists the caller's conversations, newest message first.
func (h *buddyHandlers) ListConversations(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	convs, err := h.buddy.ListConversations(c.Context(), userID)
	if err != nil {
		return buddyError(err)
	}
	out := make([]buddyConversationResponse, len(convs))
	for i, cv := range convs {
		out[i] = toBuddyConversation(cv)
	}
	return c.JSON(fiber.Map{"data": out})
}

type buddyInviteBody struct {
	Handle string `json:"handle"`
	Body   string `json:"body"`
}

// Invite opens a pending conversation. Over the daily cap it answers 429 with the moment
// the next invitation becomes possible, in both the body and Retry-After.
func (h *buddyHandlers) Invite(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in buddyInviteBody
	if err := c.BodyParser(&in); err != nil || in.Handle == "" {
		return fiber.NewError(fiber.StatusBadRequest, "handle and body are required")
	}
	conv, err := h.buddy.Invite(c.Context(), userID, in.Handle, in.Body)
	var limit *buddy.InvitationLimitError
	if errors.As(err, &limit) {
		wait := max(int(math.Ceil(time.Until(limit.RetryAt).Seconds())), 1)
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(wait))
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error":    "invitation limit reached",
			"retry_at": limit.RetryAt.UTC().Format(time.RFC3339),
		})
	}
	if err != nil {
		return buddyError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toBuddyConversation(conv)})
}

// Accept opens a pending conversation the caller was invited to.
func (h *buddyHandlers) Accept(c *fiber.Ctx) error {
	return h.answer(c, h.buddy.Accept)
}

// Decline refuses a pending invitation.
func (h *buddyHandlers) Decline(c *fiber.Ctx) error {
	return h.answer(c, h.buddy.Decline)
}

func (h *buddyHandlers) answer(c *fiber.Ctx, do func(ctx context.Context, userID, id int64) (buddy.Conversation, error)) error {
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
		return buddyError(err)
	}
	return c.JSON(fiber.Map{"data": toBuddyConversation(conv)})
}

type buddyReportBody struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// Report files a complaint about a conversation the caller is part of.
func (h *buddyHandlers) Report(c *fiber.Ctx) error {
	var in buddyReportBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	return h.byID(c, func(ctx context.Context, userID, id int64) error {
		return h.buddy.Report(ctx, userID, id, in.Reason, in.Note)
	})
}

type buddyMessageResponse struct {
	ID        int64     `json:"id"`
	Mine      bool      `json:"mine"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func toBuddyMessage(m buddy.Message) buddyMessageResponse {
	return buddyMessageResponse{ID: m.ID, Mine: m.Mine, Body: m.Body, CreatedAt: m.CreatedAt}
}

// Messages reads a conversation's messages after the given id, oldest first.
func (h *buddyHandlers) Messages(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	msgs, err := h.buddy.Messages(c.Context(), userID, id, int64(c.QueryInt("after")), c.QueryInt("limit"))
	if err != nil {
		return buddyError(err)
	}
	out := make([]buddyMessageResponse, len(msgs))
	for i, m := range msgs {
		out[i] = toBuddyMessage(m)
	}
	return c.JSON(fiber.Map{"data": out})
}

type buddySendBody struct {
	Body string `json:"body"`
}

// Send posts a message into an open conversation.
func (h *buddyHandlers) Send(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in buddySendBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	msg, err := h.buddy.Send(c.Context(), userID, id, in.Body)
	if err != nil {
		return buddyError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toBuddyMessage(msg)})
}

type buddyReadBody struct {
	UpTo int64 `json:"up_to"`
}

// MarkRead records that the caller has read up to a message.
func (h *buddyHandlers) MarkRead(c *fiber.Ctx) error {
	var in buddyReadBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	return h.byID(c, func(ctx context.Context, userID, id int64) error {
		return h.buddy.MarkRead(ctx, userID, id, in.UpTo)
	})
}

type buddyReportMessageResponse struct {
	SenderHandle string    `json:"sender_handle"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
}

type buddyReportResponse struct {
	ID             int64                        `json:"id"`
	ConversationID int64                        `json:"conversation_id"`
	Reason         string                       `json:"reason"`
	Note           string                       `json:"note"`
	CreatedAt      time.Time                    `json:"created_at"`
	ResolvedAt     *time.Time                   `json:"resolved_at"`
	ReporterHandle string                       `json:"reporter_handle"`
	ReportedHandle string                       `json:"reported_handle"`
	Messages       []buddyReportMessageResponse `json:"messages"`
}

// ListReports is the moderator queue of reported conversations. Role-gated.
func (h *buddyHandlers) ListReports(c *fiber.Ctx) error {
	limit, offset, err := pageParamsWindowed(c, buddy.DefaultReportsLimit, buddy.MaxReportsLimit)
	if err != nil {
		return err
	}
	views, err := h.buddy.ListReports(c.Context(), limit, offset, c.QueryBool("include_resolved"))
	if err != nil {
		return err
	}
	out := make([]buddyReportResponse, len(views))
	for i, v := range views {
		msgs := make([]buddyReportMessageResponse, len(v.Messages))
		for j, m := range v.Messages {
			msgs[j] = buddyReportMessageResponse{SenderHandle: m.SenderHandle, Body: m.Body, CreatedAt: m.CreatedAt}
		}
		out[i] = buddyReportResponse{
			ID: v.ID, ConversationID: v.ConversationID, Reason: v.Reason, Note: v.Note,
			CreatedAt: v.CreatedAt, ResolvedAt: v.ResolvedAt,
			ReporterHandle: v.ReporterHandle, ReportedHandle: v.ReportedHandle, Messages: msgs,
		}
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"limit": limit, "offset": offset}})
}

// ResolveReport marks a report handled. Role-gated; idempotent; 404 for an unknown id.
func (h *buddyHandlers) ResolveReport(c *fiber.Ctx) error {
	return h.byID(c, h.buddy.ResolveReport)
}
