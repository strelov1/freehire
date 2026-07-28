package handler

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/tokencrypt"
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
}

func newInboxHandlers(queries *db.Queries, pool *pgxpool.Pool, gmailConnector *gmailsync.Connector, gmailCipher *tokencrypt.Cipher, frontendOrigin string, cookieSecure bool, mailDomain string) *inboxHandlers {
	return &inboxHandlers{
		queries:        queries,
		pool:           pool,
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
	// The OAuth connect flow is the exception and stays cookie-only: it redirects
	// a browser to Google's consent screen and back, so it is meaningless to a
	// keyed client and must not be reachable by one.
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
	api.Post("/me/emails/:id/link", mw.key, h.LinkEmail)
	api.Post("/me/emails/:id/unlink", mw.key, h.UnlinkEmail)
	api.Post("/me/emails/:id/confirm", mw.key, h.ConfirmEmailLink)
	api.Post("/me/emails/:id/reject", mw.key, h.RejectEmailLink)
	if h.gmailReady() {
		api.Get("/me/gmail/connect", mw.cookie, h.GmailConnect)
		api.Get("/me/gmail/callback", mw.cookie, h.GmailCallback)
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
// redirects back to the inbox. Failures redirect with ?gmail_error (never JSON);
// the underlying cause is logged server-side first (like oauthFail), since the
// generic redirect marker tells the user nothing.
func (h *inboxHandlers) GmailCallback(c *fiber.Ctx) error {
	redirect := func(qs string, err error) error {
		log.Printf("gmail connect: %s: %v", qs, err)
		return c.Redirect(h.frontendOrigin+"/my/inbox?"+qs, fiber.StatusFound)
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
		refresh, email, err := h.gmailConnector.Exchange(c.Context(), code)
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
	}
	return c.Redirect(h.frontendOrigin+"/my/inbox?gmail=connected", fiber.StatusFound)
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
		return c.JSON(fiber.Map{"data": fiber.Map{"connected": false, "available": h.gmailReady()}})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"connected": true, "email": conn.Email, "status": conn.Status, "available": h.gmailReady(),
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
