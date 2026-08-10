## 1. Apple Developer Portal setup

- [x] 1.1 Register the `me.freehire.web` Services ID manually in the Apple Developer Portal (**not** available via the App Store Connect API — `/v1/servicesIds` returns `404`, unlike Bundle IDs), associated with the `me.freehire.mobile` App ID
- [x] 1.2 Configure the Services ID's domain (`freehire.me`) and return URL (`https://freehire.me/api/v1/auth/oauth/apple/callback`) in the portal
- [x] 1.3 Confirm in the portal that the existing signing key (Key ID `ZC7298D2TR`) is usable for the new Services ID

## 2. Config plumbing

- [x] 2.1 Add `TeamID`, `KeyID`, `PrivateKey` fields to `config.OAuthCredentials`
- [x] 2.2 Load `OAUTH_APPLE_CLIENT_ID` / `OAUTH_APPLE_TEAM_ID` / `OAUTH_APPLE_KEY_ID` / `OAUTH_APPLE_PRIVATE_KEY` in `loadOAuth`, add `apple` to `oauthProviders`

## 3. Apple provider implementation

- [x] 3.1 `internal/auth/oauth/apple.go`: client-secret JWT minting (ES256, 5-minute lifetime, `iss`/`sub`/`aud`/`kid` per design)
- [x] 3.2 `AuthCodeURL`: build Apple's authorize URL with `response_type=code`, `response_mode=form_post`, `scope=email`
- [x] 3.3 JWKS fetch + `id_token` signature verification (`golang-jwt/jwt/v5` `Keyfunc` against `https://appleid.apple.com/auth/keys`)
- [x] 3.4 `FetchIdentity`: token exchange, verify `id_token`, map `sub`/`email`/`email_verified` to `Identity`
- [x] 3.5 Update the `constructors` map signature to `func(config.OAuthCredentials, redirectURL string) Provider`; update Google/GitHub/LinkedIn constructors to the new signature (behavior unchanged); add `apple` entry
- [x] 3.6 `NewRegistry`: Apple enabled only when client id, Team ID, Key ID, and private key are all present

## 4. Callback route POST support

- [x] 4.1 `internal/handler/oauth.go`: accept `POST /api/v1/auth/oauth/:provider/callback` alongside the existing `GET`, reading `state`/`code` from `c.FormValue` on POST and `c.Query` on GET, sharing the rest of `OAuthCallback`

## 5. Web UI

- [x] 5.1 Add an `apple` entry to the provider→icon/label map the SPA auth dialog uses to render "Continue with <Provider>" buttons

## 6. Verification

- [x] 6.1 `go vet -tags=integration ./...` passes
- [x] 6.2 Manual end-to-end sign-in against the real Apple Services ID on a deployed environment — verified via browser automation on freehire.me: the "Continue with Apple" button, `/oauth/apple/start` redirect, and Apple's own consent screen ("Use your Apple Account to sign in to freehire web") all work correctly against the live `me.freehire.web` Services ID. Completing an actual login needs a real Apple ID (a human's own credentials/2FA), so that final click was left to the account owner rather than automated here.
