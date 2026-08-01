package handler

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/llmkey"
)

// usageHandlers serves the caller's own AI spend, read from the gateway rather than from
// a table of ours: the gateway is what prices a call, and a second copy would be a second
// number to disagree with.
type usageHandlers struct {
	gateway *llmkey.Client
}

// newUsageHandlers takes the gateway and no resolver on purpose. The read is scoped by the
// account id rather than by a credential, so it needs no key, cannot mint one, and still
// reports a month during which the key was replaced.
func newUsageHandlers(gateway *llmkey.Client) *usageHandlers {
	return &usageHandlers{gateway: gateway}
}

func (h *usageHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/usage", mw.key, h.GetMyUsage)
}

// usageResponse is the wire shape of what one account did this period.
//
// It carries NO money, deliberately. The gateway's cost figure is a list price against a
// mixed upstream pool: it is not what we pay, and it is certainly not what the caller
// pays — their price is credits, reported by /me/credits over this same calendar. A second
// number in dollars beside a points balance reads as a second currency for one thing, and
// one of the two would be fiction.
type usageResponse struct {
	Requests int       `json:"requests"`
	Failed   int       `json:"failed"`
	Tokens   int       `json:"tokens"`
	Period   string    `json:"period"`
	ResetsAt time.Time `json:"resets_at"`
}

// GetMyUsage reports what the caller's account did this period.
//
// It cannot fail for any reason the caller could act on. An account that has never used
// AI, a deployment with no gateway, and a gateway that is down all answer 200 with zeroes:
// this is an informational read, and rendering an outage as an error would make an
// unrelated fault look to the reader like a problem with their account.
//
// It never mints. Looking at what you used is not using anything.
func (h *usageHandlers) GetMyUsage(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": h.usage(c, userID)})
}

// usage reads the gateway, or reports zeroes and says why in the log.
//
// The period is the calendar month credits already reset on, taken from that package so
// the two cannot drift: a balance and a usage count on different months would both look
// correct and disagree.
func (h *usageHandlers) usage(c *fiber.Ctx, userID int64) usageResponse {
	now := time.Now().UTC()
	out := usageResponse{Period: credits.PeriodKey(now), ResetsAt: credits.ResetsAt(now)}
	if h.gateway == nil {
		return out
	}
	// Scoped by the account id, not by the credential — so an account whose key was
	// re-minted mid-month still reports everything it did, under both.
	act, err := h.gateway.Activity(c.Context(), userID, credits.PeriodStart(now), now)
	if err != nil {
		log.Printf("usage: read activity for user %d: %v", userID, err)
		return out
	}
	out.Requests, out.Failed, out.Tokens = act.Requests, act.Failed, act.Tokens
	return out
}
