package handler

import (
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/llmkey"
)

// usageHandlers serves the caller's own AI spend, read from the gateway rather than from
// a table of ours: the gateway is what prices a call, and a second copy would be a second
// number to disagree with.
type usageHandlers struct {
	keys    *llmkey.Resolver
	gateway *llmkey.Client
}

func newUsageHandlers(keys *llmkey.Resolver, gateway *llmkey.Client) *usageHandlers {
	return &usageHandlers{keys: keys, gateway: gateway}
}

func (h *usageHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/usage", mw.key, h.GetMyUsage)
}

// usageResponse is the wire shape of one account's spend for the current period.
//
// Spend is what the gateway's price list says these calls are worth, NOT an invoice: the
// models behind it run on a mixed pool, so the figure compares one period or one feature
// against another honestly and is wrong to quote as a bill. Limit is 0 when no ceiling is
// configured, which is the ordinary deployment, and ResetsAt is absent when the gateway
// reports no window.
type usageResponse struct {
	Spend    float64    `json:"spend"`
	Limit    float64    `json:"limit"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

// GetMyUsage reports the caller's AI spend for the current period.
//
// It cannot fail for any reason the caller could act on. An account that has never used
// AI, a deployment with no gateway, and a gateway that is down all answer 200 with zeroes:
// this is an informational read, and rendering an outage as an error would make an
// unrelated fault look to the reader like a billing problem.
//
// It never mints. Looking at what you have spent is not spending.
func (h *usageHandlers) GetMyUsage(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": h.spend(c, userID)})
}

// spend reads the gateway, or reports zeroes and says why in the log.
func (h *usageHandlers) spend(c *fiber.Ctx, userID int64) usageResponse {
	secret := h.keys.Stored(c.Context(), userID)
	if secret == "" || h.gateway == nil {
		return usageResponse{}
	}
	got, err := h.gateway.Spend(c.Context(), secret)
	if errors.Is(err, llmkey.ErrUnknownKey) {
		// The gateway has forgotten this credential. Clear it here rather than leaving
		// the account to discover it on its next model call, which would spend a
		// round trip finding out what this read already knows.
		h.keys.Forget(c.Context(), userID, secret)
		return usageResponse{}
	}
	if err != nil {
		log.Printf("usage: read spend for user %d: %v", userID, err)
		return usageResponse{}
	}
	out := usageResponse{Spend: got.Amount, Limit: got.Limit}
	if !got.ResetsAt.IsZero() {
		at := got.ResetsAt
		out.ResetsAt = &at
	}
	return out
}
