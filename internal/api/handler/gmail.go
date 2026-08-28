package handler

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/apptimeline"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/application/mailrecall"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// inboxHandlers serves the mail inbox surfaces: the "Connect Gmail" OAuth flow,
// the unified inbox reads, the email ↔ application linking, and the hosted-mailbox
// option. gmailConnector + gmailCipher are both nil when the feature is
// unconfigured (Google creds / token key absent) — the connect routes are then not
// registered and the inbox reads empty. mailDomain is empty when the
// hosted-mailbox feature is off (the claim route is unregistered and status
// reports unavailable).
type inboxHandlers struct {
	queries *db.Queries
	// pool backs the one write here that spans several statements: an ingest batch
	// is committed whole or not at all.
	pool           *pgxpool.Pool
	gmailConnector *gmailsync.Connector
	gmailCipher    *tokencrypt.Cipher
	frontendOrigin string
	cookieSecure   bool
	mailDomain     string
	// tracking records an application reconstructed from mail. The mail surface
	// borrows the tracking use case rather than writing its own apply, so the
	// applied_count guarantee stays in one place (see CreateApplicationFromEmail).
	tracking *jobtracking.Service
	// inbox holds the mail use cases these handlers render. The in-app assistant's
	// mail tools call the same service, so a rule can never hold for one reader and
	// not the other.
	inbox *inbox.Service
	// timeline reads the ledger for the application panel's history. The panel already
	// fetches this endpoint for its linked mail, so the history rides along rather than
	// costing a second request.
	timeline *apptimeline.Service
	// recall is the pull direction — from an application, find its mail. Nil on a
	// deployment with no model, and the endpoint reports the feature off rather than
	// panicking.
	recall *mailrecall.Service
	// llm binds a run to the caller's own gateway credential. Its zero value is the
	// unconfigured deployment, which every path already treats as "spend on the service
	// credential".
	llm llmBinding
	// mailboxes mints the caller's searchable mailbox for a recall run. Nil, or a caller
	// with no grant, means the sweep reads stored mail instead. An interface so a test can
	// stand a mailbox up without a Google credential — the fallback path and the search
	// path are different code, and both have to be covered.
	mailboxes mailboxes
}

// mailboxes resolves one caller's searchable mailbox, or nil when they have none.
type mailboxes interface {
	For(ctx context.Context, userID int64) mailrecall.Mailbox
}

// withRecall wires the mail-recall action after construction.
//
// Separate from newInboxHandlers because six test harnesses build these handlers for
// surfaces that make no model call, and two more positional parameters on a
// seven-parameter constructor would be paid by all of them for a field they leave nil.
func (h *inboxHandlers) withRecall(recall *mailrecall.Service, binding llmBinding, boxes mailboxes) *inboxHandlers {
	h.recall, h.llm, h.mailboxes = recall, binding, boxes
	return h
}

// trackingApplications adapts the tracking service to the one call the mail
// service makes of it. inbox does not need the resulting interaction, and taking
// it would pull jobtracking into that package for no reader's benefit.
type trackingApplications struct{ *jobtracking.Service }

func (t trackingApplications) MarkAppliedAt(ctx context.Context, userID int64, slug string, at time.Time, source string) error {
	_, err := t.Service.MarkAppliedAt(ctx, userID, slug, at, source)
	return err
}

func newInboxHandlers(queries *db.Queries, pool *pgxpool.Pool, gmailConnector *gmailsync.Connector, gmailCipher *tokencrypt.Cipher, frontendOrigin string, cookieSecure bool, mailDomain string) *inboxHandlers {
	tracking := jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))
	return &inboxHandlers{
		queries:        queries,
		pool:           pool,
		tracking:       tracking,
		inbox:          inbox.New(queries, trackingApplications{tracking}, inbox.WithIngester(inbox.NewQueriesIngester(pool, queries))),
		timeline:       apptimeline.New(queries),
		gmailConnector: gmailConnector,
		gmailCipher:    gmailCipher,
		frontendOrigin: frontendOrigin,
		cookieSecure:   cookieSecure,
		mailDomain:     mailDomain,
	}
}

