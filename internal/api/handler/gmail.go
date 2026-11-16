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

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/apptimeline"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/application/mailrecall"
	"github.com/strelov1/freehire/internal/engage/reminder"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// inboxHandlers serves the mail inbox surfaces: the Gmail OAuth flow, the unified inbox
// reads, email-to-application linking and the hosted-mailbox option. gmailConnector and
// gmailCipher are nil when Google creds or the token key are absent (connect routes are
// then unregistered, inbox reads empty). mailDomain is empty when the hosted mailbox is off.
type inboxHandlers struct {
	queries *db.Queries
	// pool backs the one multi-statement write: an ingest batch commits whole or not at all.
	pool           *pgxpool.Pool
	gmailConnector *gmailsync.Connector
	gmailCipher    *tokencrypt.Cipher
	frontendOrigin string
	cookieSecure   bool
	mailDomain     string
	// inbox holds the mail use cases; the assistant's mail tools call the same service, so a
	// rule cannot hold for one reader and not the other.
	inbox *inbox.Service
	// timeline reads the ledger for the application panel's history, riding along with the
	// linked-mail fetch.
	timeline *apptimeline.Service
	// recall is the pull direction (from an application, find its mail). Nil without a
	// model: the endpoint reports the feature off.
	recall *mailrecall.Service
	// llm binds a run to the caller's gateway credential; the zero value spends on the
	// service credential.
	llm llmBinding
	// mailboxes mints the caller's searchable mailbox for a recall run. Nil, or no grant,
	// means the sweep reads stored mail. An interface so tests need no Google credential.
	mailboxes mailboxes
	// plans meters reply drafts. Nil in a deployment with no meter: drafts run uncharged.
	plans *plan.Store
	// accounts resolves the caller's username, which a hosted-mailbox claim adopts as its
	// address. Never nil: it never touches a password, so a plain authHasher backs it.
	accounts *accounts.Service
}

// mailboxes resolves one caller's searchable mailbox, or nil when they have none.
type mailboxes interface {
	For(ctx context.Context, userID int64) mailrecall.Mailbox
}

// withRecall wires the mail-recall action after construction, so the six test harnesses
// that make no model call do not pay for two more constructor parameters.
func (h *inboxHandlers) withRecall(recall *mailrecall.Service, binding llmBinding, boxes mailboxes) *inboxHandlers {
	h.recall, h.llm, h.mailboxes = recall, binding, boxes
	return h
}

// trackingApplications adapts the tracking service to the one call the mail service makes
// of it; inbox does not need the resulting interaction.
type trackingApplications struct{ *jobtracking.Service }

func (t trackingApplications) MarkAppliedAt(ctx context.Context, userID int64, slug string, at time.Time, source string) error {
	_, err := t.Service.MarkAppliedAt(ctx, userID, slug, at, source)
	return err
}

func newInboxHandlers(queries *db.Queries, pool *pgxpool.Pool, gmailConnector *gmailsync.Connector, gmailCipher *tokencrypt.Cipher, frontendOrigin string, cookieSecure bool, mailDomain string) *inboxHandlers {
	// WithReminders, like every other construction of this service, so jobtracking's
	// saved-job reminder cancellation also runs on the mail path.
	tracking := jobtracking.New(
		jobtracking.NewQueriesRepository(queries, pool),
		jobtracking.WithReminders(reminder.New(reminder.NewQueriesRepository(queries))),
	)
	return &inboxHandlers{
		queries:        queries,
		pool:           pool,
		inbox:          inbox.New(queries, trackingApplications{tracking}, inbox.WithIngester(inbox.NewQueriesIngester(pool, queries))),
		timeline:       apptimeline.New(queries),
		gmailConnector: gmailConnector,
		gmailCipher:    gmailCipher,
		frontendOrigin: frontendOrigin,
		cookieSecure:   cookieSecure,
		mailDomain:     mailDomain,
		accounts:       accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
	}
}

