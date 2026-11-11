package handler

import (
	"context"
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

// register mounts the token-authenticated email preference routes.
//
// The token rides in the QUERY only where it has nowhere else to go. nginx logs
// query strings (freehire-ops' provision/host2/nginx/snippets/freehire-app.conf), so a never-expiring
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
func (h *emailPrefsHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The handlers below implement the contract (typespec/email-prefs.tsp); each route
	// here only names its middleware and ends in the generated server.
	api.Get("/email-prefs", contract)
	api.Patch("/email-prefs", contract)
	api.Post("/email-prefs/one-click", contract)

	api.Get("/me/email-groups", mw.cookie, contract)
	api.Patch("/me/email-groups", mw.cookie, contract)
}

// GetEmailPrefs returns the preferences a valid token opens.
func (h *emailPrefsHandlers) GetEmailPrefs(ctx context.Context, params apigen.GetEmailPrefsParams) (*apigen.GetEmailPrefsOK, error) {
	prefs, err := h.loadByToken(ctx, params.T.Value)
	if err != nil {
		return nil, err
	}
	return &apigen.GetEmailPrefsOK{Data: prefs}, nil
}

// UpdateEmailPrefs saves the switches somebody set. The token is a body field rather
// than a query parameter, so the one write path that has a choice keeps the
// credential out of the access log.
func (h *emailPrefsHandlers) UpdateEmailPrefs(ctx context.Context, req *apigen.EmailPrefsTokenUpdate) (*apigen.UpdateEmailPrefsOK, error) {
	token := req.Token.Value
	update := apigen.EmailPrefsUpdate{
		AlertsEnabled: req.AlertsEnabled, ActivityEnabled: req.ActivityEnabled,
		NewsEnabled: req.NewsEnabled, DeactivateSearches: req.DeactivateSearches,
	}
	err := applyPatch(update,
		func() (emailprefs.Prefs, error) { return h.prefs.Load(ctx, token) },
		func(in emailprefs.Update) error { return h.prefs.Save(ctx, token, in) })
	if err != nil {
		return nil, renderTokenError(err)
	}
	// Answer with what was actually stored rather than what the caller hoped it had stored.
	prefs, err := h.loadByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateEmailPrefsOK{Data: prefs}, nil
}

// applyPatch is the patch semantics both write routes share: read the stored state
// first so an ABSENT switch keeps its value rather than defaulting to false, then
// write. The two routes differ only in how they establish the user, which is what
// the two closures carry.
//
// Reading first is also what validates the caller before anything is written.
func applyPatch(
	body apigen.EmailPrefsUpdate,
	load func() (emailprefs.Prefs, error),
	save func(emailprefs.Update) error,
) error {
	current, err := load()
	if err != nil {
		return err
	}
	return save(emailprefs.Update{
		Alerts:             body.AlertsEnabled.Or(current.Alerts),
		Activity:           body.ActivityEnabled.Or(current.Activity),
		News:               body.NewsEnabled.Or(current.News),
		DeactivateSearches: body.DeactivateSearches,
	})
}

// OneClickUnsubscribe is the RFC 8058 target a mail client POSTs to with no
// confirmation screen. It turns off only the group the token names, and answers 200
// on a repeat so a retrying client reports success rather than an error.
func (h *emailPrefsHandlers) OneClickUnsubscribe(ctx context.Context, params apigen.OneClickUnsubscribeParams) (*apigen.OneClickUnsubscribeOK, error) {
	token := params.T.Value
	group, err := h.prefs.OneClick(ctx, token)
	if err != nil {
		return nil, renderTokenError(err)
	}
	// The response names what was turned off and points at the full page, so
	// somebody who meant "all of it" is one click from it. The link carries the
	// SAME token: without it the page has no way to know whose preferences to
	// open, and the "one click from the rest" would land on "this link is no
	// longer valid".
	return &apigen.OneClickUnsubscribeOK{Data: apigen.OneClickResult{
		UnsubscribedFrom: string(group),
		ManageURL:        emailprefs.Path + "?t=" + url.QueryEscape(token),
	}}, nil
}

// loadByToken reads the stored state a token opens. Both token routes answer through it.
func (h *emailPrefsHandlers) loadByToken(ctx context.Context, token string) (apigen.EmailPrefs, error) {
	prefs, err := h.prefs.Load(ctx, token)
	if err != nil {
		return apigen.EmailPrefs{}, renderTokenError(err)
	}
	return toEmailPrefsResponse(prefs), nil
}

// toEmailPrefsResponse projects the domain shape onto the wire one. Both the
// token-opened and the signed-in routes answer through it, so the two views cannot
// drift apart in what they report. It carries the address the mail already went to,
// the three switches, and the names of the searches those switches govern — nothing
// else about the account reaches this response.
func toEmailPrefsResponse(prefs emailprefs.Prefs) apigen.EmailPrefs {
	out := apigen.EmailPrefs{
		Email: prefs.Email, AlertsEnabled: prefs.Alerts, ActivityEnabled: prefs.Activity, NewsEnabled: prefs.News,
		Searches: make([]apigen.EmailPrefsSearch, 0, len(prefs.Searches)),
	}
	for _, s := range prefs.Searches {
		out.Searches = append(out.Searches, apigen.EmailPrefsSearch{ID: s.ID, Name: s.Name, Active: s.Active})
	}
	return out
}

// GetMyEmailGroups is the signed-in view of the same three switches.
func (h *emailPrefsHandlers) GetMyEmailGroups(ctx context.Context) (*apigen.GetMyEmailGroupsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := h.prefs.LoadFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.GetMyEmailGroupsOK{Data: toEmailPrefsResponse(prefs)}, nil
}

// UpdateMyEmailGroups writes them for a signed-in caller. Same patch semantics as the
// public route: an absent switch keeps its stored value.
func (h *emailPrefsHandlers) UpdateMyEmailGroups(ctx context.Context, req *apigen.EmailPrefsUpdate) (*apigen.UpdateMyEmailGroupsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	load := func() (emailprefs.Prefs, error) { return h.prefs.LoadFor(ctx, userID) }
	if err := applyPatch(*req, load, func(in emailprefs.Update) error {
		return h.prefs.SaveFor(ctx, userID, in)
	}); err != nil {
		return nil, err
	}
	prefs, err := load()
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateMyEmailGroupsOK{Data: toEmailPrefsResponse(prefs)}, nil
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
