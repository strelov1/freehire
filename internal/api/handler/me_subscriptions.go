package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/subscription"
	"github.com/strelov1/freehire/internal/platform/db"
)

// subscriptionHandlers serves the per-user filter subscriptions: subscribe a saved
// search to a channel, list/toggle/unsubscribe. The use cases live in
// subscription.Service.
type subscriptionHandlers struct {
	subscription *subscription.Service
}

func newSubscriptionHandlers(queries *db.Queries) *subscriptionHandlers {
	return &subscriptionHandlers{subscription: subscription.New(subscription.NewQueriesRepository(queries))}
}

func (h *subscriptionHandlers) register(api fiber.Router, mw middleware) {
	// Filter subscriptions are cookie-only (RequireAuth) like saved searches: a
	// browser convenience, owner-scoped (a non-owned id is 404).
	api.Get("/me/subscriptions", mw.cookie, h.ListSubscriptions)
	api.Post("/me/subscriptions", mw.cookie, h.CreateSubscription)
	api.Patch("/me/subscriptions/:id", mw.cookie, h.SetSubscriptionActive)
	api.Delete("/me/subscriptions/:id", mw.cookie, h.DeleteSubscription)
}

// subscriptionResponse is the public shape of a subscription. user_id and the
// internal start_at cursor are omitted; saved_search_name is included on list so
// the SPA can label each toggle (empty/omitted on create/patch confirmations).
type subscriptionResponse struct {
	ID              int64      `json:"id"`
	SavedSearchID   int64      `json:"saved_search_id"`
	SavedSearchName string     `json:"saved_search_name,omitempty"`
	Channel         string     `json:"channel"`
	Active          bool       `json:"active"`
	CreatedAt       *time.Time `json:"created_at"`
}

func toSubscriptionResponse(s subscription.Subscription) subscriptionResponse {
	return subscriptionResponse{
		ID:            s.ID,
		SavedSearchID: s.SavedSearchID,
		Channel:       s.Channel,
		Active:        s.Active,
		CreatedAt:     s.CreatedAt,
	}
}

func toSubscriptionListItem(s subscription.SubscriptionListItem) subscriptionResponse {
	return subscriptionResponse{
		ID:              s.ID,
		SavedSearchID:   s.SavedSearchID,
		SavedSearchName: s.SavedSearchName,
		Channel:         s.Channel,
		Active:          s.Active,
		CreatedAt:       s.CreatedAt,
	}
}

// subscriptionError maps the subscription sentinels onto HTTP statuses: an
// unsupported channel is a 400, a missing/non-owned saved search or subscription
// is a 404, a duplicate is a 409. Anything else falls through to a 500.
func subscriptionError(err error) error {
	switch {
	case errors.Is(err, subscription.ErrInvalidChannel):
		return fiber.NewError(fiber.StatusBadRequest, "unsupported notification channel")
	case errors.Is(err, subscription.ErrUnfilteredSearch):
		return fiber.NewError(fiber.StatusBadRequest,
			"add at least one filter before subscribing — an unfiltered search would notify you about every job")
	case errors.Is(err, subscription.ErrSavedSearchNotFound):
		return fiber.NewError(fiber.StatusNotFound, "saved search not found")
	case errors.Is(err, subscription.ErrDuplicate):
		return fiber.NewError(fiber.StatusConflict, "already subscribed to this saved search on this channel")
	case errors.Is(err, subscription.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "subscription not found")
	default:
		return err
	}
}

// createSubscriptionRequest is the create body: which saved search to subscribe and
// the delivery channel (defaults to telegram, the only channel today).
type createSubscriptionRequest struct {
	SavedSearchID int64  `json:"saved_search_id"`
	Channel       string `json:"channel"`
}

// setSubscriptionActiveRequest toggles a subscription on/off.
type setSubscriptionActiveRequest struct {
	Active bool `json:"active"`
}

// ListSubscriptions returns the authenticated user's subscriptions, newest first.
// Cookie-only (RequireAuth), owner-scoped.
func (h *subscriptionHandlers) ListSubscriptions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := h.subscription.List(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]subscriptionResponse, len(rows))
	for i, r := range rows {
		out[i] = toSubscriptionListItem(r)
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"total": len(out)}})
}

// CreateSubscription subscribes one of the caller's saved searches to a channel.
// A non-owned saved search is a 404, a duplicate is a 409. Cookie-only.
func (h *subscriptionHandlers) CreateSubscription(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createSubscriptionRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	channel := in.Channel
	if channel == "" {
		channel = subscription.ChannelTelegram
	}
	sub, err := h.subscription.Create(c.Context(), userID, in.SavedSearchID, channel)
	if err != nil {
		return subscriptionError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSubscriptionResponse(sub)})
}

// SetSubscriptionActive pauses/resumes a subscription, scoped to its owner. A
// missing or non-owned id is a 404. Cookie-only.
func (h *subscriptionHandlers) SetSubscriptionActive(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in setSubscriptionActiveRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	sub, err := h.subscription.SetActive(c.Context(), userID, id, in.Active)
	if err != nil {
		return subscriptionError(err)
	}
	return c.JSON(fiber.Map{"data": toSubscriptionResponse(sub)})
}

// DeleteSubscription unsubscribes by id, scoped to its owner. A missing or
// non-owned id is a 404. Cookie-only.
func (h *subscriptionHandlers) DeleteSubscription(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.subscription.Delete(c.Context(), userID, id); err != nil {
		return subscriptionError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
