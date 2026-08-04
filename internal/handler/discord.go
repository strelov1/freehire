package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/discordbot"
)

// discordHandlers serves the Discord bot's linking and contribution surface: a
// deep-link-token mint, the link status/unlink reads, and the inbound
// interaction webhook (which fields the /link and /contribute slash
// commands). All nil/empty when the bot is unconfigured — the linking endpoints
// then report the feature off and the interaction webhook is inert. Discord
// has no notion of a magic-link URL like Telegram's /start deep link, so
// LinkDiscord hands the SPA a token to paste into /link instead. Unlike
// Telegram's webhook, which is guarded by a shared secret header, Discord's
// interaction webhook is guarded by an Ed25519 signature over the raw
// request body — see DiscordInteraction.
type discordHandlers struct {
	queries *db.Queries
	// discordBot edits the deferred /contribute reply once intake finishes.
	discordBot *discordbot.Client
	// discordLinks mints/verifies the /link deep-link token.
	discordLinks *discordbot.DiscordLinkTokens
	// discordPublicKey verifies the Ed25519 signature Discord attaches to every
	// interaction webhook request.
	discordPublicKey string
	// discordApplicationID addresses this app's own interaction-response API calls
	// (e.g. EditOriginalResponse, used by the deferred /contribute reply).
	discordApplicationID string
	frontendOrigin       string
	// intake is the shared look-import-record sequence, so a link pasted into the
	// bot's /contribute command gets exactly the outcome the same link would get
	// on the website.
	intake *intakeService
}

// newDiscordHandlers wires the bot only when all four Discord config values are
// present — there is no partial-enable state, unlike Telegram's separate bot-token
// and webhook-secret checks: Discord's own interaction signature already plays
// the webhook secret's role, so nothing is gained by admitting "token set, key
// missing" as a distinct state. Absent any of the four, the linking endpoints
// report the feature off and the interaction webhook is inert (see
// discordEnabled).
func newDiscordHandlers(queries *db.Queries, jwtSecret, botToken, applicationID, publicKey, guildID, frontendOrigin string, intake *intakeService) *discordHandlers {
	h := &discordHandlers{
		queries:        queries,
		frontendOrigin: frontendOrigin,
		intake:         intake,
	}
	switch {
	case botToken != "" && applicationID != "" && publicKey != "" && guildID != "":
		h.discordBot = discordbot.NewClient(botToken)
		h.discordLinks = discordbot.NewDiscordLinkTokens(jwtSecret, discordbot.DiscordLinkTTL)
		h.discordPublicKey = publicKey
		h.discordApplicationID = applicationID
	case botToken != "" || applicationID != "" || publicKey != "" || guildID != "":
		log.Print("discord: DISCORD_BOT_TOKEN, DISCORD_APPLICATION_ID, DISCORD_PUBLIC_KEY, and DISCORD_GUILD_ID must all be set together — feature disabled")
	}
	return h
}

func (h *discordHandlers) register(api fiber.Router, mw middleware) {
	// Discord linking is cookie-only (RequireAuth), like Telegram's: a browser
	// convenience, owner-scoped.
	api.Post("/me/discord/link", mw.cookie, h.LinkDiscord)
	api.Get("/me/discord", mw.cookie, h.DiscordLinkStatus)
	api.Delete("/me/discord", mw.cookie, h.UnlinkDiscord)

	// The interaction webhook is the only unauthenticated POST: it is guarded by
	// the Ed25519 signature Discord attaches to every request (see
	// DiscordInteraction), not by a bearer credential.
	api.Post("/discord/interactions", h.DiscordInteraction)
}

// discordEnabled reports whether the Discord bot is configured (all four config
// values present). When false the linking endpoints report the feature off and
// the interaction webhook is inert.
func (h *discordHandlers) discordEnabled() bool {
	return h.discordBot != nil && h.discordLinks != nil
}

// LinkDiscord mints a one-time link token for the caller to paste into the
// bot's /link slash command — Discord has no deep-link URL equivalent to
// Telegram's t.me?start=, so the SPA shows this token as text. Cookie-only.
// 503 when the feature is unconfigured.
func (h *discordHandlers) LinkDiscord(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !h.discordEnabled() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "discord bot is not configured")
	}
	token, err := h.discordLinks.Issue(userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"token":        token,
		"instructions": "In the freehire Discord server, run /link token:" + token,
	}})
}

// DiscordLinkStatus reports whether the caller has linked a Discord account, and
// whether the feature is enabled at all (so the SPA can show/hide the UI).
func (h *discordHandlers) DiscordLinkStatus(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	out := fiber.Map{"enabled": h.discordEnabled(), "linked": false}
	link, err := h.queries.GetDiscordLink(c.Context(), userID)
	switch {
	case err == nil:
		out["linked"] = true
		out["discord_id"] = link.DiscordID
	case errors.Is(err, pgx.ErrNoRows):
		// No link row yet — linked stays false.
	default:
		return err
	}
	return c.JSON(fiber.Map{"data": out})
}

