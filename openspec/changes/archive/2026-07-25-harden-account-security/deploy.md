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

## 2. SERVED_HOSTS must list every served host

**This one bit in production.** An earlier draft said to leave `SERVED_HOSTS` unset,
reasoning that prod runs a single origin. That was checked against `/opt/freehire/.env`
alone — but the API also reads `/opt/freehire/env/hire-api.env`, and the cookie domain
lives there:

```
COOKIE_DOMAIN=.freehire.dev,.freehire.me
FRONTEND_ORIGIN=https://freehire.dev
```

So prod answers on **both** domains. The old suffix rule trusted any host under either;
the exact allowlist, left unset, narrows to `freehire.dev` alone. Sign-in on
`freehire.me` then sets its state cookie on `freehire.me`, while the provider is told to
call back to `freehire.dev` — the state can never match, and every OAuth attempt fails
with `state mismatch`.

Set it to every host the app serves (nginx `server_name`s minus the sibling services):

```
SERVED_HOSTS=freehire.dev,www.freehire.dev,freehire.me,www.freehire.me
```

in `/opt/freehire/env/hire-api.env`, then restart the active colour
(`systemctl restart freehire-api@blue`). Verify per host:

```bash
for h in freehire.dev freehire.me; do
  curl -sD - -o /dev/null "https://$h/api/v1/auth/oauth/google/start" | grep -i '^location'
done   # each redirect_uri must name the host it was requested on
```

The server now logs this misconfiguration at startup (`COOKIE_DOMAIN is set but
SERVED_HOSTS is not`), so it surfaces in seconds rather than as a broken sign-in.

Adding a **new** domain still needs its redirect URI registered with Google, GitHub and
LinkedIn before it is listed here.

`TELEGRAM_WEBHOOK_SECRET` is present on prod (verified: the webhook answers 403 to an
unauthenticated POST), so the feature survives its new enable condition.

## 3. Account email (DONE 2026-07-25)

Account mail sends through **us-east-1**, not the `eu-west-1` the rest of the stack uses.
`eu-west-1` is sandboxed and its production-access request was DENIED (case
`178385855400669`); `us-east-1` already has production access (50k/day).

What was wired, for the record:

- `mail.freehire.me` verified for sending in us-east-1 (Easy DKIM; three CNAMEs plus an
  SPF TXT at Namecheap, which holds DNS manually — it is not in terraform).
- `/opt/freehire/.env.notify` gained `AWS_REGION=us-east-1`, and its
  `NOTIFY_EMAIL_FROM` was corrected from `notifications@mail.freehire.dev` to
  `…@mail.freehire.me`. The IAM policy only ever allowed the `.me` address, so the
  notify worker could not have delivered a single digest either — that bug is fixed by
  the same line.
- A drop-in (`/etc/systemd/system/freehire-api@.service.d/mail.conf`) loads that file on
  the API **after** the shared `.env`, so only the API moves region. Safe because the API
  builds exactly one AWS client (SES); its S3 use is static MinIO-style credentials.

No new IAM user was needed: the notify key is scoped to `ses:SendEmail` with
`ses:FromAddress = notifications@mail.freehire.me` and carries no region condition.

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