func (h *inboxHandlers) register(api fiber.Router, mw middleware) {
	// Mail inbox (Gmail connect + hosted mailbox). Open to every signed-in user.
	// The read + disconnect routes are always registered (empty/no-op when not
	// connected); the OAuth connect routes only when configured.
	//
	// mw.key, not mw.cookie: a user running their own agent harness drives this
	// surface with their full-scope API key, the same credential the tracker
	// routes already accept — reading mail and linking it to applications is the
	// harness's job. mw.key is full-scope-only, so the narrow `cv` key a tailoring
	// bootstrap mints stays refused here.
	//
	// The OAuth connect flow is the exception and stays browser-bound: it
	// redirects a browser to Google's consent screen and back, so it is
	// meaningless to a keyed client and must not be reachable by one.
	api.Get("/me/gmail", mw.key, h.GmailStatus)
	api.Delete("/me/gmail", mw.key, h.GmailDisconnect)
	api.Get("/me/inbox", mw.key, h.GetInbox)
	api.Post("/me/inbox/read-all", mw.key, h.MarkAllReadInbox)
	api.Get("/me/emails/:id", mw.key, h.GetEmail)
	api.Post("/me/emails/:id/delete", mw.key, h.DeleteEmail)
	api.Post("/me/emails/:id/restore", mw.key, h.RestoreEmail)
	// Agent surface: a caller's own harness pushes mail it fetched itself and
	// records its triage verdict.
	api.Post("/me/emails", mw.key, h.IngestEmails)
	api.Post("/me/emails/:id/triage", mw.key, h.TriageEmail)
	// Email → application linking. :slug is registered after the static
	// /me/tracking/* routes (see Register) so it does not shadow them.
	api.Get("/me/tracking/:slug", mw.key, h.GetTrackedApplication)
	// The chase for an application that went quiet: assemble the draft, and record that the
	// candidate sent it. A key is admitted for the same reason the apply write admits one — the
	// CLI records applications, and a follow-up is the same kind of act.
	api.Get("/me/tracking/:slug/followup", mw.key, h.GetApplicationFollowUp)
	api.Post("/me/tracking/:slug/followup", mw.key, h.RecordApplicationFollowUp)
	api.Post("/me/emails/:id/link", mw.key, h.LinkEmail)
	api.Post("/me/emails/:id/unlink", mw.key, h.UnlinkEmail)
	api.Post("/me/emails/:id/confirm", mw.key, h.ConfirmEmailLink)
	api.Post("/me/emails/:id/reject", mw.key, h.RejectEmailLink)
	api.Post("/me/emails/:id/application", mw.key, h.CreateApplicationFromEmail)
	// The pull direction, beside the follow-up pair for the same reason they are there: it
	// acts on one application and a keyed client drives it as legitimately as a browser.
	// The limiter is the whole cost gate — this endpoint spends on the model and debits no
	// credit — and is mounted after the auth gate so it can key on the caller.
	api.Post("/me/tracking/:slug/mail-recall", mw.key, mailRecallLimiter(mw.throttler), h.RecallApplicationMail)
	// Import-and-link, for a proposal the sweep found in the mailbox and deliberately did
	// not store. Unlimited beside its sibling: this one is a person pressing Link on
	// something they just read, not a model call.
	api.Post("/me/tracking/:slug/mail-recall/link", mw.key, h.LinkRecalledMail)
	if h.gmailReady() {
		api.Get("/me/gmail/connect", mw.cookie, h.GmailConnect)
		// The callback is the browser returning from Google, not an XHR — so it is
		// mounted on optionalCookie, not cookie. Under RequireAuth a session that did
		// not survive the round-trip (expired mid-consent, or a callback landing on a
		// host the cookie is not scoped to) renders a JSON 401 into the address bar and
		// strands the user; GmailCallback answers that case itself with a redirect.
		api.Get("/me/gmail/callback", mw.optionalCookie, h.GmailCallback)
		// The calendar's own consent, beside the mail one and never folded into it:
		// connecting a mailbox must not quietly ask for a diary. Cookie-only for the
		// same reason as its neighbour — it redirects a browser to Google's screen,
		// which a keyed client cannot complete.
		api.Get("/me/calendar/connect", mw.cookie, h.CalendarConnect)
		api.Get("/me/calendar/callback", mw.optionalCookie, h.CalendarCallback)
		api.Post("/me/gmail/sync", mw.key, h.SyncGmail)
	}
	// Hosted-mailbox option: status is always available (reports unavailable when
	// the feature is off); claim/release only when a receiving domain is configured.
	api.Get("/me/mailbox", mw.key, h.GetMailbox)
	if h.mailboxReady() {
		api.Post("/me/mailbox", mw.key, h.ClaimMailbox)
		api.Delete("/me/mailbox", mw.key, h.ReleaseMailbox)
	}
}

// gmailStateCookieName carries the Gmail-connect CSRF state. It is NOT
// oauth.StateCookieName: a signed-in user can start a Gmail connect while an
// OAuth sign-in is in flight in another tab, and one shared cookie would
// overwrite the other flow's state.
const gmailStateCookieName = "hire_gmail_state"

