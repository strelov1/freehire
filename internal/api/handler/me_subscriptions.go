package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *subscriptionHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Filter subscriptions are cookie-only (RequireAuth) like saved searches: a
	// browser convenience, owner-scoped (a non-owned id is 404). The handlers below
	// implement the contract (typespec/alerts.tsp).
	api.Get("/me/subscriptions", mw.cookie, contract)
	api.Post("/me/subscriptions", mw.cookie, contract)
	api.Patch("/me/subscriptions/:id", mw.cookie, contract)
	api.Delete("/me/subscriptions/:id", mw.cookie, contract)
}

// subscriptionOut is the public shape of a subscription. user_id and the internal start_at
// cursor are omitted; saved_search_name is included on list so the SPA can label each toggle
// (omitted on create/patch confirmations).
func subscriptionOut(s subscription.Subscription, savedSearchName string) apigen.Subscription {
	out := apigen.Subscription{
		ID:              s.ID,
		SavedSearchID:   s.SavedSearchID,
		SavedSearchName: optString(savedSearchName),
		Channel:         s.Channel,
		Active:          s.Active,
	}
	setNilDateTime(&out.CreatedAt, s.CreatedAt)
	return out
}

// subscriptionError maps the subscription sentinels onto HTTP statuses: an
// unsupported channel is a 400, a missing/non-owned saved search or subscription
// is a 404, a duplicate or a push subscription with no device to reach is a 409. Anything else falls through to a 500.
func subscriptionError(err error) error {
	switch {
	case errors.Is(err, subscription.ErrInvalidChannel):
		return fiber.NewError(fiber.StatusBadRequest, "unsupported notification channel")
	case errors.Is(err, subscription.ErrUnfilteredSearch):
		return fiber.NewError(fiber.StatusBadRequest,
			"add at least one filter before subscribing — an unfiltered search would notify you about every job")
	case errors.Is(err, subscription.ErrNoPushDevice):
		return fiber.NewError(fiber.StatusConflict,
			"push alerts need the freehire mobile app — sign in there and allow notifications first")
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

// ListSubscriptions returns the authenticated user's subscriptions, newest first.
// Cookie-only (RequireAuth), owner-scoped.
func (h *subscriptionHandlers) ListSubscriptions(ctx context.Context) (*apigen.SubscriptionList, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.subscription.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.Subscription, len(rows))
	for i, r := range rows {
		out[i] = subscriptionOut(r.Subscription, r.SavedSearchName)
	}
	return &apigen.SubscriptionList{Data: out, Meta: apigen.AlertListMeta{Total: int64(len(out))}}, nil
}

// CreateSubscription subscribes one of the caller's saved searches to a channel (telegram,
// the only default, when none is given). A non-owned saved search is a 404, a duplicate is
// a 409. Cookie-only.
func (h *subscriptionHandlers) CreateSubscription(ctx context.Context, req *apigen.SubscriptionCreate) (*apigen.CreateSubscriptionCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	channel := req.Channel.Value
	if channel == "" {
		channel = subscription.ChannelTelegram
	}
	sub, err := h.subscription.Create(ctx, userID, req.SavedSearchID.Value, channel)
	if err != nil {
		return nil, subscriptionError(err)
	}
	return &apigen.CreateSubscriptionCreated{Data: subscriptionOut(sub, "")}, nil
}

// SetSubscriptionActive pauses/resumes a subscription, scoped to its owner. A
// missing or non-owned id is a 404. Cookie-only.
func (h *subscriptionHandlers) SetSubscriptionActive(ctx context.Context, req *apigen.SubscriptionActive, params apigen.SetSubscriptionActiveParams) (*apigen.SetSubscriptionActiveOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	sub, err := h.subscription.SetActive(ctx, userID, id, req.Active.Value)
	if err != nil {
		return nil, subscriptionError(err)
	}
	return &apigen.SetSubscriptionActiveOK{Data: subscriptionOut(sub, "")}, nil
}

// DeleteSubscription unsubscribes by id, scoped to its owner. A missing or
// non-owned id is a 404. Cookie-only.
func (h *subscriptionHandlers) DeleteSubscription(ctx context.Context, params apigen.DeleteSubscriptionParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	return subscriptionError(h.subscription.Delete(ctx, userID, id))
}
