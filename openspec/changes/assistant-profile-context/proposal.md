## Why

The in-app assistant opens a job-search conversation by interrogating the user for
their role, stack, seniority, work mode and salary — every one of which they already
curated at `/my/profile`. The agent has tools for the market (`facets`, `search_jobs`,
`get_job`, `get_company`, `market_fit`) and for the user's tracked jobs, but none for
the user's own stated preferences. It asks because it has no way to look.

## What Changes

- **A `get_profile` tool**, registered for every session. It returns the caller's
  specializations, skills, excluded skills and location preferences, plus their
  structured CV projected onto a contact-free view. A caller with no profile gets a
  result that directs the agent to the profile page, not an error and not an empty
  profile it might read as "no preferences".
- **The chat prompt** tells the agent to call it before asking the user what they
  are looking for, to say what it searched on, and to send a user with no profile to
  `/my/profile` rather than rebuilding the profile through questions.
- **`resumeextract.Professional`** — the contact-free projection of the structured
  résumé: everything except `full_name`, `email`, `phone` and `links`. A whitelist,
  so a field added to `Structured` later is withheld until it is added here too.
- **`matchanalysis.candidateContext` moves onto that projection.** It strips the same
  four fields today, but by deleting known keys from an unmarshalled JSON map — a
  blacklist, so a field added to `Structured` would reach the LLM prompt verbatim.
- **`GET /me/profile` gains the same `cv` block and accepts an API key** (`PUT` and
  `DELETE` stay cookie-only), so the `freehire` CLI can ground itself the same way
  the in-app agent now does.

## Capabilities

### New Capabilities

None — this extends two existing capabilities.

### Modified Capabilities

- `assistant-agent-runtime`: the tool surface gains `get_profile`, and a new
  requirement fixes the behaviour that matters — the agent reads the saved profile
  instead of interrogating the user, the result carries no contacts, and a user
  without a profile is sent to fill one in.
- `search-profiles`: the profile read accepts an API key as well as the session
  cookie, and its response carries the contact-free structured CV.

## Impact

- `internal/handler/assistant_profile_tool.go` (new), registered in the discovery set
  in `internal/handler/assistant_tools.go`; `assistantHandlers` gains the profile
  handlers so the tool and the endpoint share one assembly.
- `internal/assistant/prompt.go` — the chat playbook.
- `internal/resumeextract/structured.go` — the projection (in the file
  `cmd/gen-contracts` reads, so the TypeScript type comes with it).
- `internal/matchanalysis/analyzer.go` — the de-identification.
- `internal/handler/me_profile.go`, `internal/handler/handler.go` — the `cv` block and
  the read's gate.
- `web/src/lib/types.ts` and the generated contracts.

No schema change, no migration.