func (h *inboxHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Open to every signed-in user. Read and disconnect routes are always registered; the
	// OAuth connect routes only when configured.
	//
	// mw.key, not mw.cookie: a user's own agent harness drives this surface with a full-scope
	// API key, as on the tracker routes. mw.key is full-scope-only, so the narrow `cv` key a
	// tailoring bootstrap mints stays refused. The OAuth connect flow stays browser-bound: it
	// redirects to Google's consent screen, meaningless to a keyed client.
	api.Get("/me/gmail", mw.key, contract)
	api.Delete("/me/gmail", mw.key, contract)
	api.Get("/me/inbox", mw.key, contract)
	api.Post("/me/inbox/read-all", mw.key, contract)
	api.Get("/me/emails/:id", mw.key, contract)
	api.Post("/me/emails/:id/delete", mw.key, contract)
	api.Post("/me/emails/:id/restore", mw.key, contract)
	// Agent surface: a caller's own harness pushes mail it fetched and records its triage verdict.
	api.Post("/me/emails", mw.key, contract)
	api.Post("/me/emails/:id/triage", mw.key, contract)
	// Email-to-application linking. :slug is registered after the static /me/tracking/*
	// routes (see Register) so it does not shadow them.
	api.Get("/me/tracking/:slug", mw.key, contract)
	// The chase for a quiet application: assemble the draft, record that it was sent. A key is
	// admitted as on the apply write (the CLI records applications).
	api.Get("/me/tracking/:slug/followup", mw.key, contract)
	api.Post("/me/tracking/:slug/followup", mw.key, contract)
	api.Post("/me/emails/:id/link", mw.key, contract)
	api.Post("/me/emails/:id/unlink", mw.key, contract)
	api.Post("/me/emails/:id/confirm", mw.key, contract)
	api.Post("/me/emails/:id/reject", mw.key, contract)
	api.Post("/me/emails/:id/application", mw.key, contract)
	// The pull direction acts on one application, so a keyed client may drive it like a browser.
	api.Post("/me/tracking/:slug/mail-recall", mw.key, mw.paidAI(unmeteredAI), contract)
	// Import-and-link for a proposal the sweep found in the mailbox and did not store. Not
	// paid-AI gated: a person pressing Link, not a model call.
	api.Post("/me/tracking/:slug/mail-recall/link", mw.key, contract)
	api.Post("/me/emails/:id/reply-draft", mw.key, mw.paidAI(plan.FeatureMailReply), contract)
	if h.gmailReady() {
		api.Get("/me/gmail/connect", mw.cookie, h.GmailConnect)
		// The callback is the browser returning from Google, so optionalCookie, not cookie:
		// under RequireAuth a session lost in the round-trip would render a JSON 401 into the
		// address bar. GmailCallback answers that case with a redirect.
		api.Get("/me/gmail/callback", mw.optionalCookie, h.GmailCallback)
		// The calendar's own consent, never folded into the mail one: connecting a mailbox must
		// not quietly ask for a diary. Cookie-only, as it redirects a browser to Google.
		api.Get("/me/calendar/connect", mw.cookie, h.CalendarConnect)
		api.Get("/me/calendar/callback", mw.optionalCookie, h.CalendarCallback)
		api.Post("/me/gmail/sync", mw.key, contract)
	}
	// Hosted mailbox: status always available; claim/release only with a receiving domain.
	api.Get("/me/mailbox", mw.key, contract)
	if h.mailboxReady() {
		api.Post("/me/mailbox", mw.key, contract)
		api.Delete("/me/mailbox", mw.key, contract)
	}
}

// gmailStateCookieName carries the Gmail-connect CSRF state. Not oauth.StateCookieName: a
// sign-in in another tab would overwrite the other flow's state.
const gmailStateCookieName = "hire_gmail_state"

