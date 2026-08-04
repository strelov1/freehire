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

	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/telegramnotify"
)

// telegramHandlers serves the Telegram notification wiring: account linking via a
// deep-link token, the link status/unlink reads, and the inbound bot webhook (which
// also fields pasted board URLs as contributions). All nil/empty when the bot is
// unconfigured — the linking endpoints then report the feature off and the webhook
// is inert. telegramLinks mints/verifies the deep-link token; telegramBot replies
// to the inbound /start; telegramBotUsername builds the t.me URL;
// telegramWebhookSecret guards the inbound webhook.
type telegramHandlers struct {
	queries               *db.Queries
	telegramLinks         *telegramnotify.LinkTokens
	telegramBot           *telegramnotify.Client
	telegramBotUsername   string
	telegramWebhookSecret string
	frontendOrigin        string
	// intake is the shared look-import-record sequence, so a link pasted into the bot gets
	// exactly the outcome the same link would get on the website.
	intake *intakeService
}

// newTelegramHandlers wires the bot only with both a bot token and a JWT secret
// (the link token reuses it). Absent either, the linking endpoints report the
// feature off and the webhook is inert (see telegramEnabled).
func newTelegramHandlers(queries *db.Queries, jwtSecret, botToken, botUsername, webhookSecret, frontendOrigin string, intake *intakeService) *telegramHandlers {
	h := &telegramHandlers{
		queries:        queries,
		frontendOrigin: frontendOrigin,
		intake:         intake,
	}
	// The webhook secret is part of the enable condition, not a check inside the handler:
	// the handler's constant-time compare treats an unset secret as "matches an absent
	// header", so a deployment with a bot token but no secret would expose an
	// unauthenticated POST. Requiring it here makes "bot live, webhook open"
	// unrepresentable rather than merely guarded against.
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
	// Telegram linking is cookie-only (RequireAuth) like subscriptions: a browser
	// convenience, owner-scoped.
	api.Post("/me/telegram/link", mw.cookie, h.LinkTelegram)
	api.Get("/me/telegram", mw.cookie, h.TelegramLinkStatus)
	api.Delete("/me/telegram", mw.cookie, h.UnlinkTelegram)

	// The Telegram webhook is the only unauthenticated POST: it is guarded by the
	// shared secret token Telegram echoes in a header (see TelegramWebhook).
	api.Post("/telegram/webhook", h.TelegramWebhook)
}

// telegramEnabled reports whether the Telegram notification feature is configured
// (bot token + link-token issuer present). When false the linking endpoints report
// the feature off and the webhook is inert.
func (h *telegramHandlers) telegramEnabled() bool {
	return h.telegramLinks != nil && h.telegramBot != nil
}

// webhookSecured reports whether the inbound webhook may be served at all. It is
// deliberately separate from telegramEnabled: the secret compare below is a constant-time
// equality, and two empty strings ARE equal — so with an unset secret a request carrying
// no header would authenticate. Gating the endpoint on a non-empty secret makes "bot live,
// webhook open" unrepresentable rather than merely guarded against.
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
	token, err := h.telegramLinks.Issue(userID)
	if err != nil {
		return err
	}
	url := "https://t.me/" + h.telegramBotUsername + "?start=" + token
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

// TelegramWebhook receives Bot API updates. It is the only unauthenticated POST,
// so it is guarded by the shared secret token Telegram echoes in a header (set at
// setWebhook time). It completes account linking from a "/start <token>" message:
// the token identifies the user, the chat_id is captured, and the bot confirms.
// It always returns 200 so Telegram does not retry; problems are reported to the
// user via the bot, not via the HTTP status.
func (h *telegramHandlers) TelegramWebhook(c *fiber.Ctx) error {
	if !h.webhookSecured() {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	// Reject forged updates: the secret token must match the one registered with
	// setWebhook. Compared in constant time so a timing side-channel cannot be used
	// to recover the secret byte by byte; a mismatch is a 403. An empty configured
	// secret also fails closed — otherwise a request with no header at all would
	// pass the comparison.
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

// replyTelegram sends a confirmation back to the chat; a send failure is logged,
// not surfaced (the link itself already succeeded or failed independently).
func (h *telegramHandlers) replyTelegram(c *fiber.Ctx, chatID int64, msg string) {
	h.sendTelegram(c.Context(), chatID, msg)
}

// sendTelegram posts msg to the chat, logging (not surfacing) a send failure.
func (h *telegramHandlers) sendTelegram(ctx context.Context, chatID int64, msg string) {
	if err := h.telegramBot.SendMessage(ctx, chatID, msg); err != nil {
		log.Printf("telegram: send to chat=%d: %v", chatID, err)
	}
}

// telegramContribTimeout bounds the background intake work spawned from a webhook update — the
// catalog lookup, the import, the record, and the outbound reply — so a stuck goroutine cannot
// leak. It is generous because the intake may now fetch a whole ATS board to read one vacancy;
// the webhook itself has already ACKed, so this budget delays nobody.
const telegramContribTimeout = 60 * time.Second

// telegramURL matches the first http(s) link in a message.
var telegramURL = regexp.MustCompile(`https?://[^\s]+`)

// handleTelegramContribution routes a board link pasted into the linked chat through the same
// contribution flow as the website. It extracts the link and hands the DB + reply work to a
// background goroutine so the webhook can ACK immediately: a slow webhook makes Telegram time
// out and RE-DELIVER the update (a reply storm), and the request context is canceled the moment
// we return. A message with no link is ignored silently.
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

// processTelegramContribution resolves the chat to its user, submits the link, and replies —
// on its own bounded background context (the webhook has already returned). A link from an
// unlinked chat prompts the user to link first.
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

// telegramEmphasize renders emphasis as Telegram's HTML bold tag, matching the parse_mode:
// "HTML" every outbound message is sent with (see telegramnotify.Client.SendMessage) — the
// board name is escaped because it is untrusted text riding inside markup we control.
func telegramEmphasize(s string) string {
	return "<b>" + html.EscapeString(s) + "</b>"
}
