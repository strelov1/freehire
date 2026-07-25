# Deploy runbook — harden-account-security

Order matters: the new binary SELECTs the new columns on **every authenticated request**,
so deploying before the migration takes the whole authenticated surface to 500.

## 1. Apply the migration by hand (BEFORE the deploy)

initdb runs `migrations/` only on first volume init, so on the persistent prod volume this
does not auto-apply — same as 0005–0010, 0039, 0040.

```bash
psql "$DATABASE_URL" -f migrations/0041_account_security.sql
```

It adds `users.email_verified` (backfilled `true` for every existing row) and
`users.token_version` (default 1), `api_keys.scope` (default `'full'`), and the
`user_email_codes` table. All additive; the running old binary ignores them.

Verify:

```sql
SELECT count(*) FILTER (WHERE NOT email_verified) AS unverified,
       min(token_version) AS min_version
FROM users;                      -- expect unverified = 0, min_version = 1
SELECT DISTINCT scope FROM api_keys;   -- expect only 'full'
```

## 2. Set the new environment variables

```bash
SERVED_HOSTS=freehire.me,apply.freehire.me
```

Exact hostnames only — this is the OAuth redirect origin allowlist that replaced the
cookie-domain suffix match. If unset, every request falls back to `FRONTEND_ORIGIN`, so
OAuth started on `apply.freehire.me` would come back on `freehire.me`. Degradation, not
breakage, but set it.

Confirm `TELEGRAM_WEBHOOK_SECRET` is still present. It is now part of the Telegram enable
condition: with a bot token but no secret the feature stays **off** (the linking endpoints
report it unavailable, the webhook 404s) and the server logs why. Verified present on prod
on 2026-07-25 — the webhook answers 403 to an unauthenticated POST.

Account mail (verification, password reset) reuses the notify worker's SES config:
`AWS_REGION` + `NOTIFY_EMAIL_FROM`. Without them registration and sign-in work unchanged;
only the code-backed endpoints answer 503.

## 3. Deploy

**Every user is signed out once.** Tokens minted before this release carry no `tv` claim
and are rejected — that is the intended break, and the SPA's existing 401 handling routes
to sign-in. Announce it in the changelog entry.

## 4. Smoke-check

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://freehire.me/api/v1/telegram/webhook   # 403
curl -s -o /dev/null -w '%{http_code}\n' https://freehire.me/api/v1/auth/me                    # 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://freehire.me/api/v1/auth/password/forgot \
  -H 'Content-Type: application/json' -d '{"email":"nobody@example.invalid}'                   # 202
```

Then sign in, confirm the verification banner appears for a fresh registration, and that
`/my/security` changes a password without signing the caller out.

## Rollback

The previous image ignores the new columns entirely, so rolling back is safe with no
down-migration. Users would simply be signed out a second time (the old binary mints
claimless tokens, which the new one would again reject on a roll-forward).
