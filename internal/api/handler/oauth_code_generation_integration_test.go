//go:build integration

// Integration tests for the security invariant behind the authorization code's
// generation latch (0233): the authority chain must be session claim -> code
// generation -> consumed row -> grant, with every hop refusing a stale
// generation. The race cases interleave a revocation through the REAL handler
// path by wrapping the queries' DBTX — the same seam a pgx pool occupies — so
// the bump lands deterministically inside the exchange's consume and inside
// consent's insert, with no production-only test hook. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
)

// exchangeTestCode POSTs the authorization_code grant to a bare fiber app
// mounting only the token handler, and returns the response.
func exchangeTestCode(t *testing.T, h *authHandlers, code, clientID, redirectURI, verifier string) (*http.Response, tokenResponse) {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/token", h.OAuthToken)
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID},
		"redirect_uri": {redirectURI}, "code_verifier": {verifier},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("token exchange: app.Test: %v", err)
	}
	defer resp.Body.Close()
	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("token exchange: decode: %v", err)
	}
	return resp, out
}

// revokingDBTX passes every call through to the pool except ONE consume: the
// first DELETE of an authorization code has its row interposed so that, the
// moment the handler's Scan sees the code, the account's generation is bumped —
// deterministically INSIDE the window between the exchange's consume and its
// grant INSERT. No production code knows the hook exists.
type revokingDBTX struct {
	pool *pgxpool.Pool
	// fire runs between the consume's row read and the Scan returning.
	fire func()
	once bool
}

func (r *revokingDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return r.pool.Exec(ctx, sql, args...)
}
func (r *revokingDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return r.pool.Query(ctx, sql, args...)
}
func (r *revokingDBTX) CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, rows pgx.CopyFromSource) (int64, error) {
	return r.pool.CopyFrom(ctx, table, columns, rows)
}
func (r *revokingDBTX) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return r.pool.SendBatch(ctx, b)
}
func (r *revokingDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := r.pool.QueryRow(ctx, sql, args...)
	if strings.Contains(sql, "DELETE FROM oauth_authorization_codes") && !r.once {
		r.once = true
		return &revokedCodeRow{row: row, fire: r.fire}
	}
	return row
}

// revokedCodeRow reads the consume's fields, fires the interleaved revocation,
// and only then hands the row to the handler.
type revokedCodeRow struct {
	row    pgx.Row
	fire   func()
	read   bool
	client string
	user   int64
	redir  string
	chal   string
	ver    pgtype.Int4
	res    pgtype.Text
}

func (r *revokedCodeRow) Scan(dest ...any) error {
	if !r.read {
		if err := r.row.Scan(&r.client, &r.user, &r.redir, &r.chal, &r.ver, &r.res); err != nil {
			return err
		}
		r.fire()
		r.read = true
	}
	if len(dest) != 6 {
		return pgx.ErrNoRows
	}
	*(dest[0].(*string)) = r.client
	*(dest[1].(*int64)) = r.user
	*(dest[2].(*string)) = r.redir
	*(dest[3].(*string)) = r.chal
	*(dest[4].(*pgtype.Int4)) = r.ver
	*(dest[5].(*pgtype.Text)) = r.res
	return nil
}

// bumpTestGeneration advances the account's users.token_version the way every
// generation-changing revocation does (logout-all does exactly this; ResetPassword
// and SeizeUnverifiedAccount bump it with extra deletes in the same statement).
func bumpTestGeneration(t *testing.T, q *db.Queries, userID int64) int32 {
	t.Helper()
	bumped, err := q.BumpUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("BumpUserTokenVersion: %v", err)
	}
	return bumped
}

// liveGenerationLoader resolves the account's REAL current generation, unlike
// the package's constant testVersions stub: the generation-asserting tests here
// bump the account between approvals, so the session middleware must validate
// against what the database actually says and the cookie must carry that same
// claim — a constant stub would either refuse the bumped account or mint a
// stale claim this file's assertions would misread.
type liveGenerationLoader struct{ queries *db.Queries }

func (l liveGenerationLoader) GetUserTokenVersion(ctx context.Context, id int64) (int32, error) {
	return l.queries.GetUserTokenVersion(ctx, id)
}

// approveTestAuthorizationAtCurrentGeneration drives the real authorize-submit
// handler (decision=allow) behind a middleware whose loader reads the account's
// real current generation, and mints the session cookie at exactly that claim.
// Generation-1 tests keep the shared testTokenVersion helper; everything here
// that bumps the account must approve through this one.
func approveTestAuthorizationAtCurrentGeneration(t *testing.T, queries *db.Queries, h *authHandlers, iss *auth.Issuer, userID int64, clientID, redirectURI, challenge string) string {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(auth.OptionalCookieAuth(iss, liveGenerationLoader{queries: queries}))
	app.Post("/api/v1/oauth/authorize", h.OAuthAuthorizeSubmit)

	claim, err := queries.GetUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	cookie, err := iss.Issue(userID, claim)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	form := url.Values{
		"decision": {"allow"}, "client_id": {clientID}, "redirect_uri": {redirectURI},
		"state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("approve: app.Test: %v", err)
	}
	defer resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("approve: parse Location: %v", err)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("approve: redirect carried no code")
	}
	return code
}

