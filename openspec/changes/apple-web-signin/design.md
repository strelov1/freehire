## Context

`internal/auth/oauth` already implements Google, GitHub, and LinkedIn behind a
shared `Provider` interface and a `Registry` built from `config.OAuthCredentials{ClientID,
ClientSecret}` pairs (see `internal/auth/oauth/AGENTS.md`). All three existing
providers are OIDC-userinfo shaped: exchange a code for an access token, then GET a
userinfo endpoint for `sub`/`email`/`email_verified`.

Apple's Sign in with Apple breaks two assumptions this registry was built around:

1. **No static client secret.** Apple authenticates the token exchange with a JWT
   that the client (freehire) signs itself, using a Team ID, a Key ID, and an
   ES256 private key (`.p8`) issued once from Apple Developer. There is nothing to
   put in a `ClientSecret` env var.
2. **No userinfo endpoint.** Identity arrives only inside the token exchange's
   `id_token`, a JWT signed by Apple. Trusting it requires verifying that
   signature against Apple's published JWKS — the existing providers never verify
   a token signature themselves, since their userinfo call is itself the trust
   boundary (an authenticated HTTPS GET).

Apple also requires `response_mode=form_post` whenever the `email` scope is
requested (mandatory here — email is what account resolution keys on), so its
callback lands as a `POST` with a form-encoded body instead of the `GET` query
string every other provider's callback uses.

The Apple Developer side is already partly done: an App ID (`me.freehire.mobile`)
with Sign In with Apple enabled, a Team ID (`25U9HN34VM`), and a signing key
(Key ID `ZC7298D2TR`, `.p8` on disk) exist from the mobile-app setup work. This
change adds a Services ID for the web flow and reuses the same signing key — Apple
allows one key to back multiple Services/App ID configurations.

## Goals / Non-Goals

**Goals:**
- Add `apple` as a fourth provider in the existing OAuth registry, indistinguishable
  from the caller's point of view (`GET /api/v1/auth/oauth/providers` lists it,
  the same `/start` and `/callback` shape signs the user in).
- Verify Apple's `id_token` signature before trusting any claim in it.
- Keep the other three providers' code and config untouched in behavior — only
  their constructor's parameter shape changes (see Decisions).

**Non-Goals:**
- Native Sign in with Apple in the freehire-mobile app (a separate, later change —
  Apple's App Review guidance expects the native SDK on iOS, not a webview
  redirect, so it is not simply "the mobile app calls the same `/start` URL").
- Capturing the user's name (Apple provides it only on the first authorization,
  ever). No other provider stores a name either; skipped for parity.
- Automatic rotation of the Apple signing key. The client-secret JWT is minted
  fresh per token exchange from the long-lived `.p8` key, so there is nothing to
  rotate on a schedule — only the underlying key itself, which is out of scope
  here (same lifecycle as Apple's own key expiry/revocation, handled like any
  other credential rotation).

## Decisions

### 1. Extend `OAuthCredentials` rather than isolate Apple's config

`config.OAuthCredentials` gains three optional fields — `TeamID`, `KeyID`,
`PrivateKey` — used only by Apple; Google/GitHub/LinkedIn ignore them. The
`constructors` map's function signature changes from
`func(clientID, clientSecret, redirectURL string) Provider` to
`func(creds config.OAuthCredentials, redirectURL string) Provider`, so every
provider constructor now takes the whole credentials struct instead of two bare
strings.

**Alternative considered:** keep `OAuthCredentials` as-is and give Apple its own
config type (`AppleOAuthCredentials`) threaded through the registry as a second,
provider-specific argument. This isolates Apple's odd shape from the other three
providers' code (zero-diff for them) at the cost of a special-cased construction
path in `Registry`. Rejected in favor of the uniform-struct approach: one shape,
one env-loading loop, one place (`NewRegistry`'s per-provider enablement check)
that already has to know each provider's required fields either way.

**Alternative considered:** a pre-signed, long-lived client-secret JWT stored
directly in `OAUTH_APPLE_CLIENT_SECRET`, requiring zero config-shape changes.
Rejected — Apple caps such a JWT's validity at 6 months, turning this into a
manual rotation chore with silent-failure risk (sign-in breaks the day the JWT
expires, discovered only when someone notices). Minting the JWT fresh per
exchange from the long-lived private key removes the expiry problem entirely and
costs a few milliseconds of ES256 signing per sign-in attempt.

### 2. Client-secret JWT minted per exchange, not cached

`apple.go`'s `FetchIdentity` signs a 5-minute-lived ES256 JWT
(`iss`=Team ID, `sub`=Services ID, `aud`=`https://appleid.apple.com`, `kid`=Key ID
header) immediately before each token exchange, using `golang-jwt/jwt/v5` (already
a project dependency — no new dependency added). No caching, no background
rotation job: the private key is the only long-lived secret, and it does not
expire on Apple's schedule the way a signed assertion would.

### 3. `id_token` verified against Apple's JWKS, not merely decoded

Apple's JWKS (`https://appleid.apple.com/auth/keys`) is fetched and its keys used
with `golang-jwt/jwt/v5`'s `Keyfunc` to verify the `id_token`'s RS256 signature
before any claim (`sub`, `email`, `email_verified`) is trusted. This is the one
place in `internal/auth/oauth` that verifies a token signature itself, because
Apple is the one provider offering no live, authenticated endpoint to double as
the trust boundary. The JWKS response is small and rarely rotates; no caching
layer is added for this first version (Non-Goal) — each exchange fetches it fresh,
matching the simplicity of the existing providers' per-request userinfo GET.

### 4. Callback route gains POST alongside GET

`internal/handler/oauth.go`'s `OAuthCallback` currently reads `state`/`code` via
`c.Query`. A thin per-method wrapper reads from `c.Query` on GET and
`c.FormValue` on POST, then calls the same `OAuthCallback` body — every other
provider is unaffected since they never send a POST callback.

## Risks / Trade-offs

- **[Risk]** Apple's client-secret JWT construction is fiddly (specific `iss`/`sub`/`aud`/`kid`
  requirements) and a mistake fails silently as a generic token-exchange error.
  → **Mitigation**: unit tests assert the exact claim set and header before any
  network call; the private key used in tests is a throwaway, not the real `.p8`.
- **[Risk]** JWKS fetched fresh on every exchange adds one extra outbound HTTPS
  call (and one more thing that can time out) to Apple sign-in specifically.
  → **Mitigation**: bounded by the same `userinfoTimeout` pattern already used for
  the other providers' calls; a JWKS fetch failure fails the sign-in attempt the
  same way a userinfo fetch failure does today (redirect with `auth_error`).
- **[Trade-off]** The mobile app cannot reuse this flow as-is for a native "Sign in
  with Apple" button later — Apple's App Review guidance expects the native SDK,
  which produces an identity token through a different path than this
  authorization-code flow. Explicitly a Non-Goal here; called out so it is not
  assumed "free" later.

## Migration Plan

No data migration. Deploy is additive: the `apple` provider is enabled only once
all four `OAUTH_APPLE_*` env vars are set on production, so shipping the code
ahead of the Apple Developer Portal setup is safe (`apple` simply stays absent
from the provider list, matching every other optional provider's disabled state).

Rollback: unset the `OAUTH_APPLE_*` env vars (or revert the deploy) — no schema or
data changes to reverse.

## Open Questions

None outstanding — Services ID naming, scope (email-only), and the config-shape
decision were confirmed during brainstorming before this document was written.