// calendarStateCookieName carries the calendar-connect CSRF state, separate from the mail
// one so two consents in flight cannot complete each other.
const calendarStateCookieName = "hire_calendar_state"

// integrationsPath is where both OAuth callbacks land, success or failure: the Integrations
// tab owns connect/disconnect for every third-party account.
const integrationsPath = "/my/integrations"

// GmailConnect starts the incremental-OAuth flow: sets a CSRF state cookie and redirects
// to Google's consent screen for gmail.readonly.
func (h *inboxHandlers) GmailConnect(c *fiber.Ctx) error {
	state, err := oauth.NewState()
	if err != nil {
		return err
	}
	oauth.SetStateCookieNamed(c, gmailStateCookieName, state, h.cookieSecure)
	return c.Redirect(h.gmailConnector.AuthCodeURL(state), fiber.StatusFound)
}

// oauthCallbackFlow holds what differs between "return from Google" callbacks: the CSRF
// cookie, the query-param vocabulary, and the exchange and persist steps. The rest lives
// once, in runOAuthCallback.
type oauthCallbackFlow struct {
	// name identifies the flow in the server-side log only; the SPA reads errorParam.
	name         string
	stateCookie  string
	errorParam   string
	successQuery string
	exchange     func(ctx context.Context, code string) (refreshToken string, scopes []string, err error)
	persist      func(ctx context.Context, userID int64, refreshTokenEnc string, scopes []string) error
}

// errPartialCalendarGrant is returned by the calendar persist step when Google granted only
// part of the consent. The grant is still stored, but the caller is told.
var errPartialCalendarGrant = errors.New("google granted only part of the calendar consent")

// checkCalendarGrant reports errPartialCalendarGrant when a calendar scope is missing (the
// consent screen lets a person untick one). An empty list is not evidence of a partial grant.
func checkCalendarGrant(scopes []string) error {
	if len(scopes) > 0 && (!slices.Contains(scopes, gmailsync.CalendarScope) ||
		!slices.Contains(scopes, gmailsync.CalendarEventsScope)) {
		return errPartialCalendarGrant
	}
	return nil
}

// runOAuthCallback is the shared body of every callback below. A declined consent echoes
// the state with ?error=access_denied instead of a code, so the state check alone is not
// enough: treating a missing code as "nothing to do" would report a refused grant as connected.
func (h *inboxHandlers) runOAuthCallback(c *fiber.Ctx, flow oauthCallbackFlow) error {
	redirect := func(qs string, err error) error {
		log.Printf("%s: %s: %v", flow.name, qs, err)
		return c.Redirect(h.frontendOrigin+integrationsPath+"?"+qs, fiber.StatusFound)
	}
	userID, ok := auth.UserID(c)
	if !ok {
		return redirect(flow.errorParam+"=auth", errors.New("no authenticated user"))
	}
	cookieState := c.Cookies(flow.stateCookie)
	oauth.ClearStateCookieNamed(c, flow.stateCookie, h.cookieSecure)
	if cookieState == "" || c.Query("state") != cookieState {
		return redirect(flow.errorParam+"=state", errors.New("state cookie missing or mismatched"))
	}
	if refusal := c.Query("error"); refusal != "" {
		return redirect(flow.errorParam+"=denied", errors.New(refusal))
	}
	code := c.Query("code")
	if code == "" {
		return redirect(flow.errorParam+"=exchange", errors.New("missing code"))
	}
	refresh, scopes, err := flow.exchange(c.Context(), code)
	if err != nil {
		return redirect(flow.errorParam+"=exchange", err)
	}
	enc, err := h.gmailCipher.Encrypt(refresh)
	if err != nil {
		return redirect(flow.errorParam+"=exchange", err)
	}
	if err := flow.persist(c.Context(), userID, enc, scopes); err != nil {
		if errors.Is(err, errPartialCalendarGrant) {
			return redirect(flow.errorParam+"=partial", err)
		}
		return redirect(flow.errorParam+"=exchange", err)
	}
	return c.Redirect(h.frontendOrigin+integrationsPath+"?"+flow.successQuery, fiber.StatusFound)
}

