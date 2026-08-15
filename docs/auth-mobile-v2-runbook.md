# Mobile authentication v2 runbook

BE-2 is schema-first and disabled by default. Never place an Apple private key,
grant-encryption key, authorization code, identity token, refresh token, OAuth
state, PKCE verifier, or session cookie in Git, CI output, logs, or PR text.

## Deployment order

1. Deploy `0087_mobile_auth_v2.sql` while the old binary is still running.
2. Deploy the new binary with `AUTH_V2_ENABLED=false`. V1 web OAuth, including
   the existing Apple Services-ID flow, remains unchanged. The password recent-
   auth adapter is registered for the paired web bundle, but mobile discovery,
   exchange, identity unlink, and enforcement remain disabled.
3. Provision callback, native Apple, and encryption configuration below.
4. Start `auth-cleanup` every 5 minutes and `apple-revoke` every minute as
   run-once workers. Alert on non-zero exit and failed revocation jobs.
5. Set `AUTH_V2_ENABLED=true` in a non-production environment and exercise PKCE
   across at least two backend replicas. Validate native Apple on a real device.
6. Enable production v2 only after the verified-link callback is live. Apple is
   advertised only when its full client and grant-encryption configuration boots.
7. Do not ship native Apple in a store build until BE-3 queues Apple revocation
   before account deletion. The grant foreign key deliberately blocks deletion
   rather than silently orphaning a live Apple grant.

## Configuration

| Variable | Meaning |
|---|---|
| `AUTH_V2_ENABLED` | Registers `/api/v2/auth/*`; default `false`. |
| `MOBILE_AUTH_CALLBACKS` | Comma-separated `platform=https://verified/path` allowlist. Include `web=https://<host>/my/reauth`; add `ios=https://<host>/auth/mobile-callback` and `android=` likewise once the verified-link target is live. Browser providers stay hidden from `/api/v2/auth/providers` until one of them is set, so the mobile app shows only Apple. |
| `RECENT_AUTH_TTL` | Recent-auth proof lifetime; default `10m`, allowed `1m` to `1h`. |
| `APPLE_NATIVE_CLIENT_ID` | Native iOS bundle/client ID, separate from the web Services ID. |
| `OAUTH_APPLE_TEAM_ID` | Existing Apple developer Team ID. |
| `OAUTH_APPLE_KEY_ID` | Existing Sign in with Apple key ID. |
| `OAUTH_APPLE_PRIVATE_KEY` | Existing base64-encoded `.p8` PEM, server secret only. |
| `APPLE_GRANT_ACTIVE_KEY_ID` | Key ID used for new Apple refresh-grant encryption. |
| `APPLE_GRANT_KEYS` | `id:base64-32-byte-key,...`; retain old decrypt-only entries during rotation. |

`OAUTH_APPLE_CLIENT_ID` remains the web Services ID and is not reused as the
native audience. Partial native Apple configuration fails startup without
printing secret values. Production callback entries must use HTTPS.

Each enabled browser provider must also allow its server-owned v2 redirect URI:
`https://<served-host>/api/v2/auth/oauth/<provider>/callback`. For Apple web
reauthentication, add `/api/v2/auth/oauth/apple/callback` to the Services ID's
return URLs alongside the existing v1 callback. These provider-console redirect
URIs are separate from `MOBILE_AUTH_CALLBACKS`, which contains only FreeHire's
final completion targets.

## Mobile verified links

The app's completion target is `/auth/mobile-callback`, served by `web/` and
claimed by the app through `/.well-known/apple-app-site-association` (iOS) and
`/.well-known/assetlinks.json` (Android). Both files must list the path before
`ios=`/`android=` goes into `MOBILE_AUTH_CALLBACKS`, or the browser will land on
the web page instead of handing the code to the app.

The client sends `callback_target=ios|android` — the platform name, never a URL.
The server appends `?code=` to the URL it holds for that key, so the mobile app
cannot influence where the code is delivered.

Apple caches the association file, so a device may keep an old copy for hours.
Reinstalling the app forces a refresh during testing.

## Key rotation

Add the new 32-byte key to `APPLE_GRANT_KEYS`, deploy, then change
`APPLE_GRANT_ACTIVE_KEY_ID`. Keep every old key while any grant or revocation job
names it. Remove an old key only after database counts for that key ID are zero.
Rotating the Apple `.p8` is independent: provision the new key ID and private key,
restart server and worker, validate exchange/revoke, then retire the old key in
Apple Developer.

## Revocation operations

`apple-revoke` claims jobs with `FOR UPDATE SKIP LOCKED`, retries transient
provider failures with bounded backoff, and completes identity removal only after
revocation. A delayed `exchange_compensation` job takes encrypted custody before
grant finalization; successful finalization disarms it, while a crash or failed
write leaves it revocable. A crash after finalization revokes and removes only
the grant carrying that compensation job's row ID, never a newer retry grant.
Completion purges ciphertext, nonce, and encryption key ID while retaining the
idempotency tombstone. Inspect only `status`,
`attempts`, `next_attempt_at`, and `last_error_class`; never dump ciphertext or
try to decrypt a token by hand.
Permanent/ten-attempt failures require fixing Apple configuration and moving the
job back to `retry` with a forward operator migration or audited admin procedure.

## Rollback

First set `AUTH_V2_ENABLED=false`; this stops new attempts while v1 web OAuth
continues. Keep the binary/worker capable of reading grants and pending jobs.
Never drop BE-2 tables after production has accepted an Apple grant. Roll forward
with a corrective migration. The v1 mobile exchange can be retired only after the
minimum supported app version no longer uses it; v1 web callbacks remain separate.

## Production validation owned outside this PR

- verified HTTPS callback/associated-domain files and edge no-cache behavior;
- Apple Developer native App ID capability and the server key's permissions;
- real-device first login, repeat login with no email, reauthentication, unlink,
  revocation, and subsequent sign-in refusal while revocation is pending;
- two-replica PKCE consume/replay tests and log scans for forbidden fields.
