## Context

See `proposal.md` - Why for the motivation. Two facts shape the approach:

- The rail-inclusion of `browse` sessions was deliberate, and the reasoning is recorded
  right at the source of the leak — `internal/db/queries/assistant.sql`,
  `ListAssistantChatSessions`: *"a browsing conversation begun in the extension's side
  panel is one the candidate can pick up at their desk, where it **simply cannot see a
  page any more**."* That assumption holds only if the extension is disconnected while
  the website is used. It is not: `internal/browsertools.Hub` keys a channel by user id,
  not by session id, so a user with the side panel open in one tab and `/my/assistant`
  open in another still has a live channel, and `read_current_page` succeeds.
- `read_current_page`'s registration (`internal/handler/assistant_tools.go`, `registry`)
  is gated purely on `session.Preset`. The request that carries a turn already
  distinguishes cookie vs. Bearer auth in `internal/auth` (`ViaAPIKey` exists for
  key-vs-cookie, but nothing today separates a Bearer *session JWT* — the extension's own
  carrier — from the cookie).

## Goals / Non-Goals

**Goals:**
- Close the leak at both the layer that causes it (backend tool registration) and the
  layer that surfaces it (the website's session rail), per the confirmed direction.
- Keep the fix mechanically small: no new tables, no new capability, reuse the
  degrade-rather-than-fail pattern the codebase already uses for an unbound tailoring
  session.

**Non-Goals:**
- Preserving cross-surface continuity for browsing conversations. The proposal marks
  this **BREAKING** on purpose — restoring it safely (e.g., only when the extension is
  provably NOT the caller's) is future scope, not this change.
- The broader `assistant_*_tools.go` structural refactor (data-driven `registry()`,
  file-naming consistency) surfaced while investigating this. Real, but separate —
  left for a follow-up.

## Decisions

**1. Exclude `browse` at the SQL query, not just in the Go handler or the client.**
`ListAssistantChatSessions`'s `WHERE preset IN (...)` already excludes `tailor` this
way. Adding `browse` to the exclusion (`'chat', 'profile', 'interview', 'debrief'`)
makes the backend list endpoint itself the single source of truth — the website rail
needs no special-casing beyond what it already does for `tailor`, and any other current
or future client of `GET /assistant/sessions` inherits the same behavior for free.
Alternative considered: filter in `ListAssistantSessions` (Go) instead of SQL — rejected,
since `tailor` is already excluded at the SQL layer and splitting the two exclusions
across layers would be its own inconsistency.

**2. Also make the website treat a browse session opened by direct URL as a dead link,
matching `tailor`.** `opensInRail(preset)` (`web/src/lib/assistant/presets.ts`) currently
returns `true` for `browse`; change it to return `false`, i.e. `preset !== 'tailor' &&
preset !== 'browse'`. Without this, a bookmarked or guessed `/my/assistant/<browse-id>`
URL would still load and run — silently missing the page tool because of Decision 3, an
inconsistent-feeling degrade rather than the clear "chat unavailable" the tailoring case
already shows. Confining `browse` to the extension should mean the website never
presents it as a conversation to continue, not that it presents it with one fewer tool.

**3. Gate `read_current_page`'s registration on the request's auth carrier, in addition
to preset — defense in depth, not the primary fix. Resolve it ONCE, before either the
prompt or the tool set is built, not separately in each.** Add `auth.ViaCookie(c) bool`,
mirroring the existing `auth.ViaAPIKey(c)`: a new `localsViaCookie` local set `true` in
`RequireAuth`'s cookie path and in `RequireAuthOrScopedKey`'s cookie branch, left unset
on the Bearer branch (both the JWT and API-key cases). Neither carrier flag alone answers
"is this the extension" — an API key is never the extension's own credential either, even
though it also leaves `ViaCookie` false — so `streamSSE` combines both:
`asExtension := !auth.ViaCookie(c) && !auth.ViaAPIKey(c)`.

That combined bool feeds a new pure function, `effectivePreset(preset string, asExtension
bool) string` (`internal/handler/assistant.go`, beside `streamSSE`, its only caller):
demotes `browse` to `chat` when `!asExtension`, passes every other preset through
unchanged. `streamSSE` calls it once into a local `turnSess` (a copy of `sess` with only
`.Preset` overridden) and builds BOTH the prompt and the registry from `turnSess.Preset` —
`registry()` itself is untouched, still taking only `(sess, batchID)` and trusting
whatever preset it is handed, exactly as before this change.

This shape is not incidental. `assistant.NormalizePreset`'s own doc comment already
warns against deciding a session's effective preset twice — once for
`assistant.SystemPrompt`, once for `registry()` — because the two can disagree, and a
model told to always open by calling a tool it does not have just reports "unknown tool."
An earlier version of this change added the carrier check as a second `viaCookie bool`
parameter on `registry()` alone, leaving `SystemPrompt(sess.Preset, ...)` to read the
UNDEMOTED preset — so a cookie-authenticated `browse` turn got the chat tool set under
the browse prompt, which opens by insisting the model call `read_current_page` first. A
carrier-aware demotion is exactly the kind of second preset-deciding switch
`NormalizePreset` forbids, so it has to happen before the fork, not inside one branch of
it. A `browse` session that reaches a turn any way other than the extension's own Bearer
JWT (a stale rail entry, a future client bug, an API key) now runs, prompt and tools
alike, as an ordinary chat — the same degrade-rather-than-fail shape
`TestTailorPresetWithoutABindingHasNoCVTools` already establishes for a CV-less tailoring
session.

**4. Do not attempt to distinguish "the extension" from "any other Bearer session-JWT
client."** `asExtension` excludes the two carriers this codebase can already name —
cookie and API key — but does not prove the caller is specifically the extension: a
future first-party client authenticating with a Bearer session JWT the same way (a
mobile app, say) would also pass. That is acceptable: the leak this change closes is
specifically "a credential that is provably NOT the extension's own silently gained
page-read access" — narrowing further would need a dedicated client credential this
codebase does not have today, and the AGENTS.md invariant this restores ("meant to be
held from the browser extension") is already stated in terms of carrier, not a stronger
identity check.

**5. Close the identical leak in `POST /me/autofill/run` — found by review of this
change, not the original proposal.** `RunAgentAutofill`
(`internal/handler/autofill_agent.go`) attaches to the SAME `internal/browsertools.Hub`
channel `read_current_page` does, mounted behind the SAME `mw.key` (cookie or
full-scope API key), with no carrier check at all before this fix. The exact reasoning
that motivates the rest of this change — a user-id-keyed channel is reachable from any
of that user's authenticated requests, not only the one that opened the extension —
applies unchanged. It is worse here: autofill WRITES into the live form on whatever
page is attached, so a caller reaching it without the extension does not just read a
page, it submits data into one. Unlike `read_current_page` / the browse preset, there
is no "run as an ordinary chat" degraded mode for a form-filling endpoint — the whole
route is the browser-tool call — so the fix is a flat refusal: `RunAgentAutofill`
returns `403` unless `auth.ViaCookie(c)` and `auth.ViaAPIKey(c)` are both false. This
reuses the exact primitives Decision 3 already added; no new auth surface.

## Risks / Trade-offs

- **[Risk]** A user who relied on resuming a browsing conversation from their desktop
  loses that ability with no migration path — their old `browse` sessions still exist in
  the database but become permanently unreachable from any client (the website excludes
  them by query; the extension addresses sessions by an id in its own `storage.local`, not
  by browsing a list, so it never surfaces an old one either).
  → Mitigation: none needed functionally (no data is deleted, and nothing else read
  those rows) — call it out plainly as the proposal's BREAKING note so it isn't a
  surprise in review.
- **[Risk]** A carrier-aware demotion applied to only one of {prompt, tools} silently
  reintroduces the exact split-brain `NormalizePreset` exists to prevent.
  → Mitigation: `effectivePreset` is the only place that decides it, called once, before
  either the prompt or the registry is built; `registry()` itself stays carrier-blind, so
  there is no second switch left to disagree. `TestBrowsePresetOverCookieRunsAsAnOrdinaryChat`
  pins both halves together in one assertion.
- **[Trade-off]** `ViaCookie` is a second boolean beside `ViaAPIKey` rather than one
  three-state carrier enum (`cookie` / `session-bearer` / `api-key`). Chosen because
  today only one call site needs to ask "was this cookie or not," and `ViaAPIKey` already
  answers the API-key question independently; introducing an enum type would touch more
  of `internal/auth`'s public surface than this change needs. If a third caller needs the
  distinction, consolidate then.

- **[Risk, accepted]** The fix is applied at each caller's own site (`streamSSE` for
  `read_current_page`, `RunAgentAutofill` for autofill — Decision 5), not inside
  `internal/browsertools.Hub` itself. Both of today's harnesses now carry the check,
  but nothing in `Hub` forces a THIRD one to. Building a shared enforcement point now
  would be infrastructure for a caller that does not exist yet; the seam is `Hub`, and
  Decision 5 turning out to be needed at all is itself the signal that the seam is
  worth revisiting once a third caller arrives.
- **[Risk, accepted]** `RunAgentAutofill`'s refusal (Decision 5) is covered only for
  the cookie and API-key cases; the success path — a genuine extension Bearer JWT
  reaching past the guard into the DB-backed profile assembly — is not exercised by a
  new test, because doing so would need mocking `h.cvs`/`h.resumes`/`h.accounts` this
  file's existing tests do not set up, for a path this change does not otherwise touch.
  The guard's own boolean logic is a plain `||` of two independently-tested primitives
  (`auth.ViaCookie`, `auth.ViaAPIKey`), so the residual risk is narrow.

## Migration Plan

1. `internal/db/queries/assistant.sql` + `make sqlc` (regenerates `internal/db/*.go`).
2. `internal/auth/middleware.go`: add `localsViaCookie` / `ViaCookie`.
3. `internal/handler/assistant.go`: add `effectivePreset` and wire `asExtension` /
   `turnSess` into `streamSSE`, so both the prompt and `registry()` resolve from the
   same demoted preset.
4. `web/src/lib/assistant/presets.ts`: `opensInRail` excludes `browse`.
5. Add the new scenarios from the spec deltas: `effectivePreset`'s own cases, and one
   pinning that the prompt and the tool set agree after a demotion.
6. `internal/handler/autofill_agent.go`: refuse `RunAgentAutofill` for cookie/API-key
   auth (Decision 5).

No data migration, no feature flag: this is a behavior change deployed atomically with
the next release, matching how `tailor`'s exclusion from the rail was never flagged
either. Rollback is a plain revert — no stored data changes shape.

## Open Questions

(none — see Non-Goals for what was deliberately deferred rather than left open)
