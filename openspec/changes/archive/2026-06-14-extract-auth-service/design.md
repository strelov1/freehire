## Context

`internal/handler/oauth.go` resolves an OAuth identity into a local user id across two methods:

- `resolveOAuthUser(ctx, provider, identity)` — the **policy**: look up `(provider, providerUserID)`; if found, return the user; on `pgx.ErrNoRows`, require a **verified, non-empty** email (else `"no verified email"`); call `linkOrCreateUser`; on a unique-violation race, retry the identity lookup once.
- `linkOrCreateUser(ctx, identity, email)` — the **mechanism**: `h.pool.Begin`, `q := h.queries.WithTx(tx)`, `GetUserByEmail` → reuse or `CreateUser` (passwordless, `PasswordHash: pgtype.Text{}`) → `CreateUserIdentity` → `tx.Commit`.

The security-critical invariant ("never link or create by an unverified email" — an account-takeover vector) and the concurrent-callback race handling are pure policy, but they sit on top of `pgx`/`pgtype`/`*db.Queries` and a live transaction, so they can only be tested through a real callback + Postgres. This slice extracts the policy into a unit-testable service, following the pattern set by `internal/jobtracking`.

## Goals / Non-Goals

**Goals:**

- `internal/accounts.Service.ResolveOAuthAccount` owns the policy, testable against a fake `Repository` (no DB, no pgx, no Fiber).
- The transaction is an adapter detail behind a coarse `LinkOrCreateByEmail`.
- The verified-email gate and the race-retry are explicit, individually tested branches.
- Byte-identical OAuth behavior: same resolution order, same security rule, same race outcome, same redirects/statuses.

**Non-Goals:**

- Extracting password `Register`/`Login`/`Me` (thin, low-risk; a later slice).
- Touching the `oauth` provider registry, CSRF/return cookies, `setSession`, or any redirect/`auth_error` behavior.
- SQL/schema changes; `Register`'s parameter list.

## Decisions

### 1. Service takes primitives, not `oauth.Identity`

`ResolveOAuthAccount(ctx, provider, providerUserID, email string, emailVerified bool) (int64, error)`. The handler unpacks `oauth.Identity` into these. *Why:* keeps `internal/accounts` decoupled from the `oauth` package (it depends on neither Fiber nor oauth); the email-normalization (`lower(trim(...))`) stays in the service since it is part of the matching rule.

### 2. Coarse, transaction-owning repository

```go
type Repository interface {
    UserIDByIdentity(ctx, provider, providerUserID string) (int64, error) // ErrIdentityNotFound when absent
    LinkOrCreateByEmail(ctx, provider, providerUserID, email string) (int64, error) // runs the tx; ErrIdentityConflict on a racing unique-violation
}
```

The adapter (`*db.Queries` + `*pgxpool.Pool`) implements `LinkOrCreateByEmail` with the exact current transaction (begin → GetUserByEmail → reuse/CreateUser passwordless → CreateUserIdentity → commit). *Why a coarse op rather than exposing the tx to the service:* the atomic unit is a single use-case step; pushing a generic transactor/UnitOfWork into the service would add machinery with no second caller (YAGNI). The service stays a pure policy orchestrator. *Alternative rejected:* a `WithTx(func(repo) error)` transactor — more flexible but premature; revisit only if a second multi-step use case needs it.

### 3. Race handling via an explicit sentinel

The current code detects the concurrent-callback race with `isUniqueViolation(err)` on the identity insert and retries `GetUserByIdentity` once. In the new shape, the adapter maps that Postgres unique-violation to `ErrIdentityConflict`; the service, on `ErrIdentityConflict` from `LinkOrCreateByEmail`, retries `UserIDByIdentity` once and returns the winner (or the original error if the retry also misses). *Why:* moves the race policy into the tested service while keeping the pgx-specific detection in the adapter. `isUniqueViolation` moves to (or is shared with) the adapter.

### 4. Verified-email gate stays first-class

`ResolveOAuthAccount`: (1) `UserIDByIdentity` — found → return; (2) on `ErrIdentityNotFound`, if `!emailVerified || email == ""` → `ErrNoVerifiedEmail`; (3) normalize email, `LinkOrCreateByEmail`; (4) on `ErrIdentityConflict` → retry step 1 once. The handler maps `ErrNoVerifiedEmail` (and any error) through the existing `oauthFail` → `auth_error` redirect, exactly as today (the user never sees a distinct message; the server logs the cause).

### 5. Handler stays the transport boundary

`OAuthCallback` keeps: provider lookup, state/return-cookie handling, `FetchIdentity`, calling `h.accounts.ResolveOAuthAccount(...)`, `setSession`, and the success/`oauthFail` redirects. The `Handler` gains an `accounts *accounts.Service` field, constructed in `Register` from the shared `*db.Queries` + `pool`.

## Risks / Trade-offs

- **Security-rule regression (unverified email linking)** → Mitigation: a dedicated unit test asserts `emailVerified=false` and `email=""` both yield `ErrNoVerifiedEmail` with NO repository write call (assert the fake's `LinkOrCreateByEmail` is never invoked); plus the existing OAuth integration tests stay green.
- **Race-retry behavior drift** → Mitigation: a unit test where `LinkOrCreateByEmail` returns `ErrIdentityConflict` and the second `UserIDByIdentity` returns the winning id — asserts the service returns it; and a test where the retry still misses returns the error.
- **Transaction semantics drift in the adapter** → Mitigation: the adapter reproduces the current `linkOrCreateUser` verbatim (begin/rollback-on-defer/commit, passwordless `pgtype.Text{}`); the OAuth integration test (`internal/handler/oauth_integration_test.go` if present, else the callback path) is the end-to-end guard. Run `go test -tags=integration ./internal/handler/` (Docker) before merge.
- **Scope creep into register/login** → Mitigation: explicit Non-Goal; those handlers keep using `h.queries` this slice.

## Migration Plan

In-place refactor in an isolated worktree off `origin/main` (the shared main checkout is volatile). Sequence: (1) `internal/accounts` package — `Service` + `Repository` + sentinels + fake-repo unit tests (TDD); (2) the `*db.Queries`+pool adapter with the verbatim transaction; (3) rewire `OAuthCallback`, delete `resolveOAuthUser`/`linkOrCreateUser`; (4) verify unit + `-tags=integration` suites green. Rollback = revert the branch. No deploy/schema/data migration.

## Open Questions

- Should `isUniqueViolation` (currently shared in `handler`) move wholly into `internal/accounts`, or stay in `handler` for `Register` too? Default: duplicate the tiny SQLSTATE check in the adapter (it is two lines) to keep `internal/accounts` self-contained; remove the handler copy only once `Register` is also extracted. Non-blocking.
