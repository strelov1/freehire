## Why

Sign in with Apple is required by Apple's App Store review guidelines whenever an
app offers other third-party sign-in options — the freehire mobile app just shipped
Google OAuth, so Apple becomes a submission blocker. Building it on the web first
(reusing the existing `internal/auth/oauth` provider registry) gives freehire.me an
Apple sign-in button now and puts the account-resolution and provider plumbing in
place before the mobile app adds its own native entry point later.

## What Changes

- Add an `apple` OAuth provider to the existing registry (`internal/auth/oauth`),
  alongside Google/GitHub/LinkedIn.
- Apple's credential shape differs from every existing provider: instead of a
  static client secret, it authenticates with a JWT that freehire signs itself
  (ES256, using a Team ID + Key ID + `.p8` private key from Apple Developer). This
  JWT is minted fresh on every code exchange, not stored or rotated.
- Apple returns identity only inside a signed `id_token` (no userinfo endpoint) —
  freehire verifies its signature against Apple's published JWKS before trusting
  `sub`/`email`/`email_verified`.
- Apple requires `response_mode=form_post` when the `email` scope is requested, so
  its callback arrives as a `POST` with a form-encoded body, not the `GET` query
  parameters every other provider uses. The callback route gains POST support.
- `config.OAuthCredentials` gains optional `TeamID`, `KeyID`, `PrivateKey` fields
  used only by the Apple provider; the other three providers are unaffected.
- Web SPA: no new UI code — the existing "Continue with <Provider>" button list is
  driven by `GET /api/v1/auth/oauth/providers`, so Apple appears automatically once
  configured. Only the icon/label lookup needs an Apple entry.
- Apple Developer Portal: a new Services ID (`me.freehire.web`) is registered and
  associated with the existing App ID (`me.freehire.mobile`), with `freehire.me`
  and the callback URL configured as its domain/return URL.

## Capabilities

### New Capabilities

(none — this extends the existing OAuth sign-in capability)

### Modified Capabilities

- `user-auth`: the "OAuth provider sign-in" requirement gains Apple as a fourth
  provider, a POST-callback path for providers that require `response_mode=form_post`,
  and a provider-enablement rule that accounts for Apple's non-uniform credential
  shape (Team ID/Key ID/private key instead of a client secret).

## Impact

- `internal/auth/oauth/`: new `apple.go` (provider), `constructors` signature
  change (`func(config.OAuthCredentials, redirectURL string) Provider`), JWKS
  fetch/verify helper.
- `internal/config/config.go`: `OAuthCredentials` gains `TeamID`/`KeyID`/`PrivateKey`;
  new env vars `OAUTH_APPLE_CLIENT_ID` (Services ID), `OAUTH_APPLE_TEAM_ID`,
  `OAUTH_APPLE_KEY_ID`, `OAUTH_APPLE_PRIVATE_KEY`.
- `internal/handler/oauth.go`: callback route accepts POST in addition to GET.
- `web/src/lib/...`: provider→icon/label map gains an `apple` entry.
- Apple Developer Portal: register the `me.freehire.web` Services ID manually —
  no public API covers Services IDs (unlike Bundle IDs).
- No database schema changes — Apple identities live in the existing
  `user_identities` table like every other provider.
