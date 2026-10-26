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
// and the invitation that starts a chat. The chat itself is the messaging core's
// (conversationHandlers). Every route is cookie-only, so a leaked API key cannot act for a
// member.
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
	api.Get("/me/buddies/profiles/:handle/activity", mw.cookie, h.Activity)
	api.Get("/me/buddies/hidden-applications", mw.cookie, h.HiddenApplications)
	api.Put("/me/buddies/hidden-applications/:id", mw.cookie, h.HideApplication)
	api.Delete("/me/buddies/hidden-applications/:id", mw.cookie, h.ShowApplication)
	api.Post("/me/buddies/invite", mw.cookie, h.Invite)
}

// buddyError renders a buddy sentinel as the status its declaration documents, and a
// messaging one as messagingError does. Anything else passes through to the error
// renderer, which answers 500.
func buddyError(err error) error {
	switch {
	case errors.Is(err, buddy.ErrNotMember):
		return fiber.NewError(fiber.StatusForbidden, "join the buddy network first")
	case errors.Is(err, buddy.ErrRecipientUnavailable):
		return fiber.NewError(fiber.StatusForbidden, "this member cannot be invited")
	case errors.Is(err, buddy.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, buddy.ErrBadFeedRequest):
		return fiber.NewError(fiber.StatusBadRequest, "invalid feed mode or cursor")
	}
	return messagingError(err)
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

type buddyActivityDayResponse struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// Activity is one member's feed events per day over the last year, for their page's grid.
func (h *buddyHandlers) Activity(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	days, err := h.buddy.MemberActivity(c.Context(), userID, c.Params("handle"))
	if err != nil {
		return buddyError(err)
	}
	out := make([]buddyActivityDayResponse, len(days))
	for i, d := range days {
		out[i] = buddyActivityDayResponse{Day: d.Day, Count: d.Count}
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"days": out}})
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

type buddyInviteBody struct {
	Handle string `json:"handle"`
	Body   string `json:"body"`
}

// Invite starts a pending buddy conversation. Over the daily cap it answers 429 with the moment
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
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toConversation(conv)})
}