// calendarStateCookieName carries the calendar-connect CSRF state, separate from the mail
// one so two consents in flight cannot complete each other.
const calendarStateCookieName = "hire_calendar_state"

// integrationsPath is where both the mail and calendar OAuth callbacks land, success or
// failure — the Integrations tab is the one surface that owns connect/disconnect for
// every third-party account.
const integrationsPath = "/my/integrations"

// GmailConnect starts the "Connect Gmail" incremental-OAuth flow for the
// signed-in user: it sets a CSRF state cookie and redirects to Google's consent
// screen for gmail.readonly.
func (h *inboxHandlers) GmailConnect(c *fiber.Ctx) error {
	state, err := oauth.NewState()
	if err != nil {
		return err
	}
	oauth.SetStateCookieNamed(c, gmailStateCookieName, state, h.cookieSecure)
	return c.Redirect(h.gmailConnector.AuthCodeURL(state), fiber.StatusFound)
}

// GmailCallback finishes the flow: it verifies state, exchanges the code for a
// refresh token + the connected address, stores the token encrypted, and
// redirects back to the Integrations tab — the surface the connect was started
// from. Failures redirect with ?gmail_error (never JSON); the underlying cause
// is logged server-side first (like oauthFail), since the generic redirect
// marker tells the user nothing.
func (h *inboxHandlers) GmailCallback(c *fiber.Ctx) error {
	redirect := func(qs string, err error) error {
		log.Printf("gmail connect: %s: %v", qs, err)
		return c.Redirect(h.frontendOrigin+integrationsPath+"?"+qs, fiber.StatusFound)
	}
	userID, ok := auth.UserID(c)
	if !ok {
		return redirect("gmail_error=auth", errors.New("no authenticated user"))
	}
	cookieState := c.Cookies(gmailStateCookieName)
	oauth.ClearStateCookieNamed(c, gmailStateCookieName, h.cookieSecure)
	if cookieState == "" || c.Query("state") != cookieState {
		return redirect("gmail_error=state", errors.New("state cookie missing or mismatched"))
	}
	if code := c.Query("code"); code != "" {
		refresh, email, granted, err := h.gmailConnector.Exchange(c.Context(), code)
		if err != nil {
			return redirect("gmail_error=exchange", err)
		}
		enc, err := h.gmailCipher.Encrypt(refresh)
		if err != nil {
			return redirect("gmail_error=exchange", err)
		}
		if err := h.queries.UpsertGmailConnection(c.Context(), db.UpsertGmailConnectionParams{
			UserID: userID, Email: email, RefreshTokenEnc: enc,
		}); err != nil {
			return redirect("gmail_error=exchange", err)
		}
		// Record what this grant covers. cal-sync selects connections by their recorded
		// scopes, so a row that never says what it holds is a row no worker can use.
		if err := h.queries.RecordGrantScopes(c.Context(), db.RecordGrantScopesParams{
			UserID: userID, Scopes: granted,
		}); err != nil {
			return redirect("gmail_error=exchange", err)
		}
	}
	return c.Redirect(h.frontendOrigin+integrationsPath+"?gmail=connected", fiber.StatusFound)
}

// CalendarConnect starts the calendar consent. Its own state cookie: two flows in flight
// at once must not be able to complete each other.
func (h *inboxHandlers) CalendarConnect(c *fiber.Ctx) error {
	state, err := oauth.NewState()
	if err != nil {
		return err
	}
	oauth.SetStateCookieNamed(c, calendarStateCookieName, state, h.cookieSecure)
	return c.Redirect(h.gmailConnector.CalendarAuthCodeURL(state), fiber.StatusFound)
}

// CalendarCallback finishes it: verify state, exchange the code, store the grant and note
// that it now covers the calendar. Failures redirect with ?calendar_error and are logged
// server-side first, exactly as the mail flow does — the marker tells the user nothing.
//
// It lands back on Integrations rather than the tracking calendar: that is the surface
// the connect was started from, same as the mail flow.
func (h *inboxHandlers) CalendarCallback(c *fiber.Ctx) error {
	redirect := func(qs string, err error) error {
		log.Printf("calendar connect: %s: %v", qs, err)
		return c.Redirect(h.frontendOrigin+integrationsPath+"?"+qs, fiber.StatusFound)
	}
	userID, ok := auth.UserID(c)
	if !ok {
		return redirect("calendar_error=auth", errors.New("no authenticated user"))
	}
	cookieState := c.Cookies(calendarStateCookieName)
	oauth.ClearStateCookieNamed(c, calendarStateCookieName, h.cookieSecure)
	if cookieState == "" || c.Query("state") != cookieState {
		return redirect("calendar_error=state", errors.New("state cookie missing or mismatched"))
	}
	if code := c.Query("code"); code != "" {
		refresh, granted, err := h.gmailConnector.ExchangeCalendar(c.Context(), code)
		if err != nil {
			return redirect("calendar_error=exchange", err)
		}
		enc, err := h.gmailCipher.Encrypt(refresh)
		if err != nil {
			return redirect("calendar_error=exchange", err)
		}
		if err := h.queries.UpsertCalendarGrant(c.Context(), db.UpsertCalendarGrantParams{
			// What Google says the grant covers, not what we asked for: the two differ
			// whenever a candidate declines part of a consent, and the record is what
			// every worker's filter reads.
			UserID: userID, RefreshTokenEnc: enc, Scopes: granted,
		}); err != nil {
			return redirect("calendar_error=exchange", err)
		}
	}
	return c.Redirect(h.frontendOrigin+integrationsPath+"?calendar=connected", fiber.StatusFound)
}

