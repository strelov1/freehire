## Why

A `browse`-preset session (created by the extension's side panel) is currently listed
in the ordinary website's `/my/assistant` rail and can be continued there — that
inclusion was a deliberate choice (`assistant-sessions`'s "session list spans every
conversation" requirement), meant to let a candidate pick up a browsing conversation
at their desk. But the backend still grants that continued session the
`read_current_page` tool purely because `session.preset == "browse"`
(`internal/handler/assistant_tools.go`), with no check on how the turn's request
authenticated. The browser-tool relay (`internal/browsertools.Hub`) is keyed by user
id, not by session id, so if the user's browser extension is connected, the "ordinary"
website assistant will silently read whatever page is open in that extension while the
user believes they are talking to a chat with no page access. This contradicts the
documented invariant in `internal/assistant/AGENTS.md` ("A browse session is one held
from the browser extension") and is a real capability leak, not a hypothetical one.

## What Changes

- Drop `browse` from the session list the website's `/my/assistant` surface can see:
  `GET /assistant/sessions` no longer returns `browse` sessions (alongside the
  existing `tailor` exclusion), so a browsing conversation is no longer listed,
  selectable, or continuable from the ordinary site. **BREAKING**: a candidate can no
  longer resume a side-panel conversation from the desktop website; each surface's
  history stays on that surface.
- Add a defense-in-depth check at the tool-registry layer: `read_current_page` is
  registered only when the turn's request itself authenticated via a Bearer session
  JWT (the extension's carrier), never via the website's cookie — even for a
  `browse`-preset session reached some other way (a stale rail entry, a direct URL, a
  future client bug). A `browse` session continued over cookie auth degrades to a
  plain chat with no page tool, the same graceful-degradation pattern already used for
  an unbound tailoring session.
- `internal/auth` gains a way for a handler to tell whether the current request
  authenticated via the cookie or via a Bearer session JWT (today it only exposes
  API-key-vs-not).
- Close the same class of leak in `POST /me/autofill/run` (`RunAgentAutofill`), found
  during review of this change: it attaches to the identical
  `internal/browsertools.Hub` channel unconditionally for any `mw.key`-authenticated
  caller (cookie or API key), and unlike `read_current_page` it WRITES into the
  attached browser's form. It now refuses outright unless the request authenticated as
  the extension's own Bearer session JWT — there is no degraded mode for a write.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `assistant-sessions`: the session list SHALL no longer include `browse` sessions —
  only `chat`, `profile`, `interview` and `debrief` remain listed; `browse` joins
  `tailor` as excluded.
- `assistant-page-awareness`: `read_current_page` SHALL be registered only when the
  request authenticated via a Bearer session JWT, in addition to the existing
  preset check — a `browse` session reached over cookie auth SHALL NOT be offered the
  tool.
- `extension-autofill`: the agent-driven fill (`POST /me/autofill/run`) SHALL refuse a
  request that authenticated by cookie or API key, admitting only the extension's own
  Bearer session JWT — the read endpoint (`GET /me/autofill-profile`) is unchanged.

## Impact

- Backend: `internal/handler/assistant.go` (`ListAssistantSessions`, `streamSSE`,
  `effectivePreset`), `internal/handler/assistant_tools.go` (`registry`, unchanged in
  signature — see design.md Decision 3), `internal/handler/autofill_agent.go`
  (`RunAgentAutofill`), `internal/auth/middleware.go` (new carrier signal).
- Frontend (web): `web/src/lib/assistant/presets.ts` (`opensInRail` becomes
  unnecessary as a rail filter once the backend stops returning `browse`, but the
  "dead link" UX it exists for — arriving with no id when the newest session was
  filtered out — must still resolve to a fresh chat rather than an error).
- Frontend (extension): none — it never calls `GET /assistant/sessions` and always
  authenticates via Bearer, so its own `browse` conversation is unaffected.
- Out of scope: the broader stylistic inconsistencies across
  `internal/handler/assistant_*_tools.go` (naming, single-tool registration idiom)
  found while investigating this — real, but a separate readability concern from this
  security-relevant fix, left for a follow-up change.
