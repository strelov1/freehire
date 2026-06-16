package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/subscription"
)

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

func toSubscriptionResponse(s db.Subscription) subscriptionResponse {
	return subscriptionResponse{
		ID:            s.ID,
		SavedSearchID: s.SavedSearchID,
		Channel:       s.Channel,
		Active:        s.Active,
		CreatedAt:     timePtr(s.CreatedAt),
	}
}

func toSubscriptionListItem(s db.ListSubscriptionsRow) subscriptionResponse {
	return subscriptionResponse{
		ID:              s.ID,
		SavedSearchID:   s.SavedSearchID,
		SavedSearchName: s.SavedSearchName,
		Channel:         s.Channel,
		Active:          s.Active,
		CreatedAt:       timePtr(s.CreatedAt),
	}
}

// subscriptionError maps the subscription sentinels onto HTTP statuses: an
// unsupported channel is a 400, a missing/non-owned saved search or subscription
// is a 404, a duplicate is a 409. Anything else falls through to a 500.
func subscriptionError(err error) error {
	switch {
	case errors.Is(err, subscription.ErrInvalidChannel):
		return fiber.NewError(fiber.StatusBadRequest, "unsupported notification channel")
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
func (a *API) ListSubscriptions(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	rows, err := a.subscription.List(c.Context(), userID)
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
func (a *API) CreateSubscription(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	var in createSubscriptionRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	channel := in.Channel
	if channel == "" {
		channel = subscription.ChannelTelegram
	}
	sub, err := a.subscription.Create(c.Context(), userID, in.SavedSearchID, channel)
	if err != nil {
		return subscriptionError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSubscriptionResponse(sub)})
}

// SetSubscriptionActive pauses/resumes a subscription, scoped to its owner. A
// missing or non-owned id is a 404. Cookie-only.
func (a *API) SetSubscriptionActive(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid subscription id")
	}
	var in setSubscriptionActiveRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	sub, err := a.subscription.SetActive(c.Context(), userID, int64(id), in.Active)
	if err != nil {
		return subscriptionError(err)
	}
	return c.JSON(fiber.Map{"data": toSubscriptionResponse(sub)})
}

// DeleteSubscription unsubscribes by id, scoped to its owner. A missing or
// non-owned id is a 404. Cookie-only.
func (a *API) DeleteSubscription(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid subscription id")
	}
	if err := a.subscription.Delete(c.Context(), userID, int64(id)); err != nil {
		return subscriptionError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
