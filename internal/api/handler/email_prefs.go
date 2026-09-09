package handler

import (
	"errors"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// emailPrefsHandlers serve the preference page a mail links to. Every route here is
// UNAUTHENTICATED by design: the whole point is that somebody holding one of our
// mails can turn it off without an account, a password, or a support ticket. What
// stands in for a session is the signed token in the link, which names one user and
// one group of mail and nothing else.
//
// No route here issues a cookie. A page reached by a forwarded link must not become
// a way into the account.
type emailPrefsHandlers struct {
	prefs *emailprefs.Service
}

func newEmailPrefsHandlers(svc *emailprefs.Service) *emailPrefsHandlers {
	return &emailPrefsHandlers{prefs: svc}
}

// register mounts the three routes behind a shared rate limiter.
//
// The token rides in the QUERY only where it has nowhere else to go. nginx logs
// query strings (deploy/nginx/snippets/freehire-app.conf), so a never-expiring
// credential in a URL lands in a bulk store that outlives the click:
//
//   - GET has no body, and the link in the mail has to carry it. The page strips it
//     from the address bar after the first read.
//   - PATCH has a body already, so the token goes there and never reaches a log.
//   - POST (one-click) has no choice: RFC 8058 fixes the body to
//     `List-Unsubscribe=One-Click`, so the URL Gmail was handed is the only place
//     left. The access log is switched off for that path on the host instead.
//
// The two /me routes are the same three switches for somebody who IS signed in, so
// the account's own settings page shows and writes exactly what the emailed link
// would. They share the service rather than the endpoint because only the way the
// user is established differs — and two views that disagreed about one boolean would
// read as the product losing a setting.
//
// The limiter is built here from the shared throttler, the way every other feature
// handler that needs one does it. 30/minute is generous for somebody tapping
// switches on one page and mean for anyone walking the id space; forging a token
// needs the signing secret, so this bounds nuisance rather than forgery.
func (h *emailPrefsHandlers) register(api fiber.Router, mw middleware) {
	limiter := ratelimit.Middleware(mw.throttler, ratelimit.KeyByIP("email-prefs"), 30, time.Minute)

	api.Get("/email-prefs", limiter, h.Get)
	api.Patch("/email-prefs", limiter, h.Patch)
	api.Post("/email-prefs/one-click", limiter, h.OneClick)

	api.Get("/me/email-groups", mw.cookie, h.GetMine)
	api.Patch("/me/email-groups", mw.cookie, h.PatchMine)
}

// emailPrefsResponse is the wire shape of the page's state. It carries the address
// the mail already went to, the three switches, and the names of the searches those
// switches govern — nothing else about the account reaches this response.
type emailPrefsResponse struct {
	Email    string                     `json:"email"`
	Alerts   bool                       `json:"alerts_enabled"`
	Activity bool                       `json:"activity_enabled"`
	News     bool                       `json:"news_enabled"`
	Searches []emailPrefsSearchResponse `json:"searches"`
}

type emailPrefsSearchResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// Get returns the preferences a valid token opens.
func (h *emailPrefsHandlers) Get(c *fiber.Ctx) error {
	return h.respondWithState(c, c.Query("t"))
}

// emailPrefsUpdateRequest is what the page sends back.
//
// The token is a body field here rather than a query parameter, so the one write
// path that has a choice keeps the credential out of the access log.
//
// The switches are pointers so "absent" and "false" are different things: a partial
// body must not read as "turn everything off". A missing switch keeps its stored
// value, which is what stops this endpoint being a full replace — the trap the
// contacts endpoint already fell into once.
type emailPrefsUpdateRequest struct {
	Token              string  `json:"token"`
	Alerts             *bool   `json:"alerts_enabled"`
	Activity           *bool   `json:"activity_enabled"`
	News               *bool   `json:"news_enabled"`
	DeactivateSearches []int64 `json:"deactivate_searches"`
}

// Patch saves the switches somebody set.
func (h *emailPrefsHandlers) Patch(c *fiber.Ctx) error {
	var body emailPrefsUpdateRequest
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	err := applyPatch(body,
		func() (emailprefs.Prefs, error) { return h.prefs.Load(c.Context(), body.Token) },
		func(in emailprefs.Update) error { return h.prefs.Save(c.Context(), body.Token, in) })
	if err != nil {
		return renderTokenError(err)
	}
	return h.respondWithState(c, body.Token)
}

// applyPatch is the patch semantics both write routes share: read the stored state
// first so an ABSENT switch keeps its value rather than defaulting to false, then
// write. The two routes differ only in how they establish the user, which is what
// the two closures carry.
//
// Reading first is also what validates the caller before anything is written.
func applyPatch(
	body emailPrefsUpdateRequest,
	load func() (emailprefs.Prefs, error),
	save func(emailprefs.Update) error,
) error {
	current, err := load()
	if err != nil {
		return err
	}
	return save(emailprefs.Update{
		Alerts:             boolOr(body.Alerts, current.Alerts),
		Activity:           boolOr(body.Activity, current.Activity),
		News:               boolOr(body.News, current.News),
		DeactivateSearches: body.DeactivateSearches,
	})
}

// OneClick is the RFC 8058 target a mail client POSTs to with no confirmation
// screen. It turns off only the group the token names, and answers 200 on a repeat
// so a retrying client reports success rather than an error.
func (h *emailPrefsHandlers) OneClick(c *fiber.Ctx) error {
	token := c.Query("t")
	group, err := h.prefs.OneClick(c.Context(), token)
	if err != nil {
		return renderTokenError(err)
	}
	// The response names what was turned off and points at the full page, so
	// somebody who meant "all of it" is one click from it. The link carries the
	// SAME token: without it the page has no way to know whose preferences to
	// open, and the "one click from the rest" would land on "this link is no
	// longer valid".
	return c.JSON(fiber.Map{"data": fiber.Map{
		"unsubscribed_from": string(group),
		"manage_url":        emailprefs.Path + "?t=" + url.QueryEscape(token),
	}})
}

// respondWithState reads and returns the stored state. Both the read route and the
// write route answer through it, so a save renders what was actually stored rather
// than what the caller hoped it had stored.
func (h *emailPrefsHandlers) respondWithState(c *fiber.Ctx, token string) error {
	prefs, err := h.prefs.Load(c.Context(), token)
	if err != nil {
		return renderTokenError(err)
	}
	return c.JSON(fiber.Map{"data": toEmailPrefsResponse(prefs)})
}

// toEmailPrefsResponse projects the domain shape onto the wire one. Both the
// token-opened and the signed-in routes answer through it, so the two views cannot
// drift apart in what they report.
func toEmailPrefsResponse(prefs emailprefs.Prefs) emailPrefsResponse {
	out := emailPrefsResponse{
		Email: prefs.Email, Alerts: prefs.Alerts, Activity: prefs.Activity, News: prefs.News,
		Searches: make([]emailPrefsSearchResponse, 0, len(prefs.Searches)),
	}
	for _, s := range prefs.Searches {
		out.Searches = append(out.Searches, emailPrefsSearchResponse{ID: s.ID, Name: s.Name, Active: s.Active})
	}
	return out
}

// GetMine is the signed-in view of the same three switches.
func (h *emailPrefsHandlers) GetMine(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	prefs, err := h.prefs.LoadFor(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toEmailPrefsResponse(prefs)})
}

// PatchMine writes them for a signed-in caller. Same patch semantics as the public
// route: an absent switch keeps its stored value.
func (h *emailPrefsHandlers) PatchMine(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var body emailPrefsUpdateRequest
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	load := func() (emailprefs.Prefs, error) { return h.prefs.LoadFor(c.Context(), userID) }
	if err := applyPatch(body, load, func(in emailprefs.Update) error {
		return h.prefs.SaveFor(c.Context(), userID, in)
	}); err != nil {
		return err
	}
	prefs, err := load()
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toEmailPrefsResponse(prefs)})
}

// renderTokenError maps every refusal to ONE response. A caller here is
// unauthenticated, so telling them apart — forged signature, unknown group, account
// deleted — would answer which user ids exist, one 404 at a time. The distinction
// survives in the wrapped error the log records.
func renderTokenError(err error) error {
	if errors.Is(err, emailprefs.ErrInvalidToken) {
		return fiber.NewError(fiber.StatusNotFound, "this link is no longer valid")
	}
	return err
}

// boolOr resolves an optional wire field against the value already stored.
func boolOr(v *bool, current bool) bool {
	if v == nil {
		return current
	}
	return *v
}
