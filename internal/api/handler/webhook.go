package handler

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// webhookHandlers serves the account's single saved-search webhook
// destination: create/update, view, enable/disable, delete. Deliveries are
// plain, unsigned POSTs — there is no secret to manage.
type webhookHandlers struct {
	queries *db.Queries
}

func newWebhookHandlers(queries *db.Queries) *webhookHandlers {
	return &webhookHandlers{queries: queries}
}

func (h *webhookHandlers) register(api fiber.Router, mw middleware) {
	// Cookie-only, like subscription management: a browser convenience, never
	// an API key.
	api.Get("/me/webhook", mw.cookie, h.GetWebhook)
	api.Post("/me/webhook", mw.cookie, h.CreateOrUpdateWebhook)
	api.Patch("/me/webhook", mw.cookie, h.SetWebhookEnabled)
	api.Delete("/me/webhook", mw.cookie, h.DeleteWebhook)
}

// webhookResponse is the public shape of a webhook destination.
type webhookResponse struct {
	URL           string     `json:"url"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     *time.Time `json:"created_at"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	DisabledAt    *time.Time `json:"disabled_at"`
}

func toWebhookResponse(w db.WebhookConfig) webhookResponse {
	return webhookResponse{
		URL:           w.URL,
		Enabled:       w.Enabled,
		CreatedAt:     pgconv.TimePtr(w.CreatedAt),
		LastSuccessAt: pgconv.TimePtr(w.LastSuccessAt),
		DisabledAt:    pgconv.TimePtr(w.DisabledAt),
	}
}

type createWebhookRequest struct {
	URL string `json:"url"`
}

type setWebhookEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// validateWebhookURL rejects anything but an http(s) URL, per the
// webhook-notifications spec's creation-time scheme check.
func validateWebhookURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid webhook url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fiber.NewError(fiber.StatusBadRequest, "webhook url must be http or https")
	}
	return nil
}

// CreateOrUpdateWebhook creates the account's webhook destination, or updates
// its URL if one already exists — there is exactly one per account.
func (h *webhookHandlers) CreateOrUpdateWebhook(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createWebhookRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := validateWebhookURL(in.URL); err != nil {
		return err
	}

	row, err := h.queries.UpsertWebhookConfig(c.Context(), db.UpsertWebhookConfigParams{
		UserID: userID,
		URL:    in.URL,
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toWebhookResponse(row)})
}

// GetWebhook returns the authenticated user's webhook destination metadata, or
// null if none is configured.
func (h *webhookHandlers) GetWebhook(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	row, err := h.queries.GetWebhookConfig(c.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"data": nil})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toWebhookResponse(row)})
}

// SetWebhookEnabled toggles the destination on/off without changing its URL.
// A missing destination is a 404.
func (h *webhookHandlers) SetWebhookEnabled(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in setWebhookEnabledRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if in.Enabled {
		if _, err := h.queries.EnableWebhookConfig(c.Context(), userID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	} else if _, err := h.queries.DisableWebhookConfig(c.Context(), userID); err != nil {
		return err
	}

	// A single follow-up read renders the final state and is what actually
	// reports "no destination configured" — EnableWebhookConfig's own
	// pgx.ErrNoRows is swallowed above rather than duplicating that check.
	row, err := h.queries.GetWebhookConfig(c.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "no webhook destination configured")
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toWebhookResponse(row)})
}

// DeleteWebhook removes the account's webhook destination entirely. A missing
// destination is a 404.
func (h *webhookHandlers) DeleteWebhook(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	affected, err := h.queries.DeleteWebhookConfig(c.Context(), userID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "no webhook destination configured")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