// The race: a revocation lands AFTER the exchange consumed the code but BEFORE
// the grant INSERT. The grant must carry the consumed code's generation — the
// dead one — so it exists but authenticates nothing. The exchange itself must
// still succeed: the code was live when it was consumed.
func TestOAuthToken_RaceRevocationBetweenConsumeAndGrant(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "token-race@example.test")
	seedOAuthTestClient(t, queries, "client-race", []string{"http://127.0.0.1:9999/callback"})
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss}

	verifier := "a-high-entropy-verifier-the-client-generated-race11"
	code := approveTestAuthorization(t, h, iss, userID, "client-race", "http://127.0.0.1:9999/callback", challengeForVerifier(verifier))
	oldGen, err := queries.GetUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}

	// The same handler, over an intercepted DBTX: consume -> bump -> grant INSERT.
	intercepted := &revokingDBTX{pool: pool, fire: func() {
		if _, err := queries.BumpUserTokenVersion(context.Background(), userID); err != nil {
			t.Errorf("interleaved bump: %v", err)
		}
	}}
	racing := &authHandlers{queries: db.New(intercepted), issuer: iss}

	resp, out := exchangeTestCode(t, racing, code, "client-race", "http://127.0.0.1:9999/callback", verifier)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("exchange under interleave: status = %d, want 200 (code was live at consume)", resp.StatusCode)
	}
	if out.AccessToken == "" {
		t.Fatal("exchange minted no token")
	}

	// The grant the handler just created carries the DEAD generation, so it can
	// never authenticate — exactly what a revocation that lost the race must
	// guarantee.
	if _, err := queries.AuthenticateOAuthGrant(context.Background(), db.AuthenticateOAuthGrantParams{AccessTokenHash: oauth2server.HashToken(out.AccessToken)}); err == nil {
		t.Fatal("grant minted after an interleaved revocation authenticates; the consumed code's generation was upgraded")
	}
	var issued int32
	if err := pool.QueryRow(context.Background(),
		`SELECT issued_token_version FROM oauth_grants WHERE user_id = $1`, userID).Scan(&issued); err != nil {
		t.Fatalf("read grant generation: %v", err)
	}
	if issued != oldGen {
		t.Errorf("grant issued_token_version = %d, want the consumed code's %d", issued, oldGen)
	}
}

// issuingDBTX passes every call through to the pool except the FIRST code
// INSERT (or, failing that, the handler's first generation read), which it
// precedes with a generation bump — the revocation landing between the
// middleware's session validation and the handler's authority write.
type issuingDBTX struct {
	pool    *pgxpool.Pool
	userID  int64
	queries *db.Queries
	fired   bool
	bumpErr error
}

func (i *issuingDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "INSERT INTO oauth_authorization_codes") && !i.fired {
		i.fired = true
		if _, err := i.queries.BumpUserTokenVersion(ctx, i.userID); err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	return i.pool.Exec(ctx, sql, args...)
}
func (i *issuingDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return i.pool.Query(ctx, sql, args...)
}
func (i *issuingDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	// Cover the fresh-read regression path too: a handler stamping a fresh
	// GetUserTokenVersion would read the bumped generation here. Match on the
	// statement's NAME marker, not a hand-copied line — the generated SQL is
	// multi-line, and a stale literal would silently miss the hook.
	if strings.Contains(sql, "-- name: GetUserTokenVersion") && !i.fired {
		i.fired = true
		// Mirroring db.New's wrapper shape: run the bump against the unwrapped
		// queries, then let the read proceed (through the pool) so a reverted
		// fresh-read handler observes the POST-bump generation and fails the
		// stored-code assertion. The bump's own failure still must not vanish.
		if _, err := i.queries.BumpUserTokenVersion(ctx, i.userID); err != nil {
			i.bumpErr = err
		}
	}
	return i.pool.QueryRow(ctx, sql, args...)
}
func (i *issuingDBTX) CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, rows pgx.CopyFromSource) (int64, error) {
	return i.pool.CopyFrom(ctx, table, columns, rows)
}
func (i *issuingDBTX) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return i.pool.SendBatch(ctx, b)
}