// UnlinkDiscord removes the caller's Discord link. Cookie-only. Idempotent: no
// existing link still returns 204.
func (h *discordHandlers) UnlinkDiscord(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if _, err := h.queries.DeleteDiscordLink(c.Context(), userID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// DiscordInteraction receives Discord's interaction webhook: PING (the
// verification handshake) and slash-command invocations. It is the only
// unauthenticated POST, so it is guarded by the Ed25519 signature Discord
// signs every request with — verified over the RAW body, before any JSON
// parsing, so a malformed or hostile payload is rejected without the parser
// ever seeing it. A 403 tells Discord the endpoint failed verification; any
// later failure (bad command, DB error) still answers 200 with an in-payload
// error, because Discord does not retry interaction responses the way it
// retries a plain webhook.
func (h *discordHandlers) DiscordInteraction(c *fiber.Ctx) error {
	if !h.discordEnabled() {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}

	body := c.Body()
	sig := c.Get("X-Signature-Ed25519")
	timestamp := c.Get("X-Signature-Timestamp")
	if !discordbot.VerifySignature(h.discordPublicKey, []byte(timestamp), body, sig) {
		// Deliberately no signature, timestamp, or body in the log line: this endpoint is
		// unauthenticated and reachable by anyone, so nothing attacker-controlled belongs in it —
		// only the fixed fact that verification failed, which is what tells a stale/rotated
		// DISCORD_PUBLIC_KEY apart from an inert (unconfigured) feature in journalctl.
		log.Print("discord: interaction signature verification failed")
		return fiber.NewError(fiber.StatusForbidden, "forbidden")
	}

	var interaction discordbot.Interaction
	if err := json.Unmarshal(body, &interaction); err != nil {
		// The signature already proved this came from Discord, so a body it
		// cannot parse is Discord sending a shape we don't understand yet, not an
		// attack — acknowledge instead of erroring so nothing retries a request
		// that will never parse differently.
		return c.JSON(discordbot.EphemeralResponse("Sorry, that request wasn't understood."))
	}

	switch interaction.Type {
	case discordbot.InteractionTypePing:
		return c.JSON(discordbot.PongResponse())
	case discordbot.InteractionTypeApplicationCommand:
		return h.dispatchCommand(c, interaction)
	default:
		return c.JSON(discordbot.EphemeralResponse("Sorry, that request wasn't understood."))
	}
}

// dispatchCommand routes an APPLICATION_COMMAND interaction by command name.
// "link" always replies synchronously; "contribute" replies synchronously for
// an unidentified/unlinked caller and otherwise defers, finishing in the
// background (see handleContributeCommand). Any other name gets a generic
// not-yet-available reply rather than a panic on an unrecognized shape.
func (h *discordHandlers) dispatchCommand(c *fiber.Ctx, interaction discordbot.Interaction) error {
	if interaction.Data == nil {
		return c.JSON(discordbot.EphemeralResponse("Sorry, that command wasn't understood."))
	}
	switch interaction.Data.Name {
	case "link":
		return h.handleLinkCommand(c, interaction)
	case "contribute":
		return h.handleContributeCommand(c, interaction)
	default:
		return c.JSON(discordbot.EphemeralResponse("This command isn't available yet."))
	}
}

// handleLinkCommand completes account linking from the /link slash command's
// token argument. It replies synchronously (type 4, not deferred) — the DB
// upsert is fast enough to fit Discord's 3-second interaction window, so
// there's no need for the defer/EditOriginalResponse dance /contribute uses.
func (h *discordHandlers) handleLinkCommand(c *fiber.Ctx, interaction discordbot.Interaction) error {
	token := commandOption(interaction.Data, "token")
	userID, err := h.discordLinks.Parse(token)
	if err != nil {
		return c.JSON(discordbot.EphemeralResponse("⚠️ This link is invalid or has expired. Open the link again from the site."))
	}

	discordID, ok := interactionUserID(interaction)
	if !ok {
		return c.JSON(discordbot.EphemeralResponse("⚠️ Could not identify your Discord account."))
	}

	if err := h.queries.UpsertDiscordLink(c.Context(), db.UpsertDiscordLinkParams{UserID: userID, DiscordID: discordID}); err != nil {
		log.Printf("discord interaction: upsert link user=%d: %v", userID, err)
		return c.JSON(discordbot.EphemeralResponse("⚠️ Something went wrong linking your account. Please try again."))
	}

	return c.JSON(discordbot.EphemeralResponse("✅ Linked! You can now use /contribute here."))
}

// discordContribTimeout bounds the background intake work spawned from a /contribute
// interaction — the catalog lookup, the import, the record, and the EditOriginalResponse
// call — so a stuck goroutine cannot leak. Mirrors telegramContribTimeout (same value, same
// rationale): the interaction has already been deferred, so this budget delays nobody, but
// intake may now fetch a whole ATS board to read one vacancy.
const discordContribTimeout = 60 * time.Second

// handleContributeCommand resolves the caller's linked account SYNCHRONOUSLY, inside Discord's
// 3-second interaction budget — interactionUserID is in-memory and GetUserIDByDiscordID is a
// single indexed lookup, both well within it — and only THEN decides whether to defer: an
// unidentified or unlinked caller gets an immediate ephemeral reply and intakeService.Resolve is
// never reached (see the package's Global Constraints on no anonymous contribution). Only the
// genuinely slow part, intake.Resolve, runs in a background goroutine bounded by
// discordContribTimeout, behind a deferred response.
//
// Resolving the account first (rather than deferring unconditionally, as an earlier version
// did) also fixes a real race: the deferred ack and the goroutine that would otherwise act on an
// unresolved account both start immediately, so a fast failure path could call
// EditOriginalResponse before Discord had finished processing the deferred ack, 404ing the PATCH
// and leaving the user's reply stuck on "thinking".
func (h *discordHandlers) handleContributeCommand(c *fiber.Ctx, interaction discordbot.Interaction) error {
	url := commandOption(interaction.Data, "url")
	discordID, ok := interactionUserID(interaction)
	if !ok {
		return c.JSON(discordbot.EphemeralResponse("⚠️ Could not identify your Discord account."))
	}

	userID, err := h.queries.GetUserIDByDiscordID(c.Context(), discordID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(discordbot.EphemeralResponse("🔗 Link your freehire account first — run /link on the Contribute page (" + h.frontendOrigin + "/my/contributions), then /contribute again."))
	}
	if err != nil {
		log.Printf("discord: resolve discord_id=%d: %v", discordID, err)
		return c.JSON(discordbot.EphemeralResponse("⚠️ Something went wrong. Please try again."))
	}

	go h.processDiscordContribution(interaction.Token, userID, url)
	return c.JSON(discordbot.DeferredResponse())
}

// processDiscordContribution runs the intake sequence for an already-resolved account and edits
// the deferred reply with the outcome — on its own bounded background context (the interaction
// has already been acknowledged by the deferred response handleContributeCommand returned).
func (h *discordHandlers) processDiscordContribution(interactionToken string, userID int64, rawURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), discordContribTimeout)
	defer cancel()

	out, err := h.intake.Resolve(ctx, userID, rawURL, contribution.SurfaceDiscord)
	if err != nil {
		log.Printf("discord: intake user=%d: %v", userID, err)
		h.editOriginalResponse(ctx, interactionToken, "⚠️ Something went wrong. Please try again.")
		return
	}
	h.editOriginalResponse(ctx, interactionToken, renderIntakeOutcome(out, h.frontendOrigin, discordEmphasize))
}

