package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"html"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/engage/telegramnotify"
	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/platform/db"
)

// telegramHandlers serves Telegram account linking via a deep-link token, the link
// status/unlink reads, and the inbound bot webhook (which also fields pasted board URLs as
// contributions). All fields are nil/empty when the bot is unconfigured: linking then reports
// the feature off and the webhook is inert. telegramLinks mints/verifies the token,
// telegramBot replies, telegramBotUsername builds the t.me URL, telegramWebhookSecret guards the webhook.
type telegramHandlers struct {
	queries               *db.Queries
	telegramLinks         *telegramnotify.LinkTokens
	telegramBot           *telegramnotify.Client
	telegramBotUsername   string
	telegramWebhookSecret string
	frontendOrigin        string
	// intake is the shared look-import-record sequence: a pasted link gets the website's outcome.
	intake *intakeService
}

// newTelegramHandlers wires the bot only with both a bot token and a JWT secret (the link
// token reuses it); absent either the feature is off (see telegramEnabled).
func newTelegramHandlers(queries *db.Queries, jwtSecret, botToken, botUsername, webhookSecret, frontendOrigin string, intake *intakeService) *telegramHandlers {
	h := &telegramHandlers{
		queries:        queries,
		frontendOrigin: frontendOrigin,
		intake:         intake,
	}
	// The webhook secret is part of the enable condition: the handler's constant-time compare
	// treats an unset secret as matching an absent header, which would expose an unauthenticated POST.
	if botToken != "" && jwtSecret != "" {
		if webhookSecret == "" {
			log.Print("telegram: TELEGRAM_BOT_TOKEN is set but TELEGRAM_WEBHOOK_SECRET is not — feature disabled")
		} else {
			h.telegramLinks = telegramnotify.NewLinkTokens(jwtSecret, telegramLinkTTL)
			h.telegramBot = telegramnotify.NewClient(botToken)
			h.telegramBotUsername = botUsername
			h.telegramWebhookSecret = webhookSecret
		}
	}
	return h
}

func (h *telegramHandlers) register(api fiber.Router, mw middleware) {
	// Cookie-only like subscriptions, owner-scoped.
	api.Post("/me/telegram/link", mw.cookie, h.LinkTelegram)
	api.Get("/me/telegram", mw.cookie, h.TelegramLinkStatus)
	api.Delete("/me/telegram", mw.cookie, h.UnlinkTelegram)

	// The only unauthenticated POST, guarded by the secret token (see TelegramWebhook).
	api.Post("/telegram/webhook", h.TelegramWebhook)
}

// telegramEnabled reports whether the feature is configured (bot token + link-token issuer).
func (h *telegramHandlers) telegramEnabled() bool {
	return h.telegramLinks != nil && h.telegramBot != nil
}

// webhookSecured reports whether the inbound webhook may be served. Separate from
// telegramEnabled because the constant-time compare treats two empty strings as equal, so an
// unset secret would authenticate a request with no header.
func (h *telegramHandlers) webhookSecured() bool {
	return h.telegramEnabled() && h.telegramWebhookSecret != ""
}

// LinkTelegram mints a one-time deep-link token and returns the t.me URL the user
// opens to link their chat. Cookie-only. 503 when the feature is unconfigured.
func (h *telegramHandlers) LinkTelegram(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !h.telegramEnabled() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "telegram notifications are not configured")
	}
	url := "https://t.me/" + h.telegramBotUsername + "?start=" + h.telegramLinks.Issue(userID)
	return c.JSON(fiber.Map{"data": fiber.Map{"url": url}})
}

// TelegramLinkStatus reports whether the caller has linked a Telegram chat, and
// whether the feature is enabled at all (so the SPA can show/hide the UI).
func (h *telegramHandlers) TelegramLinkStatus(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	out := fiber.Map{"enabled": h.telegramEnabled(), "linked": false}
	link, err := h.queries.GetTelegramLink(c.Context(), userID)
	switch {
	case err == nil:
		out["linked"] = true
		out["chat_id"] = link.ChatID
	case errors.Is(err, pgx.ErrNoRows):
		// No link row yet — linked stays false.
	default:
		return err
	}
	return c.JSON(fiber.Map{"data": out})
}

