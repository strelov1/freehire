package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/gmailsync"
)

// GmailConnect starts the "Connect Gmail" incremental-OAuth flow for the
// signed-in user: it sets a CSRF state cookie and redirects to Google's consent
// screen for gmail.readonly.
func (a *API) GmailConnect(c *fiber.Ctx) error {
	state, err := oauth.NewState()
	if err != nil {
		return err
	}
	oauth.SetStateCookie(c, state, a.cookieSecure)
	return c.Redirect(a.gmailConnector.AuthCodeURL(state), fiber.StatusFound)
}

// GmailCallback finishes the flow: it verifies state, exchanges the code for a
// refresh token + the connected address, stores the token encrypted, and
// redirects back to the inbox. Failures redirect with ?gmail_error (never JSON).
func (a *API) GmailCallback(c *fiber.Ctx) error {
	redirect := func(qs string) error {
		return c.Redirect(a.frontendOrigin+"/my/inbox?"+qs, fiber.StatusFound)
	}
	userID, ok := auth.UserID(c)
	if !ok {
		return redirect("gmail_error=auth")
	}
	cookieState := c.Cookies(oauth.StateCookieName)
	oauth.ClearStateCookie(c, a.cookieSecure)
	if cookieState == "" || c.Query("state") != cookieState {
		return redirect("gmail_error=state")
	}
	if code := c.Query("code"); code != "" {
		refresh, email, err := a.gmailConnector.Exchange(c.Context(), code)
		if err != nil {
			return redirect("gmail_error=exchange")
		}
		enc, err := a.gmailCipher.Encrypt(refresh)
		if err != nil {
			return redirect("gmail_error=exchange")
		}
		if err := a.queries.UpsertGmailConnection(c.Context(), db.UpsertGmailConnectionParams{
			UserID: userID, Email: email, RefreshTokenEnc: enc,
		}); err != nil {
			return redirect("gmail_error=exchange")
		}
	}
	return redirect("gmail=connected")
}

// GmailStatus reports whether the caller has connected Gmail.
func (a *API) GmailStatus(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	conn, err := a.queries.GetGmailConnection(c.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// available signals whether the connect flow is wired (Google creds + token
		// key), so the SPA hides the Connect button when it would 404.
		return c.JSON(fiber.Map{"data": fiber.Map{"connected": false, "available": a.gmailReady()}})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"connected": true, "email": conn.Email, "status": conn.Status, "available": a.gmailReady(),
	}})
}

// SyncGmail triggers an on-demand sync of the caller's ATS mail. It runs in the
// background (a full backfill can exceed the request write timeout) and returns
// immediately; the SPA polls the inbox for results.
func (a *API) SyncGmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	conn, err := a.queries.GetGmailConnection(c.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusBadRequest, "Gmail is not connected")
	}
	if err != nil {
		return err
	}
	worker := gmailsync.NewWorker(gmailsync.NewDBStore(a.queries), a.gmailCipher, a.gmailConnector.ReaderFactory())
	// Background context: the sync outlives this request.
	go worker.SyncUser(context.Background(), gmailsync.Connection{
		UserID: conn.UserID, Email: conn.Email, Cursor: conn.SyncCursor,
	})
	return c.JSON(fiber.Map{"data": fiber.Map{"started": true}})
}

// GmailDisconnect revokes the grant (best-effort) and purges the token and the
// user's synced mail.
func (a *API) GmailDisconnect(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	// Best-effort revoke with Google before purging our copy (only when the
	// feature is wired — otherwise there is nothing to revoke through).
	if a.gmailReady() {
		if tok, err := a.queries.GetGmailRefreshToken(c.Context(), userID); err == nil {
			if refresh, decErr := a.gmailCipher.Decrypt(tok.RefreshTokenEnc); decErr == nil {
				a.gmailConnector.Revoke(c.Context(), refresh)
			}
		}
	}
	if err := a.queries.DeleteUserEmails(c.Context(), userID); err != nil {
		return err
	}
	if err := a.queries.DeleteGmailConnection(c.Context(), userID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"connected": false}})
}

// gmailReady reports whether the Gmail feature is wired (config present).
func (a *API) gmailReady() bool {
	return a.gmailConnector != nil && a.gmailCipher != nil
}
