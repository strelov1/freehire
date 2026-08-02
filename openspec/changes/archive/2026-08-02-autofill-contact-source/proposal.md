## Why

The browser extension's autofill serves the candidate's contact block into application
forms, and it reads that block from one place: the most recently updated row in `cvs`.
On production that source is empty for almost everyone. 174 users have a current
structured résumé — name, email, phone, location and links, already parsed into typed
fields — and only 10 of them have a CV. Among the 17 API-key holders who are the
extension's actual audience, exactly **one** has a CV; seven more have a structured
résumé and get nothing but their account email.

The read is also the only raw SQL left in `internal/handler`: two `pool.QueryRow` calls
that sqlc cannot see, so a column rename in `migrations/` breaks autofill at runtime
rather than at build time. And the `ORDER BY updated_at DESC LIMIT 1` disagrees with the
domain's own definition of "the user's CV" — `cv.Store.BaseCV` deliberately excludes
tailored copies, because a tailored copy is written for one vacancy.

## What Changes

- The autofill contact block is assembled from an ordered source list instead of one
  query: the base CV's header, then the structured résumé's contact fields, then the
  account email alone. The first source that yields a name wins; the account email
  continues to backstop an absent or empty address at every tier.
- **BREAKING (behaviour, narrow):** a user whose only CVs are tailored copies would now
  be served the base CV or the structured résumé rather than the tailored copy's header.
  No production user is in this state (0 rows), so the change is observable only in
  principle.
- The two raw `pool.QueryRow` calls in `internal/handler/autofill_profile.go` are replaced
  by `cv.Store`, the existing structured-résumé read, and `GetUserByID`. `internal/handler`
  is left with no raw SQL.
- Both callers — `GET /api/v1/me/autofill-profile` (the extension's deterministic fill)
  and `POST /api/v1/me/autofill/run` (the agent-driven fill) — inherit the change through
  the one function they share.
- The endpoints gain an OpenSpec capability; today they have none.

## Capabilities

### New Capabilities

- `extension-autofill`: the canonical autofill contact block the browser extension writes
  into application forms — which fields it carries, which sources answer for them and in
  what order, and what an absent source yields.

### Modified Capabilities

None. `resume-structured-profile` gains a reader, not a requirement: this change consumes
the structure it already guarantees (typed, sanitized, stamped against the current résumé)
and adds no obligation to it. `cv-autofill` is a different capability despite the name —
it covers the onboarding wizard's pre-fill from a résumé, not form autofill.

## Impact

- `internal/handler/autofill_profile.go` — the assembly function and its two raw queries.
- `internal/handler/autofill_agent.go` — unchanged; it calls the same function.
- `internal/handler/handler.go` — `*API` gains the two dependencies the assembly needs
  (`*cv.Store` and the queries handle it already holds), or a small feature handler holds
  them. `browserTools` stays on `API`: the assistant shares it.
- No migration, no new column, no wire-shape change. `autofillProfile`'s nine JSON fields
  are unchanged, so the extension needs no release.
- Read-only against `users` and `cvs`; nothing in this change writes.
