package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
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
	a.sendTelegram(c.Context(), chatID, msg)
}

// sendTelegram posts msg to the chat, logging (not surfacing) a send failure.
func (a *API) sendTelegram(ctx context.Context, chatID int64, msg string) {
	if err := a.telegramBot.SendMessage(ctx, chatID, msg); err != nil {
		log.Printf("telegram: send to chat=%d: %v", chatID, err)
	}
}

// alreadyTrackedReply is the "we already cover this" message, linking to the company page when
// the board resolves to a tracked company; otherwise a plain acknowledgement.
func (a *API) alreadyTrackedReply(ctx context.Context, rawURL string) string {
	name, slug, ok := a.contribution.TrackedCompany(ctx, rawURL)
	if !ok || slug == "" || a.frontendOrigin == "" {
		return "👍 We already track that board — nothing to add."
	}
	return fmt.Sprintf(
		"👍 <b>%s</b> is already tracked. That exact role might not be in the catalogue yet, but it'll appear on the next crawl.\n%s/companies/%s",
		html.EscapeString(name), a.frontendOrigin, slug,
	)
}

// telegramContribTimeout bounds the background contribution work spawned from a webhook
// update — the DB lookups plus the outbound reply — so a stuck goroutine cannot leak.
const telegramContribTimeout = 15 * time.Second

// telegramURL matches the first http(s) link in a message.
var telegramURL = regexp.MustCompile(`https?://[^\s]+`)

// handleTelegramContribution routes a board link pasted into the linked chat through the same
// contribution flow as the website. It extracts the link and hands the DB + reply work to a
// background goroutine so the webhook can ACK immediately: a slow webhook makes Telegram time
// out and RE-DELIVER the update (a reply storm), and the request context is canceled the moment
// we return. A message with no link is ignored silently.
func (a *API) handleTelegramContribution(_ *fiber.Ctx, update telegramnotify.Update) {
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
	go a.processTelegramContribution(chatID, rawURL)
}

// processTelegramContribution resolves the chat to its user, submits the link, and replies —
// on its own bounded background context (the webhook has already returned). A link from an
// unlinked chat prompts the user to link first.
func (a *API) processTelegramContribution(chatID int64, rawURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), telegramContribTimeout)
	defer cancel()

	userID, err := a.queries.GetUserIDByTelegramChat(ctx, chatID)
	if errors.Is(err, pgx.ErrNoRows) {
		a.sendTelegram(ctx, chatID, "🔗 Link your freehire account first (Settings → Telegram on the site), then send board links here.")
		return
	}
	if err != nil {
		log.Printf("telegram: resolve chat=%d: %v", chatID, err)
		return
	}

	rec, err := a.contribution.Submit(ctx, userID, rawURL)
	switch {
	case errors.Is(err, contribution.ErrUnsupportedATS):
		a.sendTelegram(ctx, chatID, "🤔 That link isn't from a supported ATS board. Send a link from a company's careers page on a supported ATS (Greenhouse, Lever, Ashby, Recruitee, BambooHR, SmartRecruiters, and many more).")
	case errors.Is(err, contribution.ErrBoardAlreadyTracked):
		a.sendTelegram(ctx, chatID, a.alreadyTrackedReply(ctx, rawURL))
	case errors.Is(err, contribution.ErrBoardAlreadyContributed):
		a.sendTelegram(ctx, chatID, "✅ That board was already contributed — no new point, but thanks!")
	case err != nil:
		log.Printf("telegram: submit user=%d: %v", userID, err)
		a.sendTelegram(ctx, chatID, "⚠️ Something went wrong. Please try again.")
	default:
		a.sendTelegram(ctx, chatID, "🎉 <b>"+rec.Board+"</b> ("+rec.Source+") is a new board — we'll start crawling it. +1 point!")
	}
}