// GmailStatus reports whether the caller has connected Gmail.
func (h *inboxHandlers) GmailStatus(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	conn, err := h.queries.GetGmailConnection(c.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// available signals whether the connect flow is wired (Google creds + token
		// key), so the SPA hides the Connect button when it would 404.
		return c.JSON(fiber.Map{"data": fiber.Map{
			"connected": false, "available": h.gmailReady(), "calendar_connected": false,
		}})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		// The row's existence alone does not mean mail is connected: UpsertCalendarGrant
		// inserts this same row with an empty address for a calendar-only consent (see its
		// comment, and ListConnectedGmailUsers' matching `email <> ''` guard). Reporting
		// `connected: true` for that row would show the SPA's Mail card as connected with
		// no address behind it.
		"connected": conn.Email != "", "email": conn.Email, "status": conn.Status, "available": h.gmailReady(),
		// Whether this grant also covers the calendar. Read from the recorded scopes and
		// not from the row's existence: the two consents are separate, so a connected
		// mailbox says nothing about the calendar and a calendar grant may have no
		// mailbox behind it at all.
		"calendar_connected": slices.Contains(conn.Scopes, gmailsync.CalendarScope),
	}})
}

// SyncGmail triggers an on-demand sync of the caller's ATS mail. It runs in the
// background (a full backfill can exceed the request write timeout) and returns
// immediately; the SPA polls the inbox for results.
func (h *inboxHandlers) SyncGmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	conn, err := h.queries.GetGmailConnection(c.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusBadRequest, "Gmail is not connected")
	}
	if err != nil {
		return err
	}
	gmailStore := gmailsync.NewDBStore(h.queries)
	worker := gmailsync.NewWorker(gmailStore, h.gmailCipher, h.gmailConnector.ReaderFactory()).WithLearnedDomains(gmailStore)
	// Background context: the sync outlives this request.
	go worker.SyncUser(context.Background(), gmailsync.Connection{
		UserID: conn.UserID, Email: conn.Email, Cursor: conn.SyncCursor,
	})
	return c.JSON(fiber.Map{"data": fiber.Map{"started": true}})
}

// GmailDisconnect revokes the grant (best-effort) and purges the token and the
// user's synced mail.
func (h *inboxHandlers) GmailDisconnect(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	// Best-effort revoke with Google before purging our copy.
	if err := h.revokeGmailGrant(c.Context(), userID); err != nil {
		log.Printf("gmail disconnect: revoke for user %d: %v", userID, err)
	}
	// Purge only this user's Gmail-sourced mail; a hosted mailbox's mail stays.
	if err := h.queries.DeleteEmailsBySource(c.Context(), db.DeleteEmailsBySourceParams{UserID: userID, Source: "gmail"}); err != nil {
		return err
	}
	if err := h.queries.DeleteGmailConnection(c.Context(), userID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"connected": false}})
}

// gmailReady reports whether the Gmail feature is wired (config present).
func (h *inboxHandlers) gmailReady() bool {
	return h.gmailConnector != nil && h.gmailCipher != nil
}

// revokeGmailGrant surrenders the user's Gmail grant at Google, so losing our copy of
// the token is not the only thing standing between us and their mailbox. Shared by
// disconnect and account deletion. A user with no connection — or a deployment with
// Gmail unconfigured — has nothing to revoke, which is success, not failure.
func (h *inboxHandlers) revokeGmailGrant(ctx context.Context, userID int64) error {
	if !h.gmailReady() {
		return nil
	}
	tok, err := h.queries.GetGmailRefreshToken(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	refresh, err := h.gmailCipher.Decrypt(tok.RefreshTokenEnc)
	if err != nil {
		return err
	}
	h.gmailConnector.Revoke(ctx, refresh)
	return nil
}
