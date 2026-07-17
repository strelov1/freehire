package handler

import (
	"crypto/subtle"
	"errors"
	"log"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/telegramnotify"
)

// telegramEnabled reports whether the Telegram notification feature is configured
// (bot token + link-token issuer present). When false the linking endpoints report
// the feature off and the webhook is inert.
func (a *API) telegramEnabled() bool {
	return a.telegramLinks != nil && a.telegramBot != nil
}

// LinkTelegram mints a one-time deep-link token and returns the t.me URL the user
// opens to link their chat. Cookie-only. 503 when the feature is unconfigured.
func (a *API) LinkTelegram(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !a.telegramEnabled() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "telegram notifications are not configured")
	}
	token, err := a.telegramLinks.Issue(userID)
	if err != nil {
		return err
	}
	url := "https://t.me/" + a.telegramBotUsername + "?start=" + token
	return c.JSON(fiber.Map{"data": fiber.Map{"url": url}})
}

// TelegramLinkStatus reports whether the caller has linked a Telegram chat, and
// whether the feature is enabled at all (so the SPA can show/hide the UI).
func (a *API) TelegramLinkStatus(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	out := fiber.Map{"enabled": a.telegramEnabled(), "linked": false}
	link, err := a.queries.GetTelegramLink(c.Context(), userID)
	if err == nil {
		out["linked"] = true
		out["chat_id"] = link.ChatID
	}
	return c.JSON(fiber.Map{"data": out})
}

// UnlinkTelegram removes the caller's Telegram link. Cookie-only. Idempotent: no
// existing link still returns 204.
func (a *API) UnlinkTelegram(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if _, err := a.queries.DeleteTelegramLink(c.Context(), userID); err != nil {
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
func (a *API) TelegramWebhook(c *fiber.Ctx) error {
	if !a.telegramEnabled() {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	// Reject forged updates: the secret token must match the one registered with
	// setWebhook. Compared in constant time so a timing side-channel cannot be used
	// to recover the secret byte by byte; a mismatch is a 403.
	if subtle.ConstantTimeCompare([]byte(c.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(a.telegramWebhookSecret)) != 1 {
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
		a.handleTelegramContribution(c, update)
		return c.SendStatus(fiber.StatusOK)
	}

	userID, err := a.telegramLinks.Parse(token)
	if err != nil {
		log.Printf("telegram webhook: invalid link token: %v", err)
		a.replyTelegram(c, chatID, "⚠️ This link is invalid or has expired. Open the link again from the site.")
		return c.SendStatus(fiber.StatusOK)
	}

	if err := a.queries.UpsertTelegramLink(c.Context(), db.UpsertTelegramLinkParams{UserID: userID, ChatID: chatID}); err != nil {
		log.Printf("telegram webhook: upsert link user=%d: %v", userID, err)
		a.replyTelegram(c, chatID, "⚠️ Something went wrong linking your account. Please try again.")
		return c.SendStatus(fiber.StatusOK)
	}

	a.replyTelegram(c, chatID, "✅ Linked! You'll get notifications here for your saved searches.")
	return c.SendStatus(fiber.StatusOK)
}

// replyTelegram sends a confirmation back to the chat; a send failure is logged,
// not surfaced (the link itself already succeeded or failed independently).
func (a *API) replyTelegram(c *fiber.Ctx, chatID int64, msg string) {
	if err := a.telegramBot.SendMessage(c.Context(), chatID, msg); err != nil {
		log.Printf("telegram webhook: reply to chat=%d: %v", chatID, err)
	}
}

// telegramURL matches the first http(s) link in a message.
var telegramURL = regexp.MustCompile(`https?://[^\s]+`)

// handleTelegramContribution runs a board link pasted into the linked chat through the same
// contribution flow as the website: it resolves the chat to its user, submits the first URL
// in the message, and replies with the outcome. A message with no link is ignored silently
// (so ordinary chatter draws no reply); a link from an unlinked chat prompts the user to link.
func (a *API) handleTelegramContribution(c *fiber.Ctx, update telegramnotify.Update) {
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

	userID, err := a.queries.GetUserIDByTelegramChat(c.Context(), chatID)
	if errors.Is(err, pgx.ErrNoRows) {
		a.replyTelegram(c, chatID, "🔗 Link your freehire account first (Settings → Telegram on the site), then send board links here.")
		return
	}
	if err != nil {
		log.Printf("telegram webhook: resolve chat=%d: %v", chatID, err)
		return
	}

	rec, err := a.contribution.Submit(c.Context(), userID, rawURL)
	switch {
	case errors.Is(err, contribution.ErrUnsupportedATS):
		a.replyTelegram(c, chatID, "🤔 That link isn't from a supported ATS board. Send a link from a company's Greenhouse, Lever, Ashby, or Workable careers page.")
	case errors.Is(err, contribution.ErrBoardAlreadyTracked):
		a.replyTelegram(c, chatID, "👍 We already track that board — nothing to add.")
	case errors.Is(err, contribution.ErrBoardAlreadyContributed):
		a.replyTelegram(c, chatID, "✅ That board was already contributed — no new point, but thanks!")
	case err != nil:
		log.Printf("telegram webhook: submit user=%d: %v", userID, err)
		a.replyTelegram(c, chatID, "⚠️ Something went wrong. Please try again.")
	default:
		a.replyTelegram(c, chatID, "🎉 <b>"+rec.Board+"</b> ("+rec.Source+") is a new board — we'll start crawling it. +1 point!")
	}
}
