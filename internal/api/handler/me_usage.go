package handler

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
)

// usageHandlers serves the caller's own AI spend, read from the gateway, which prices a
// call, rather than from a table of ours that could disagree.
type usageHandlers struct {
	gateway *llmkey.Client
	keys    *llmkey.Resolver
}

// newUsageHandlers takes the resolver as well as the gateway, only to READ. The gateway
// aggregates spend under the credential's own identifier, which lives in our database, so the
// id is looked up with Stored, never For: minting here would issue a credential to every
// visitor who opened the page.
func newUsageHandlers(gateway *llmkey.Client, keys *llmkey.Resolver) *usageHandlers {
	return &usageHandlers{gateway: gateway, keys: keys}
}

func (h *usageHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/usage", mw.key, contract)
}

// The wire shape (apigen.Usage) carries NO money, deliberately: the gateway's cost is a list
// price against a mixed upstream pool, not what we pay or what the caller pays (that is a
// plan allowance, reported by /me/plan). A dollar figure beside it would be a second,
// fictional currency.
//
// GetMyUsage reports what the caller's account did this period. It cannot fail for any
// reason the caller could act on: no AI use, no gateway, or a gateway outage all answer 200
// with zeroes, so an unrelated fault does not look like a problem with their account. It
// never mints.
func (h *usageHandlers) GetMyUsage(ctx context.Context) (*apigen.GetMyUsageOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return &apigen.GetMyUsageOK{Data: h.usage(ctx, userID)}, nil
}

// usage reads the gateway, or reports zeroes and says why in the log. The period is the UTC
// day the plan allowances reset on, taken from that package so the two cannot drift.
func (h *usageHandlers) usage(ctx context.Context, userID int64) apigen.Usage {
	now := time.Now().UTC()
	out := apigen.Usage{Period: plan.Day(now).Format(time.DateOnly), ResetsAt: plan.ResetsAt(now)}
	if h.gateway == nil {
		return out
	}
	// Scoped by the credential: the gateway keys its usage log by credential id and cannot ask
	// by account, so a key replaced mid-month leaves the earlier one's calls out. That happens
	// only when the gateway refuses a stored key; tracking retired ids to sum them is not
	// worth the dead credentials.
	act, err := h.gateway.Activity(ctx, h.keys.Stored(ctx, userID).ID, plan.Day(now), now)
	if err != nil {
		log.Printf("usage: read activity for user %d: %v", userID, err)
		return out
	}
	out.Requests, out.Failed, out.Tokens = int64(act.Requests), int64(act.Failed), int64(act.Tokens)
	return out
}
