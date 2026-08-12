# Accounts conventions

Account resolution and the credential lifecycle: password register/login, mailed six-digit
codes, password reset/change, and mapping an external sign-in identity onto a local user.

## Scope boundary

Four packages split this surface — keep the split:

- **`internal/accounts`** (here) — the *policy*: who an identity resolves to, what a valid
  password is, when a code is burnt.
- **`internal/auth`** — the *primitives*: bcrypt, JWT issue/verify, cookie transport,
  middleware. See [../auth/AGENTS.md](../auth/AGENTS.md).
- **`internal/auth/oauth`** — provider registry and the authorization-code flow.
- **`internal/handler`** — HTTP shape and rate limiting; handlers never re-implement policy.

## Always true

- **Identity-first, then verified-email, never anything else.** `ResolveOAuthAccount` tries
  `(provider, provider_user_id)` first, and only then falls back to email — and only when
  the provider reports it **verified**. An unverified or empty email is `ErrNoVerifiedEmail`
  and resolves to nothing. This is the anti-takeover gate; do not add a path around it.
- **The seizure rule.** When a provider-verified identity arrives for an address held by an
  *unverified, password-backed* account, that account is **seized**: `password_hash = NULL`,
  `email_verified = true`, `token_version + 1`, **and every row in `api_keys` deleted** (see
  `migrations`/`users.sql`). The password someone set on an address they never proved is
  destroyed rather than silently joined — otherwise registering against a stranger's address
  would pre-hijack their future OAuth sign-in.
- **`token_version` does not reach API keys — the takeover statements delete them.** A key is
  matched against `api_keys.token_hash` and `expires_at` and never joins `users`, so bumping
  the generation leaves it live. That is intentional for `logout-all` and a password change
  (a key is a durable programmatic credential, not a session), and wrong for the two events
  where the account changes hands: `SeizeUnverifiedAccount` and `ResetUserPassword` therefore
  carry a `DELETE FROM api_keys` as a data-modifying CTE, so the revocation cannot be
  separated from the takeover. A new takeover-shaped path must do the same; bumping the
  version alone would leave a never-expiring bearer credential in the previous holder's hands.
- **Every credential change bumps `token_version`.** Set/reset password and seizure all
  increment it, which strands every outstanding JWT. That coupling is the entire revocation
  story for a stateless token — a new credential write path that skips the bump silently
  leaves old sessions alive.
- **Login is constant-work.** An unknown email still spends a bcrypt check against
  `dummyPasswordHash`, so response timing doesn't disclose whether an account exists.
  `TestLogin_UserNotFound_SpendsDummyCheck` pins it.
- **Password bounds are bcrypt's, not a style choice.** `maxPasswordLen = 72` because bcrypt
  silently truncates past 72 bytes — accepting a longer one would ignore its tail.
- **Codes are keyed `(user_id, purpose)`**, at most one outstanding per purpose, so a pending
  email verification and a pending password reset never overwrite each other. TTL 15 min,
  5 wrong guesses, 60 s resend cooldown.
- `ErrInvalidCode` deliberately covers wrong / consumed / burnt without distinguishing them.
- OAuth callbacks race: `ResolveOAuthAccount` handles both `ErrIdentityConflict` (same
  identity, concurrent callback) and `ErrEmailRace` (different identity, same verified
  email) by retrying rather than failing. Both paths are unit-tested — preserve them if you
  touch the resolution order.

## Limitations

- No magic-link sign-in, no identity unlinking (the repository seam exists, the UI doesn't).
- The rate limiter on credential endpoints fails open on Redis errors (`internal/ratelimit`, Redis-backed GCRA — a Redis outage means no throttling, only log warnings).
