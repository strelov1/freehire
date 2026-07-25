# Deploy runbook — harden-account-security

Prod is host-2 bare metal: `root@89.167.94.146`, release via
`/opt/freehire/bin/release.sh freehire`. Order matters — the new binary SELECTs the new
columns on **every authenticated request**, so deploying before the migration takes the
whole authenticated surface to 500.

## 1. Apply the migration by hand (BEFORE the deploy)

Migrations only run on a fresh volume/DB, so on prod this does not auto-apply.

```bash
ssh root@89.167.94.146 "runuser -u postgres -- psql -d hire -v ON_ERROR_STOP=1" \
  < migrations/0041_account_security.sql
```

**Then fix ownership.** psql runs as the `postgres` superuser, so the new table is owned
by `postgres` while the app connects as role `hire` — without this every read/write on it
500s even though the table exists:

```bash
ssh root@89.167.94.146 "runuser -u postgres -- psql -d hire \
  -c 'ALTER TABLE public.user_email_codes OWNER TO hire;'"
```

(The `ALTER TABLE`s on `users` and `api_keys` need nothing — those tables already belong
to `hire`.)

Verify:

```sql
SELECT count(*) FILTER (WHERE NOT email_verified) AS unverified,
       min(token_version) AS min_version
FROM users;                             -- expect unverified = 0, min_version = 1
SELECT DISTINCT scope FROM api_keys;    -- expect only 'full'
\dt user_email_codes                    -- Owner must be hire
```

## 2. Do NOT set SERVED_HOSTS on this deploy

An earlier draft of this runbook said to set `SERVED_HOSTS=freehire.me,apply.freehire.me`.
That was written from the code's own docs, not from prod, and it is wrong here.

Prod has `FRONTEND_ORIGIN=https://freehire.dev` and **no** `COOKIE_DOMAIN`, so today
`requestOrigin` returns the canonical frontend origin for *every* request. Leaving
`SERVED_HOSTS` unset reproduces that behaviour exactly (it defaults to the frontend
origin's own host). Setting it to include `freehire.me` would make an OAuth flow started
on `freehire.me` use `https://freehire.me` as its `redirect_uri` for the first time — which
fails unless that URI is registered with Google, GitHub and LinkedIn.

Set it only as part of a deliberate multi-domain OAuth change, after registering the
redirect URIs with each provider.

`TELEGRAM_WEBHOOK_SECRET` is present on prod (verified: the webhook answers 403 to an
unauthenticated POST), so the feature survives its new enable condition.

## 3. Account email needs the us-east-1 path

The verification and password-reset flows are **inert** until the API can send mail. As of
2026-07-25 it cannot, for two independent reasons:

- **SES in `eu-west-1` is sandboxed and its production-access request was DENIED**
  (case `178385855400669`). Only `strelov1@gmail.com` can receive there.
- The API reads `/opt/freehire/.env`, whose `AWS_*` key belongs to apply's mail worker
  (S3+SQS only) and cannot call `ses:SendEmail`.

`us-east-1` **does** have production access (50k/day). The fix, once
`mail.freehire.me` is DKIM-verified there:

```bash
# One region line, in the file that already holds the SES-capable key and sender.
ssh root@89.167.94.146 "grep -q '^AWS_REGION=' /opt/freehire/.env.notify \
  || echo 'AWS_REGION=us-east-1' >> /opt/freehire/.env.notify"
```

Then add that file as a **second** `EnvironmentFile` on `freehire-api@.service` (systemd
applies them in order, so it overrides `AWS_REGION` for the API alone and leaves the other
twelve units on `eu-west-1` for their SQS/S3 work), and `daemon-reload`.

The notify IAM key is already scoped to `ses:SendEmail` with
`ses:FromAddress = notifications@mail.freehire.me` and carries no region condition, so no
new IAM user or policy is needed.

Until this is done, `/auth/verify/request` and `/auth/password/forgot` answer 503 and the
SPA's confirm-email banner offers a button that reports delivery is unavailable.

## 4. Deploy

```bash
ssh root@89.167.94.146 /opt/freehire/bin/release.sh freehire
```

**Every user is signed out once.** Tokens minted before this release carry no `tv` claim
and are rejected — the intended break; the SPA's existing 401 handling routes to sign-in.

## 5. Smoke-check

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://freehire.dev/api/v1/telegram/webhook   # 403
curl -s -o /dev/null -w '%{http_code}\n' https://freehire.dev/api/v1/auth/me                    # 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://freehire.dev/api/v1/auth/password/forgot \
  -H 'Content-Type: application/json' -d '{"email":"nobody@example.invalid"}'                   # 202
```

Then register a throwaway account and confirm the code actually arrives (this is the only
check that proves the SES wiring, not just the code path); confirm `/my/security` changes a
password without signing the caller out.

## Rollback

The previous image ignores the new columns entirely, so rolling back is safe with no
down-migration. Users would simply be signed out a second time (the old binary mints
claimless tokens, which the new one rejects again on a roll-forward).
