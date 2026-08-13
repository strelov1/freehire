package handler

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/discordbot"
)

// discordApp mounts the interaction webhook on an enabled handler signing with
// the given Ed25519 key pair. No DB is needed: the signature guard, PING, and
// an invalid-token /link reply never reach the queries field.
func discordApp(pub ed25519.PublicKey) (*fiber.App, *discordHandlers) {
	h := &discordHandlers{
		discordBot:       discordbot.NewClient("bottoken"),
		discordLinks:     discordbot.NewDiscordLinkTokens("test-secret", 10*time.Minute),
		discordPublicKey: hex.EncodeToString(pub),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/discord/interactions", h.DiscordInteraction)
	return app, h
}

// signedRequest builds an interaction POST with a valid Ed25519 signature over
// timestamp||body, as Discord computes it.
func signedRequest(t *testing.T, priv ed25519.PrivateKey, timestamp string, body []byte) *http.Request {
	t.Helper()
	sig := ed25519.Sign(priv, append([]byte(timestamp), body...))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/discord/interactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
	req.Header.Set("X-Signature-Timestamp", timestamp)
	return req
}

func TestDiscordInteraction_missingSignatureForbidden(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := discordApp(pub)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/discord/interactions", bytes.NewReader([]byte(`{"type":1}`)))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestDiscordInteraction_invalidSignatureForbidden(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	// Sign with a DIFFERENT key than the one the handler verifies against.
	_, otherPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := discordApp(pub)

	body := []byte(`{"type":1}`)
	req := signedRequest(t, otherPriv, "1700000000", body)
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

// TestDiscordInteraction_tamperedBodyForbidden guards that the signature covers
// the RAW body actually processed: a body that would parse fine but does not
// match the signed bytes must still be rejected before any JSON parsing.
func TestDiscordInteraction_tamperedBodyForbidden(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := discordApp(pub)

	signedBody := []byte(`{"type":1}`)
	sig := ed25519.Sign(priv, append([]byte("1700000000"), signedBody...))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/discord/interactions", bytes.NewReader([]byte(`{"type":2}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
	req.Header.Set("X-Signature-Timestamp", "1700000000")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestDiscordInteraction_ping(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := discordApp(pub)

	req := signedRequest(t, priv, "1700000000", []byte(`{"type":1}`))
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["type"] != float64(1) {
		t.Errorf("body = %+v, want type=1 (PONG)", out)
	}
}

// TestDiscordInteraction_linkCommand_invalidToken exercises command routing
// down to the /link handler with a garbage token. Parse fails before any DB
// access, so this is safe to run against a handler with nil queries — a panic
// here would mean the code tried to touch the DB despite the invalid token.
func TestDiscordInteraction_linkCommand_invalidToken(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := discordApp(pub)

	body, err := json.Marshal(discordbot.Interaction{
		Type: discordbot.InteractionTypeApplicationCommand,
		Data: &discordbot.InteractionData{
			Name:    "link",
			Options: []discordbot.InteractionOption{{Name: "token", Value: "garbage-token"}},
		},
		Member: &discordbot.Member{User: &discordbot.User{ID: "123456789"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := signedRequest(t, priv, "1700000000", body)
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (interaction responses are always 200)", res.StatusCode)
	}
	var out discordbot.Response
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Type != discordbot.ResponseTypeChannelMessageWithSource {
		t.Errorf("response type = %d, want %d (channel message)", out.Type, discordbot.ResponseTypeChannelMessageWithSource)
	}
	if out.Data == nil || out.Data.Flags != discordbot.FlagEphemeral {
		t.Errorf("response data = %+v, want ephemeral flag set", out.Data)
	}
}

// TestDiscordInteraction_unknownCommand guards that an unrecognized command
// name replies with a generic ephemeral error instead of panicking on an
// unhandled shape.
func TestDiscordInteraction_unknownCommand(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := discordApp(pub)

	body, err := json.Marshal(discordbot.Interaction{
		Type: discordbot.InteractionTypeApplicationCommand,
		Data: &discordbot.InteractionData{Name: "bogus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := signedRequest(t, priv, "1700000000", body)
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out discordbot.Response
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Data == nil || out.Data.Flags != discordbot.FlagEphemeral {
		t.Errorf("response data = %+v, want ephemeral flag set", out.Data)
	}
}

// TestNewDiscordHandlers_allOrNothingGate checks that the constructor's enable
// condition is genuinely all-or-nothing: any one of the four Discord config
// values missing must leave the feature disabled, not partially wired.
func TestNewDiscordHandlers_allOrNothingGate(t *testing.T) {
	full := []string{"token", "app-id", "pub-key", "guild-id"}
	tests := []struct {
		name                             string
		botToken, appID, pubKey, guildID string
	}{
		{"fully configured", full[0], full[1], full[2], full[3]},
		{"missing bot token", "", full[1], full[2], full[3]},
		{"missing application id", full[0], "", full[2], full[3]},
		{"missing public key", full[0], full[1], "", full[3]},
		{"missing guild id", full[0], full[1], full[2], ""},
		{"nothing configured", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDiscordHandlers(nil, "jwt-secret", tt.botToken, tt.appID, tt.pubKey, tt.guildID, "", nil)
			want := tt.name == "fully configured"
			if got := h.discordEnabled(); got != want {
				t.Errorf("discordEnabled() = %v, want %v", got, want)
			}
		})
	}
}

func TestDiscordInteraction_disabledReturns404(t *testing.T) {
	h := &discordHandlers{} // no config → disabled
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/discord/interactions", h.DiscordInteraction)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/discord/interactions", bytes.NewReader([]byte(`{"type":1}`)))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// TestDiscordInteraction_contributeMissingIdentityEphemeral exercises /contribute's response
// to an interaction that carries no Member/User: handleContributeCommand resolves the caller's
// identity SYNCHRONOUSLY, before deciding whether to defer, so an unidentified caller gets an
// immediate ephemeral reply (type 4, "could not identify") rather than a deferred response —
// there is nothing left to defer for, since intake.Resolve is never reached. See
// TestNoAnonymousContribution_missingIdentityNeverReachesIntake for the same property asserted
// directly against the dispatch code.
func TestDiscordInteraction_contributeMissingIdentityEphemeral(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := discordApp(pub)

	body, err := json.Marshal(discordbot.Interaction{
		Type: discordbot.InteractionTypeApplicationCommand,
		Data: &discordbot.InteractionData{
			Name:    "contribute",
			Options: []discordbot.InteractionOption{{Name: "url", Value: "https://boards.example.com/co/jobs/123"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := signedRequest(t, priv, "1700000000", body)
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out discordbot.Response
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Type != discordbot.ResponseTypeChannelMessageWithSource {
		t.Errorf("response type = %d, want %d (immediate channel message, not deferred)", out.Type, discordbot.ResponseTypeChannelMessageWithSource)
	}
	if out.Data == nil || out.Data.Flags != discordbot.FlagEphemeral {
		t.Errorf("response data = %+v, want ephemeral flag set", out.Data)
	}
}

// TestCommandOption_readsURL is the option-parsing unit the /contribute handler relies on:
// commandOption must find the "url" option by name among however many options a command
// invocation carries, and return "" when it's absent rather than panicking.
func TestCommandOption_readsURL(t *testing.T) {
	data := &discordbot.InteractionData{
		Name: "contribute",
		Options: []discordbot.InteractionOption{
			{Name: "other", Value: "ignored"},
			{Name: "url", Value: "https://boards.example.com/co/jobs/123"},
		},
	}
	if got := commandOption(data, "url"); got != "https://boards.example.com/co/jobs/123" {
		t.Errorf("commandOption(url) = %q, want the url option's value", got)
	}
	if got := commandOption(&discordbot.InteractionData{Name: "contribute"}, "url"); got != "" {
		t.Errorf("commandOption(url) on a command with no options = %q, want \"\"", got)
	}
}

// TestNoAnonymousContribution_missingIdentityNeverReachesIntake covers ONE of the two ways
// /contribute can face an unlinked caller: the interaction itself carries no Member/User, so
// interactionUserID returns ok=false and handleContributeCommand takes its first early return
// (discord.go's "could not identify your Discord account" branch) — synchronously, before ever
// spawning the background goroutine, and without ever touching h.queries or h.intake. This test
// drives handleContributeCommand directly (rather than processDiscordContribution — there is no
// goroutine on this path anymore to bypass) against a handler whose queries and intake fields
// are nil: a call to either would panic, so completing without panic is proof neither was
// reached, and the response type confirms it was answered immediately rather than deferred.
//
// It does NOT cover the other, more common way a caller is unlinked: an interaction that DOES
// identify a Discord account, but GetUserIDByDiscordID returns pgx.ErrNoRows because that
// account was never linked to a freehire user (discord.go's second early return). h.queries is
// a concrete *db.Queries, not an interface, so exercising that branch needs a real Postgres
// connection to return ErrNoRows from — there is no seam to stub it in a unit test. That branch
// is proven only by reading the code (it returns before reaching h.intake.Resolve, same as this
// one) plus the DB-backed integration coverage in discord_integration_test.go.
func TestNoAnonymousContribution_missingIdentityNeverReachesIntake(t *testing.T) {
	h := &discordHandlers{
		// queries and intake are deliberately nil: GetUserIDByDiscordID or intake.Resolve
		// would panic on a nil receiver/field, so this is a hard guard, not just an assertion.
	}
	interaction := discordbot.Interaction{
		Type: discordbot.InteractionTypeApplicationCommand,
		Data: &discordbot.InteractionData{
			Name:    "contribute",
			Options: []discordbot.InteractionOption{{Name: "url", Value: "https://boards.example.com/co/jobs/123"}},
		},
		// No Member, no User: interactionUserID reports ok=false.
	}
	discordID, ok := interactionUserID(interaction)
	if ok {
		t.Fatalf("interactionUserID = (%d, true), want ok=false for an interaction with no Member/User", discordID)
	}

	// Calls the handler directly rather than through app.Test — this must return without
	// panicking despite nil queries/intake, which is the proof this path never dereferences
	// either.
	c := fiberCtx()
	if err := h.handleContributeCommand(c, interaction); err != nil {
		t.Fatalf("handleContributeCommand: %v", err)
	}
	var out discordbot.Response
	if err := json.Unmarshal(c.Response().Body(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Type != discordbot.ResponseTypeChannelMessageWithSource {
		t.Errorf("response type = %d, want %d (immediate channel message, not deferred)", out.Type, discordbot.ResponseTypeChannelMessageWithSource)
	}
	if out.Data == nil || out.Data.Flags != discordbot.FlagEphemeral {
		t.Errorf("response data = %+v, want ephemeral flag set", out.Data)
	}
}

// TestRenderIntakeOutcome is the "no regression from the extraction" check: renderIntakeOutcome
// was pulled out of telegramHandlers.intakeReply verbatim, so this locks its wording — read
// directly off the pre-extraction switch — for every intakeOutcome.Status value, across both
// surfaces that now call it.
func TestRenderIntakeOutcome(t *testing.T) {
	const origin = "https://freehire.me"
	tests := []struct {
		name string
		out  intakeOutcome
		// want is renderIntakeOutcome's output under telegramEmphasize (HTML) — read directly
		// off the pre-extraction switch, so it also proves Telegram's exact prior wording is
		// unchanged.
		want string
		// wantMarkdown is the output under discordEmphasize; empty means "identical to want" —
		// true for every branch except outcomeQueued/Rewarded, the only one with an emphasis
		// span.
		wantMarkdown string
	}{
		{
			name: "found",
			out:  intakeOutcome{Status: outcomeFound, PublicSlug: "acme-swe"},
			want: "👍 We already have this one:\nhttps://freehire.me/jobs/acme-swe",
		},
		{
			name: "tracked",
			out:  intakeOutcome{Status: outcomeTracked, PublicSlug: "acme-swe", CompanySlug: "acme"},
			want: "✅ Added — and we already track this company, so the rest of its roles will follow on the next crawl.\n" +
				"https://freehire.me/jobs/acme-swe\nhttps://freehire.me/companies/acme",
		},
		{
			name: "imported with known company",
			out:  intakeOutcome{Status: outcomeImported, PublicSlug: "acme-swe", CompanySlug: "acme"},
			want: "✅ Added — we already carry this company, and now we'll crawl this board of theirs too.\n" +
				"https://freehire.me/jobs/acme-swe\nhttps://freehire.me/companies/acme",
		},
		{
			name: "imported with new company",
			out:  intakeOutcome{Status: outcomeImported, PublicSlug: "acme-swe"},
			want: "🎉 Added, and this company is new to us — we'll start crawling its board.\nhttps://freehire.me/jobs/acme-swe",
		},
		{
			name: "review with known company",
			out:  intakeOutcome{Status: outcomeReview, PublicSlug: "acme-swe", CompanySlug: "acme"},
			want: "✅ Added — we already carry this company. Its careers site isn't one we can crawl yet, so we'll look at it by hand.\n" +
				"https://freehire.me/jobs/acme-swe\nhttps://freehire.me/companies/acme",
		},
		{
			name: "review with new company",
			out:  intakeOutcome{Status: outcomeReview, PublicSlug: "acme-swe"},
			want: "✅ Added. Its careers site isn't one we can crawl yet — we'll check by hand whether we can pull the rest of its jobs.\n" +
				"https://freehire.me/jobs/acme-swe",
		},
		{
			name: "queued and rewarded",
			out:  intakeOutcome{Status: outcomeQueued, Board: "Acme <Careers>", Rewarded: true},
			want: "🎉 We couldn't open that page, but <b>Acme &lt;Careers&gt;</b> is a company we don't crawl yet — added to the queue. +1 AI credit!",
			wantMarkdown: "🎉 We couldn't open that page, but **Acme <Careers>** is a company we don't crawl yet — " +
				"added to the queue. +1 AI credit!",
		},
		{
			name: "queued but board already known",
			out:  intakeOutcome{Status: outcomeQueued, Board: "Acme", Rewarded: false},
			want: "👍 We couldn't open that page, but that company's board is already known to us — nothing to add.",
		},
		{
			name: "queued with no board recognised",
			out:  intakeOutcome{Status: outcomeQueued},
			want: "🤔 We couldn't read that page. We'll check by hand whether we can pull its jobs — if we can, you'll get a credit. Not credited yet.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/html", func(t *testing.T) {
			if got := renderIntakeOutcome(tt.out, origin, telegramEmphasize); got != tt.want {
				t.Errorf("renderIntakeOutcome() =\n%q\nwant\n%q", got, tt.want)
			}
		})
		wantMarkdown := tt.wantMarkdown
		if wantMarkdown == "" {
			wantMarkdown = tt.want
		}
		t.Run(tt.name+"/markdown", func(t *testing.T) {
			if got := renderIntakeOutcome(tt.out, origin, discordEmphasize); got != wantMarkdown {
				t.Errorf("renderIntakeOutcome() =\n%q\nwant\n%q", got, wantMarkdown)
			}
		})
	}
}

// TestDiscordLink_disabledReturns503 mirrors TestTelegramDisabledWhenUnconfigured:
// a partially- or un-configured bot must not mint a link token.
func TestDiscordLink_disabledReturns503(t *testing.T) {
	h := &discordHandlers{} // no config → disabled
	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/discord/link", auth.RequireAuth(iss, testVersions), h.LinkDiscord)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/me/discord/link", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", res.StatusCode)
	}
}