// GmailCallback finishes the flow: verifies state, exchanges the code for a refresh token
// and the connected address, stores the token encrypted and redirects to the Integrations
// tab. Failures redirect with ?gmail_error (never JSON); the cause is logged first because
// the marker tells the user nothing.
//
// Its exchange yields the address, which rides a closure rather than widening
// oauthCallbackFlow for one caller.
func (h *inboxHandlers) GmailCallback(c *fiber.Ctx) error {
	var email string
	return h.runOAuthCallback(c, oauthCallbackFlow{
		name: "gmail connect", stateCookie: gmailStateCookieName,
		errorParam: "gmail_error", successQuery: "gmail=connected",
		exchange: func(ctx context.Context, code string) (string, []string, error) {
			refresh, addr, scopes, err := h.gmailConnector.Exchange(ctx, code)
			email = addr
			return refresh, scopes, err
		},
		persist: func(ctx context.Context, userID int64, enc string, scopes []string) error {
			return h.saveGmailGrant(ctx, userID, email, enc, scopes)
		},
	})
}

// saveGmailGrant stores a mail grant. Scopes ride in the same statement as the token:
// cal-sync selects connections by their recorded scopes, and a separate write would let a
// concurrent calendar connect pair its token with this grant's scopes.
func (h *inboxHandlers) saveGmailGrant(ctx context.Context, userID int64, email, enc string, scopes []string) error {
	return h.queries.UpsertGmailConnection(ctx, db.UpsertGmailConnectionParams{
		UserID: userID, Email: email, RefreshTokenEnc: enc, Scopes: scopes,
	})
}

// CalendarConnect starts the calendar consent, with its own state cookie so two flows in
// flight cannot complete each other.
func (h *inboxHandlers) CalendarConnect(c *fiber.Ctx) error {
	state, err := oauth.NewState()
	if err != nil {
		return err
	}
	oauth.SetStateCookieNamed(c, calendarStateCookieName, state, h.cookieSecure)
	return c.Redirect(h.gmailConnector.CalendarAuthCodeURL(state), fiber.StatusFound)
}

// CalendarCallback finishes it: verify state, exchange the code, store the grant and its
// scopes. Failures redirect with ?calendar_error and are logged first, as in the mail flow.
// It lands on Integrations, where the connect was started.
func (h *inboxHandlers) CalendarCallback(c *fiber.Ctx) error {
	return h.runOAuthCallback(c, oauthCallbackFlow{
		name: "calendar connect", stateCookie: calendarStateCookieName,
		errorParam: "calendar_error", successQuery: "calendar=connected",
		exchange: h.gmailConnector.ExchangeCalendar,
		persist: func(ctx context.Context, userID int64, enc string, scopes []string) error {
			// What Google says the grant covers, not what we asked for: they differ when someone
			// declines part of a consent, and every worker's filter reads this record.
			if err := h.queries.UpsertCalendarGrant(ctx, db.UpsertCalendarGrantParams{
				UserID: userID, RefreshTokenEnc: enc, Scopes: scopes,
			}); err != nil {
				return err
			}
			// The one calendar consent covers a mentor's busy-time sync too. The flag stays because
			// grants made before the consents merged may hold calendar.readonly for interviews
			// alone; only cmd/mentor-busy-sync reads it, for mentors.
			if err := h.queries.SetMentorBusySyncOptedIn(ctx, userID); err != nil {
				return err
			}
			return checkCalendarGrant(scopes)
		},
	})
}

