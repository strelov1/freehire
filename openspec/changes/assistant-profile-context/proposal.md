## Why

The in-app assistant at `/my/assistant` interrogates the user for role, stack, work
mode, salary and seniority before it will search — data the user already curated at
`/my/profile`. The cause is not a lazy prompt: a normal assistant session runs the
`freehire` CLI with no credential at all (only a CV-tailoring session is handed a
minted key), so every user-scoped read is anonymous, and `GET /me/profile` is
cookie-only besides. The assistant asks because, to the API, it is nobody.

## What Changes

- `GET /api/v1/me/profile` accepts an API key or a bearer session token in addition
  to the session cookie (`RequireAuthOrScopedKey` admitting `ScopeCV`, i.e. the
  handler's `mw.cvKey`). The narrow scope is admitted so the CV-tailoring agent can
  read the profile too; `mw.key` would answer its `cv`-scoped credential `403`.
  `PUT` and `DELETE` stay cookie-only — a leaked key must not rewrite the profile.
- The `GET /api/v1/me/profile` response gains a `cv` block: the caller's structured
  résumé projected through a field whitelist that omits `full_name`, `email`,
  `phone` and `links`. It is `null` when the caller has no current structured
  résumé. The other fields are unchanged, so existing consumers keep working.
- Contact details are unaffected and stay where they already live, on
  `GET /api/v1/me/resume`, which the profile page's contact card reads. The web app
  needs no change.

Not in this change, and deliberately so: no new API-key scope, no bootstrap
endpoint, no key minted for the assistant, no separate "agent" endpoint, and no use
of `internal/pii`. The résumé's contacts are already severed fail-closed upstream at
extraction time, so a whitelist projection is sufficient.

## Capabilities

### New Capabilities

None — this extends an existing capability.

### Modified Capabilities

- `search-profiles`: the profile read requirement changes on two counts — the
  credential it accepts (cookie, API key, or bearer session token, where previously
  only a cookie authenticated it) and its response shape (a new `cv` block carrying
  the contact-free structured résumé). The session-scoping requirement changes to
  distinguish the read, which admits a key, from the writes, which do not.

## Impact

- `internal/handler/me_profile.go` — the route's middleware and the response type.
- `internal/handler/handler.go` — the profile handlers need the résumé store wired
  in to read the structured CV.
- `internal/resumeextract` — gains the contact-free projection of `Structured`.
- `web/src/lib/api.ts` and the generated contract types — the profile response type
  grows a field. No behavioural change in the SPA; the profile page keeps reading
  contacts from `/me/resume`.

Out of this repository, and required before the assistant actually benefits:

- `~/Projects/freehire-cli` — a `freehire profile [--json]` command, and the
  `using-freehire` skill telling an agent to read the profile before interrogating
  the user.
- `~/Projects/freehire-agent` — `roy-management` passes the caller's `hire_token`
  into an assistant session's environment as `FREEHIRE_TOKEN`, alongside the branch
  that already does this for tailoring sessions. Its bundled copy of the
  `using-freehire` skill has drifted from the CLI's and needs the same edit.