// Issuance interleaving: a revocation lands AFTER the session middleware
// validated the consent request but BEFORE the code INSERT. The stored code
// must carry the session's tv claim (the old generation), never the post-bump
// current one — pinning the session-claim binding the whole chain rests on.
func TestOAuthAuthorizeSubmit_RaceRevocationBeforeCodeInsert(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "token-issrace@example.test")
	seedOAuthTestClient(t, queries, "client-issrace", []string{"http://127.0.0.1:9999/callback"})
	iss := auth.NewIssuer("test-secret", time.Hour)
	claimGen, err := queries.GetUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}

	// Approve through REAL middleware validating against the REAL loader: the
	// session and its tv claim are resolved (against the unwrapped queries)
	// before the handler's wrapped ones run, so the claim is genuinely claimGen.
	// Only the handler's queries carry the interception, so the bump fires at
	// the handler's own authority write (INSERT — or, for a reverted fresh-read
	// handler, its GetUserTokenVersion read, which then observes the POST-bump
	// generation and fails the stored-code assertion below).
	// Source-verified ordering: the reverted handler would read only AFTER
	// requireUserID and SessionVersion, i.e. strictly after the middleware's own
	// validation, so the bump at that read can never corrupt the claim itself.
	intercepted := &issuingDBTX{pool: pool, userID: userID, queries: queries}
	consenting := &authHandlers{queries: db.New(intercepted), issuer: iss}

	verifier := "a-high-entropy-verifier-the-client-generated-issr22"
	code := approveTestAuthorizationAtCurrentGeneration(t, queries, consenting, iss, userID, "client-issrace", "http://127.0.0.1:9999/callback", challengeForVerifier(verifier))
	if !intercepted.fired {
		t.Fatal("the interleaved bump never ran; the interception missed the INSERT")
	}
	if intercepted.bumpErr != nil {
		t.Fatalf("interleaved bump: %v", intercepted.bumpErr)
	}

	var stored int32
	if err := pool.QueryRow(context.Background(),
		`SELECT issued_token_version FROM oauth_authorization_codes WHERE user_id = $1`, userID).Scan(&stored); err != nil {
		t.Fatalf("read code generation: %v", err)
	}
	if stored != claimGen {
		t.Errorf("code issued_token_version = %d, want the session claim %d (a fresh read would have written %d)", stored, claimGen, claimGen+1)
	}

	// And the chain holds end to end: the account is now one generation ahead of
	// the code, so the real token exchange refuses it.
	h := &authHandlers{queries: queries, issuer: iss}
	resp, out := exchangeTestCode(t, h, code, "client-issrace", "http://127.0.0.1:9999/callback", verifier)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("exchange of the pre-revocation code: status = %d, want 400 (token %q)", resp.StatusCode, out.AccessToken)
	}
}

// Consent minted AFTER a revocation still works end to end, and the resulting
// grant carries the CURRENT generation so a later revocation reaches it.
func TestOAuthToken_AcceptsCodeApprovedAfterRevocation(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "token-latch-2@example.test")
	seedOAuthTestClient(t, queries, "client-latch-2", []string{"http://127.0.0.1:9999/callback"})
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss}

	bumpTestGeneration(t, queries, userID)

	verifier := "a-high-entropy-verifier-the-client-generated-latc11"
	code := approveTestAuthorizationAtCurrentGeneration(t, queries, h, iss, userID, "client-latch-2", "http://127.0.0.1:9999/callback", challengeForVerifier(verifier))

	resp, out := exchangeTestCode(t, h, code, "client-latch-2", "http://127.0.0.1:9999/callback", verifier)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("post-revocation code: status = %d, want 200", resp.StatusCode)
	}
	if out.AccessToken == "" {
		t.Fatal("post-revocation code minted no access token")
	}
	// Exactly one grant, stamped with the account's CURRENT generation.
	current, err := queries.GetUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	var issued int32
	if err := pool.QueryRow(context.Background(),
		`SELECT issued_token_version FROM oauth_grants WHERE user_id = $1`, userID).Scan(&issued); err != nil {
		t.Fatalf("read grant generation: %v", err)
	}
	if issued != current {
		t.Errorf("grant issued_token_version = %d, want the current %d", issued, current)
	}
	if _, err := queries.AuthenticateOAuthGrant(context.Background(), db.AuthenticateOAuthGrantParams{AccessTokenHash: oauth2server.HashToken(out.AccessToken)}); err != nil {
		t.Fatalf("fresh grant does not authenticate: %v", err)
	}
}

// A code approved BEFORE a revocation is refused at the exchange and left
// standing dead — no retry can revive it.
func TestOAuthToken_RefusesCodeApprovedBeforeRevocation(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "token-latch@example.test")
	seedOAuthTestClient(t, queries, "client-latch", []string{"http://127.0.0.1:9999/callback"})
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss}

	verifier := "a-high-entropy-verifier-the-client-generated-latch00"
	stale := approveTestAuthorization(t, h, iss, userID, "client-latch", "http://127.0.0.1:9999/callback", challengeForVerifier(verifier))

	bumpTestGeneration(t, queries, userID)

	resp, out := exchangeTestCode(t, h, stale, "client-latch", "http://127.0.0.1:9999/callback", verifier)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("pre-revocation code: status = %d, want 400 (body access_token %q)", resp.StatusCode, out.AccessToken)
	}
	if out.AccessToken != "" {
		t.Fatal("pre-revocation code minted an access token")
	}
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count codes: %v", err)
	}
	if n != 1 {
		t.Errorf("%d pending code rows after the refused exchange, want 1 (standing dead)", n)
	}
	// A retry re-derives the same refusal — the row's generation never matches again.
	resp2, out2 := exchangeTestCode(t, h, stale, "client-latch", "http://127.0.0.1:9999/callback", verifier)
	defer resp2.Body.Close()
	if resp2.StatusCode != fiber.StatusBadRequest || out2.AccessToken != "" {
		t.Fatal("retry of the dead code did not stay refused")
	}
}