// discordEmphasize renders emphasis as Discord Markdown bold. Discord interaction responses are
// Markdown, not HTML — unlike Telegram's parse_mode: "HTML" replies (see telegramEmphasize) — so
// no escaping is applied here.
func discordEmphasize(s string) string {
	return "**" + s + "**"
}

// editOriginalResponse edits the deferred interaction reply, logging (not surfacing) a send
// failure — the caller has already returned to Discord, so there is nothing left to fail.
func (h *discordHandlers) editOriginalResponse(ctx context.Context, interactionToken, content string) {
	if err := h.discordBot.EditOriginalResponse(ctx, h.discordApplicationID, interactionToken, content); err != nil {
		log.Printf("discord: edit original response: %v", err)
	}
}

// commandOption reads a named string option from a command invocation, or ""
// when absent.
func commandOption(data *discordbot.InteractionData, name string) string {
	for _, opt := range data.Options {
		if opt.Name == name {
			return opt.Value
		}
	}
	return ""
}

// interactionUserID resolves the invoking Discord account's snowflake id:
// member.user.id inside a guild (the expected case for this bot), user.id in a
// DM. ok is false when neither is present or the id doesn't parse — Discord's
// ids are numeric strings, but nothing enforces that of a field we don't own.
func interactionUserID(interaction discordbot.Interaction) (int64, bool) {
	var idStr string
	switch {
	case interaction.Member != nil && interaction.Member.User != nil:
		idStr = interaction.Member.User.ID
	case interaction.User != nil:
		idStr = interaction.User.ID
	default:
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
