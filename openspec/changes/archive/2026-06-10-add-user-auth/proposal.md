## Why

The `hire` backend and its Svelte SPA are fully anonymous — there is no notion
of a user, so nothing can be personalized, owned, or protected. Before features
like saved jobs, applications, or admin-only mutations can exist, the project
needs a user identity and a way to authenticate requests. This change lays that
foundation.

## What Changes

- Add a `users` table (email + bcrypt password hash) as the canonical user
  identity, with `email` as the unique login key.
- Add three HTTP endpoints under `/api/v1/auth`:
  - `POST /register` — create an account, return a signed JWT.
  - `POST /login` — verify credentials, return a signed JWT.
  - `GET /me` — return the authenticated user (requires a valid JWT).
- Add stateless JWT authentication: a signing secret (HS256), token issuance on
  register/login, and a Fiber middleware that validates `Authorization: Bearer`
  and injects the user identity into the request context.
- Existing read endpoints (`jobs`, `companies`) stay **public and unchanged** —
  this change only adds the auth surface and a reusable "require auth" guard for
  future protected routes.
- Add config (`JWT_SECRET`, token TTL) and one Go dependency for JWT.
- Integrate auth into the Svelte SPA at the **layout level**: a client auth
  store (token persisted in localStorage, bootstrapped via `GET /me`), login and
  register forms reachable from the top bar, and the top bar showing the signed
  in user with a logout action (or Login/Register actions when signed out).

## Capabilities

### New Capabilities

- `user-auth`: User accounts and stateless authentication — registration, login,
  password hashing, JWT issuance/validation, the authenticated-identity
  middleware, and the `GET /me` endpoint.

### Modified Capabilities
<!-- None. jobs/companies read endpoints are untouched; no existing spec's
     requirements change. -->

## Impact

- **Database**: new `migrations/0005_users.sql`; new `internal/db/queries/users.sql`
  consumed via regenerated sqlc code.
- **Code**: new `internal/auth/` package (password hashing, JWT issue/verify,
  Fiber middleware); new `internal/handler/auth.go` handlers wired in
  `handler.Register`. `Register`'s signature grows to accept auth config.
- **Config**: new `JWT_SECRET` and `JWT_TTL` env vars in `internal/config`.
- **Dependencies**: `golang.org/x/crypto/bcrypt` and a JWT library
  (`github.com/golang-jwt/jwt/v5`) added to `go.mod`.
- **Frontend (`web/`)**: new auth store (`src/lib/auth.svelte.ts`, mirroring the
  `theme.svelte.ts` pattern), auth functions + bearer-token attachment in
  `src/lib/api.ts`, login/register form components, and auth controls wired into
  `TopBar.svelte` / the layout. No existing endpoint or response shape changes;
  the public jobs/companies views are untouched.