// UnlinkTelegram removes the caller's Telegram link. Cookie-only. Idempotent: no
// existing link still returns 204.
func (h *telegramHandlers) UnlinkTelegram(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if _, err := h.queries.DeleteTelegramLink(c.Context(), userID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// TelegramWebhook receives Bot API updates, guarded by the secret token Telegram echoes in a
// header (set at setWebhook time). It completes linking from a "/start <token>" message:
// the token identifies the user, the chat_id is captured, and the bot confirms. It returns 200
// after the secret check so Telegram does not retry; problems go to the user via the bot.
func (h *telegramHandlers) TelegramWebhook(c *fiber.Ctx) error {
	if !h.webhookSecured() {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	// Reject forged updates: constant-time compare (no timing side-channel), 403 on mismatch.
	// An empty configured secret fails closed, or a request with no header would pass.
	if h.telegramWebhookSecret == "" ||
		subtle.ConstantTimeCompare([]byte(c.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(h.telegramWebhookSecret)) != 1 {
		return fiber.NewError(fiber.StatusForbidden, "forbidden")
	}

	var update telegramnotify.Update
	if err := c.BodyParser(&update); err != nil {
		// A malformed update is acknowledged (200) so Telegram stops retrying it.
		return c.SendStatus(fiber.StatusOK)
	}

	token, chatID, ok := telegramnotify.StartToken(update)
	if !ok {
		// Not a /start link command — a linked user may have pasted a board link to contribute.
		h.handleTelegramContribution(c, update)
		return c.SendStatus(fiber.StatusOK)
	}

	userID, err := h.telegramLinks.Parse(token)
	if err != nil {
		log.Printf("telegram webhook: invalid link token: %v", err)
		h.replyTelegram(c, chatID, "⚠️ This link is invalid or has expired. Open the link again from the site.")
		return c.SendStatus(fiber.StatusOK)
	}

	if err := h.queries.UpsertTelegramLink(c.Context(), db.UpsertTelegramLinkParams{UserID: userID, ChatID: chatID}); err != nil {
		log.Printf("telegram webhook: upsert link user=%d: %v", userID, err)
		h.replyTelegram(c, chatID, "⚠️ Something went wrong linking your account. Please try again.")
		return c.SendStatus(fiber.StatusOK)
	}

	h.replyTelegram(c, chatID, "✅ Linked! You'll get notifications here for your saved searches.")
	return c.SendStatus(fiber.StatusOK)
}

// replyTelegram sends a confirmation back to the chat; a send failure is only logged.
func (h *telegramHandlers) replyTelegram(c *fiber.Ctx, chatID int64, msg string) {
	h.sendTelegram(c.Context(), chatID, msg)
}

// sendTelegram posts msg to the chat, logging (not surfacing) a send failure.
func (h *telegramHandlers) sendTelegram(ctx context.Context, chatID int64, msg string) {
	if err := h.telegramBot.SendMessage(ctx, chatID, msg); err != nil {
		log.Printf("telegram: send to chat=%d: %v", chatID, err)
	}
}

// telegramContribTimeout bounds the background intake work (lookup, import, record, reply) so a
// stuck goroutine cannot leak. Generous because intake may fetch a whole ATS board; the webhook has already ACKed.
const telegramContribTimeout = 60 * time.Second

// telegramURL matches the first http(s) link in a message.
var telegramURL = regexp.MustCompile(`https?://[^\s]+`)

// handleTelegramContribution routes a board link pasted into the linked chat through the
// website's contribution flow. The DB + reply work runs in a background goroutine so the
// webhook ACKs at once: a slow webhook makes Telegram RE-DELIVER the update (a reply storm),
// and the request context is canceled on return. A message with no link is ignored.
func (h *telegramHandlers) handleTelegramContribution(_ *fiber.Ctx, update telegramnotify.Update) {
	if update.Message == nil {
		return
	}
	chatID := update.Message.Chat.ID
	m := telegramURL.FindString(update.Message.Text)
	if m == "" {
		return // no link — not a contribution attempt, stay quiet
	}
	// Trim trailing punctuation a user (or Telegram) may append to the link.
	rawURL := strings.TrimRight(m, ").,!?;:")
	go h.processTelegramContribution(chatID, rawURL)
}

// processTelegramContribution resolves the chat to its user, submits the link, and replies on
// its own bounded context. A link from an unlinked chat prompts the user to link first.
func (h *telegramHandlers) processTelegramContribution(chatID int64, rawURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), telegramContribTimeout)
	defer cancel()

	userID, err := h.queries.GetUserIDByTelegramChat(ctx, chatID)
	if errors.Is(err, pgx.ErrNoRows) {
		h.sendTelegram(ctx, chatID, "🔗 Link your freehire account first (Settings → Telegram on the site), then send board links here.")
		return
	}
	if err != nil {
		log.Printf("telegram: resolve chat=%d: %v", chatID, err)
		return
	}

	out, err := h.intake.Resolve(ctx, userID, rawURL, contribution.SurfaceTelegram)
	if err != nil {
		log.Printf("telegram: intake user=%d: %v", userID, err)
		h.sendTelegram(ctx, chatID, "⚠️ Something went wrong. Please try again.")
		return
	}
	h.sendTelegram(ctx, chatID, renderIntakeOutcome(out, h.frontendOrigin, telegramEmphasize))
}

// telegramEmphasize renders Telegram's HTML bold tag (outbound messages use parse_mode "HTML",
// see telegramnotify.Client.SendMessage); the text is escaped because it is untrusted.
func telegramEmphasize(s string) string {
	return "<b>" + html.EscapeString(s) + "</b>"
}
