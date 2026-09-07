package handler

import (
	"context"
	"errors"
	"log"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/discordlink"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
)

// discordStateCookieName carries the CSRF state across the Discord consent round-trip. Its
// own name, not shared with the Gmail or Calendar flows: two connects in flight at once must
// not be able to complete each other.
const discordStateCookieName = "fh_discord_state"

// discordCallbackPath is where Discord returns the browser. It is registered with the
// Discord application by hand, so it is written once here and read into the redirect_uri of
// both the consent URL and the token exchange — the two must be byte-identical or Discord
// refuses the exchange.
const discordCallbackPath = "/api/v1/me/discord/callback"

// discordScopes is the consent asked for. `identify` reads the account id we bind to;
// `guilds.join` is what lets one click put the user on the server, instead of sending them
// off to find an invite and come back.
const discordScopes = "identify guilds.join"

// DiscordLinker is the part of *discordlink.Service these routes use. Declared by the
// consumer and kept to three methods, so the service can grow without widening what the
// HTTP layer may do.
type DiscordLinker interface {
	Link(ctx context.Context, userID int64, code, redirectURI string) (discordlink.Link, error)
	Unlink(ctx context.Context, userID int64) error
	Status(ctx context.Context, userID int64) (discordlink.Link, error)
}

// discordHandlers serves linking a Discord account to a freehire account, so the paid role
// on the community server can follow the subscription.
//
// A nil svc is how the composition root says "not configured": register then mounts nothing
// and every route 404s. That is what lets this ship before the Discord application exists,
// and what rolling it back looks like.
type discordHandlers struct {
	svc            DiscordLinker
	clientID       string
	frontendOrigin string
	cookieSecure   bool
}

func newDiscordHandlers(svc DiscordLinker, clientID, frontendOrigin string, cookieSecure bool) *discordHandlers {
	return &discordHandlers{
		svc:            svc,
		clientID:       clientID,
		frontendOrigin: frontendOrigin,
		cookieSecure:   cookieSecure,
	}
}

func (h *discordHandlers) register(api fiber.Router, mw middleware) {
	if h.svc == nil {
		return
	}
	// Cookie-only, like every other integration connect: a redirect to a consent screen is
	// something a browser completes, and an API key cannot.
	api.Get("/me/discord", mw.cookie, h.Status)
	api.Get("/me/discord/connect", mw.cookie, h.Connect)
	api.Delete("/me/discord", mw.cookie, h.Unlink)

	// The callback is the browser returning from Discord, so it is mounted on
	// optionalCookie rather than cookie. Under RequireAuth a session that did not survive
	// the round-trip renders a JSON 401 into the address bar and strands the user; the
	// handler answers that case itself, with a redirect. The Gmail callback learned this
	// the hard way — see its own test.
	api.Get("/me/discord/callback", mw.optionalCookie, h.Callback)
}

// redirectURI is the address Discord returns the browser to. Built from the configured
// origin so the consent URL and the token exchange cannot disagree about it.
func (h *discordHandlers) redirectURI() string {
	return h.frontendOrigin + discordCallbackPath
}

// Status reports whether the caller has linked a Discord account, and whether the feature
// exists at all — so the SPA can render the card without a second request. Reaching this
// handler means the routes were mounted, so enabled is always true here; it is sent anyway
// because the SPA reads the same shape from every integration.
func (h *discordHandlers) Status(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	out := fiber.Map{"enabled": true, "linked": false}
	link, err := h.svc.Status(c.Context(), userID)
	switch {
	case err == nil:
		out["linked"] = true
		out["role_granted"] = link.RoleGranted
	case errors.Is(err, discordlink.ErrNotLinked):
		// Not linked is a state, not a failure — linked stays false.
	default:
		return err
	}
	return c.JSON(fiber.Map{"data": out})
}

// Connect sets the CSRF state cookie and sends the browser to Discord's consent screen.
func (h *discordHandlers) Connect(c *fiber.Ctx) error {
	if _, err := requireUserID(c); err != nil {
		return err
	}
	state, err := oauth.NewState()
	if err != nil {
		return err
	}
	oauth.SetStateCookieNamed(c, discordStateCookieName, state, h.cookieSecure)
	return c.Redirect(h.authCodeURL(state), fiber.StatusFound)
}

func (h *discordHandlers) authCodeURL(state string) string {
	q := url.Values{
		"client_id":     {h.clientID},
		"response_type": {"code"},
		"scope":         {discordScopes},
		"state":         {state},
		"redirect_uri":  {h.redirectURI()},
		// Discord otherwise skips the screen for a user who has consented before, which
		// silently re-links an account they may have deliberately disconnected.
		"prompt": {"consent"},
	}
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

// Callback finishes the flow and lands the browser back on Integrations — the surface the
// connect was started from.
//
// Every outcome is a redirect carrying a marker, never JSON: this is a top-level navigation
// returning from another site, and a JSON body here is rendered into the address bar. The
// underlying cause is logged server-side first, because the marker tells the user what
// happened and tells us nothing.
func (h *discordHandlers) Callback(c *fiber.Ctx) error {
	redirect := func(qs string, err error) error {
		log.Printf("discord connect: %s: %v", qs, err)
		return c.Redirect(h.frontendOrigin+integrationsPath+"?"+qs, fiber.StatusFound)
	}
	userID, ok := auth.UserID(c)
	if !ok {
		return redirect("discord_error=auth", errors.New("no authenticated user"))
	}

	cookieState := c.Cookies(discordStateCookieName)
	oauth.ClearStateCookieNamed(c, discordStateCookieName, h.cookieSecure)
	if cookieState == "" || c.Query("state") != cookieState {
		return redirect("discord_error=state", errors.New("state cookie missing or mismatched"))
	}
	// Discord echoes the state on its refusal redirect too, so passing the check above says
	// nothing about whether consent was given. A declined screen arrives as
	// ?error=access_denied with no code — the same trap the Gmail flow fell into, where it
	// read as success and told the user they were connected the moment they refused.
	if refusal := c.Query("error"); refusal != "" {
		return redirect("discord_error=denied", errors.New(refusal))
	}
	code := c.Query("code")
	if code == "" {
		return redirect("discord_error=exchange", errors.New("missing code"))
	}

	if _, err := h.svc.Link(c.Context(), userID, code, h.redirectURI()); err != nil {
		if errors.Is(err, discordlink.ErrAlreadyLinkedElsewhere) {
			// Its own marker: this one is the user's to resolve — they must disconnect the
			// Discord account from the other freehire account first — and "something went
			// wrong" would leave them retrying forever.
			return redirect("discord_error=taken", err)
		}
		return redirect("discord_error=exchange", err)
	}
	return c.Redirect(h.frontendOrigin+integrationsPath+"?discord=connected", fiber.StatusFound)
}

// Unlink revokes the role and removes the binding. Idempotent: unlinking something that is
// not linked is what the caller asked for, so it succeeds.
func (h *discordHandlers) Unlink(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Unlink(c.Context(), userID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"linked": false}})
}