// GmailStatus reports whether the caller has connected Gmail.
func (h *inboxHandlers) GmailStatus(ctx context.Context) (*apigen.GmailStatusOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := h.queries.GetGmailConnection(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// available says whether the connect flow is wired, so the SPA hides a button that would 404.
		return &apigen.GmailStatusOK{Data: apigen.GmailStatus{Available: h.gmailReady()}}, nil
	}
	if err != nil {
		return nil, err
	}
	return &apigen.GmailStatusOK{Data: apigen.GmailStatus{
		// The row's existence is not enough: UpsertCalendarGrant inserts this same row with an
		// empty address for a calendar-only consent (see ListConnectedGmailUsers' `email <> ''`
		// guard), and that must not show the Mail card as connected.
		Connected: conn.Email != "", Email: apigen.NewOptString(conn.Email), Status: apigen.NewOptString(conn.Status), Available: h.gmailReady(),
		// Read from the recorded scopes, not the row: mail and calendar are separate consents,
		// and a calendar grant may have no mailbox behind it.
		CalendarConnected: slices.Contains(conn.Scopes, gmailsync.CalendarScope),
		// Whether the grant covers calendar.events, the write scope mentor-google-meet-link
		// reads for auto-generated Meet links. Gated on status too: a needs_reconsent grant is
		// not connected (see mentorship.GetMentorCalendarGrant), or bookings would silently
		// land with no link.
		MentorCalendarConnected: conn.Status == "connected" && slices.Contains(conn.Scopes, gmailsync.CalendarEventsScope),
		// Whether the grant covers busy-sync: needs the opt-in flag AND the scope AND status,
		// since a pre-merge grant may hold calendar.readonly for interviews alone (see
		// CalendarCallback).
		MentorBusySyncConnected: conn.MentorBusySyncOptedIn && conn.Status == "connected" && slices.Contains(conn.Scopes, gmailsync.CalendarScope),
	}}, nil
}

// SyncGmail triggers an on-demand sync of the caller's ATS mail in the background (a full
// backfill can exceed the request write timeout); the SPA polls the inbox.
func (h *inboxHandlers) SyncGmail(ctx context.Context) (*apigen.SyncGmailOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := h.queries.GetGmailConnection(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Gmail is not connected")
	}
	if err != nil {
		return nil, err
	}
	gmailStore := gmailsync.NewDBStore(h.queries)
	worker := gmailsync.NewWorker(gmailStore, h.gmailCipher, h.gmailConnector.ReaderFactory()).WithLearnedDomains(gmailStore)
	// Background context: the sync outlives this request.
	go worker.SyncUser(context.Background(), gmailsync.Connection{
		UserID: conn.UserID, Email: conn.Email, Cursor: conn.SyncCursor, SentCursor: conn.SentSyncCursor,
	})
	return &apigen.SyncGmailOK{Data: apigen.GmailSyncStarted{Started: true}}, nil
}

// GmailDisconnect revokes the grant (best-effort) and purges the token and the
// user's synced mail.
func (h *inboxHandlers) GmailDisconnect(ctx context.Context) (*apigen.GmailDisconnectOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	// Best-effort revoke with Google before purging our copy.
	if err := h.revokeGmailGrant(ctx, userID); err != nil {
		log.Printf("gmail disconnect: revoke for user %d: %v", userID, err)
	}
	// Purge only this user's Gmail-sourced mail; a hosted mailbox's mail stays.
	if err := h.queries.DeleteEmailsBySource(ctx, db.DeleteEmailsBySourceParams{UserID: userID, Source: "gmail"}); err != nil {
		return nil, err
	}
	if err := h.queries.DeleteGmailConnection(ctx, userID); err != nil {
		return nil, err
	}
	return &apigen.GmailDisconnectOK{Data: apigen.GmailDisconnected{Connected: false}}, nil
}

// gmailReady reports whether the Gmail feature is wired (config present).
func (h *inboxHandlers) gmailReady() bool {
	return h.gmailConnector != nil && h.gmailCipher != nil
}

// revokeGmailGrant surrenders the user's Gmail grant at Google, shared by disconnect and
// account deletion. No connection, or Gmail unconfigured, is success.
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
