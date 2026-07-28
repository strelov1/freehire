## Context

Today the assistant is an external process. `AssistantChat.svelte` speaks the roy
control protocol (`wire.ts`) over a WebSocket relay to `freehire-agent`, which
holds sessions in its own SQLite + JSONL journals and spawns `claude-code-acp` on
a machine the user connected with `freehire runner`. The agent acts on freehire
through the `freehire` CLI binary, authenticating with a short-lived `cv`-scoped
API key that `mintTailoringKey` issues per tailoring session. Ownership is
enforced at the roy-management boundary (`session_meta.created_by`).

The hire backend already has the parts needed to host the agent itself:

- `internal/llm` — a langchaingo client over an OpenAI-compatible endpoint
  (litellm), with a per-call timeout, optional Langfuse tracing, and usage
  extraction. It currently exposes only one-shot JSON helpers.
- `internal/handler/match_analysis_stream.go` — a working SSE pattern on Fiber:
  `SetBodyStreamWriter`, named events, a heartbeat comment, and write-deadline
  handling.
- The services the CLI reaches over HTTP — `internal/search`, `internal/cv`,
  `internal/userjob`, `internal/jobtracking`, `db.Queries` — all already
  available on the `API` struct.

Constraints: the chat UI is liked and must keep its current behaviour; the change
must not disturb the public agent surfaces (CLI, MCP, plugin, `/agent/jobs/search`);
inference moves onto freehire's bill, so a turn must be bounded.

## Goals / Non-Goals

**Goals:**

- Run the assistant inside `cmd/server`, with no second runtime and no user-side
  installation.
- Give the model typed tools whose results are structured data, not parsed CLI
  output.
- Keep `AssistantChat.svelte` and its sub-components rendering the same way, by
  replacing only the transport modules beneath them.
- Persist sessions and transcripts in Postgres, owner-scoped by the same
  `RequireAuth` the rest of `/api/v1` uses.
- Bound a turn: a hard step cap, a per-call timeout, and cancellation on client
  disconnect.

**Non-Goals:**

- Metering or billing chat turns. CV tailoring keeps its existing per-tailor
  credit debit at bootstrap; chat turns are unmetered while behind the beta gate.
- Multi-agent or sub-agent orchestration, planner/critic loops, retrieval beyond
  the existing search tools.
- Changing the public CLI/MCP surface, or the `agent-jobs-search` capability.
- Structured job cards emitted from tool results. The existing URL unfurl
  (`unfurl.ts` + `JobCardUnfurl.svelte`) is kept as-is; emitting cards from
  `tool_result` payloads is a later refinement, noted as a seam.
- Removing the `runner` command from `freehire-cli`, or archiving the
  `freehire-agent` repository — follow-ups in those repositories.

## Decisions

### Own turn loop over langchaingo tool calls, not `agents.Executor`

`langchaingo`'s agent executors are built around textual ReAct prompting; its
native tool-calling support is the `llms.Tool` / `ContentChoice.ToolCalls` layer
on `GenerateContent`. The loop we need is small and its stopping rules are the
whole point:

```
for step := 0; step < maxSteps; step++ {
    choice := llm.GenerateContent(ctx, history, WithTools(tools), WithStreamingFunc(...))
    if len(choice.ToolCalls) == 0 { return final text }
    for each call: run tool → append tool-result message
}
// cap reached: one final call with tools withheld, forcing a text answer
```

*Alternative considered:* `agents.NewOpenAIFunctionsAgent` + `agents.Executor`.
Rejected: it owns the loop, the prompt, and the error handling we specifically
want to control, and it does not surface per-step streaming in the shape the UI
already consumes.

### `internal/llm` grows a chat method; `GenerateJSON` is untouched

Add a conversational entry point that takes `[]llms.MessageContent`, a tool list,
and a token-delta callback, returning the full `*llms.ContentChoice` (text +
`ToolCalls` + usage). The existing timeout, tracer, and usage plumbing are
reused. The JSON helpers keep their current signatures so enrichment,
telegram-extract, matchanalysis and resumeextract are unaffected.

### Tools call internal services, not our own HTTP API

A tool is a Go function `func(ctx, userID int64, raw json.RawMessage) (any, error)`
registered with an `llms.Tool` JSON schema. It calls the same service the HTTP
handler calls. This removes the minted API key entirely: the tool already knows
the caller.

`search_jobs` needs no new seam: `buildSearchFilter(c)` is already a one-line
delegation to `search.FilterFromValues(url.Values)`, and `a.search.Search` takes
a plain `search.SearchParams`. The tool renders its typed arguments as
`url.Values` and calls the same two functions the handler calls, so facet
semantics — disjunctive groups, `_mode=and`, `_exclude`, the location OR group —
are identical by construction rather than by duplication. Nothing in the search
handler is reshaped.

*Alternative considered:* have tools issue in-process HTTP requests
(`app.Test(req)`). Rejected: it re-serialises everything, re-runs auth, and keeps
the credential-minting problem alive in a new form.

### Two tool presets, one runtime

A session records a preset. `chat` registers the discovery and tracking tools;
`tailor` registers those plus the CV tools and is bound to a CV id and a job.
The preset selects the system prompt and the registered tool set — nothing else
differs. This keeps `AssistantChat.svelte` usable unchanged on both `/my/assistant`
and `/tailor/<slug>`, which is how it is already embedded.

*Alternative considered:* replace the tailor chat with deterministic
button-driven LLM calls. Rejected: the tailoring contract (`missing_gap`
requirements must be confirmed with the candidate before they are written) is a
dialogue, and removing the dialogue would let the agent fabricate.

### SSE for the turn, plain JSON for everything else

`POST /api/v1/assistant/sessions/:id/messages` streams the turn as named SSE
events, reusing the `writeSSE` pattern. Session create/list/delete and transcript
read are ordinary JSON endpoints. The roy protocol's `attach`/`from_seq`/
`acquire_input`/`release_input`/harness listing all disappear: a session is a row,
its transcript is a query, and the input lease is unnecessary when the turn is a
single HTTP request.

*Alternative considered:* keep the WebSocket and the roy wire shape so
`client.ts` survives untouched. Rejected: it would port a multi-process daemon's
protocol into a request/response server for no behavioural gain.

### The event union stays close to today's `TurnEvent`

The reducer in `chat.ts` and the tool cards in `tool-formatters.ts` are keyed off
`TurnEvent` variants. The SSE events keep the same names where they still mean
something — `user_prompt`, `assistant_text`, `assistant_thought`, `tool_use`,
`usage`, `result` — and add `tool_result` (which the CLI model had no place for).
`system`, `note` and `raw` are dropped. That keeps the reducer's shape and the
tool-card rendering nearly intact; `tool-formatters.ts` loses its shell-command
branches (`bashCommand`, `isNoiseShellCall`, `isFreehireGroup`) because there is
no shell any more.

### Transcript persistence

Two tables: `assistant_sessions` (id, user_id, preset, label, cv_id, job_id,
timestamps) and `assistant_messages` (session_id, seq, role, content jsonb,
created_at). The stored message shape is the LLM history — assistant tool calls
and tool results included — so a session resumes exactly where it stopped, and
the UI transcript is projected from the same rows. Ownership is a `WHERE
user_id = $1` on every read and write; there is no separate owner table and no
IDOR surface of the kind PR#7 fixed in roy.

### Bounding a turn

Three limits: `maxSteps` tool-call rounds per turn (default 8, config), the
existing per-call LLM timeout, and cancellation — when the client disconnects,
the request context is cancelled and the loop stops before the next LLM call.
Tool results are truncated to a byte cap before entering history, so one
`search_jobs` with full descriptions cannot blow the context window. The
assistant stays behind the beta-tester gate while it is free.

## Risks / Trade-offs

- **Model quality drops versus Claude Code.** The current agent is a
  purpose-built coding agent with its own planning; a plain tool loop on a
  general chat model may be worse at multi-step research. → Mitigate with a
  focused system prompt carrying the `using-freehire` playbook (read `facets`
  before filtering, canonical skill slugs, one job per line as
  `/jobs/<public_slug>`), and keep the model configurable so it can be raised.

- **Tool calling reliability varies by model behind litellm.** Some models
  behind the gateway emit malformed arguments. → Decode tool arguments strictly
  and return the decode error to the model as the tool result so it can retry;
  count a failed decode against the step cap. This mirrors how `cv.DecodePatch`
  already reports mis-addressed patches.

- **Cost is unmetered.** A beta tester can run many turns for free. → The beta
  gate plus the step cap bound it for now; `internal/credits` is the seam for a
  `FeatureChat` debit later.

- **Context growth on long sessions.** Full history plus fat tool results will
  eventually exceed the window. → Truncate tool results, and cap the replayed
  history to the most recent N messages when composing a turn. Summarisation is
  explicitly not in this change.

- **Losing BYO-Claude removes a privacy story.** "Your Claude credentials never
  reach our servers" goes away. → The user's data already lives on freehire's
  servers; only the inference moves. Worth a line in the changelog rather than a
  technical mitigation.

- **The tailoring response shape changes.** Dropping `cli_token` breaks any
  consumer reading it. → The only consumers are the tailor route and the CLI's
  tailoring flow; the CLI keeps working against user-created `cv`-scoped keys
  from `/me`, which are unaffected.

## Migration Plan

1. Ship the backend (tables, tools, loop, routes) with the old WebSocket path
   still live — nothing user-visible changes yet.
2. Switch the frontend transport modules and delete the runner UI. `/my/assistant`
   and `/tailor/<slug>` now use the in-process agent.
3. After a soak, retire the ops surface: remove `PUBLIC_ASSISTANT_ORIGIN` from
   the web env, drop the `agent` vhost and the two `freehire-agent-*` systemd
   units, and stop `release-agent.sh`.
4. Follow-ups outside this change: remove `runner` from `freehire-cli`, archive
   the `freehire-agent` repository.

Rollback: steps 1–2 are a single deploy of the hire repo; reverting it restores
the WebSocket client, and the agent backend is still running until step 3.
Existing roy sessions are not migrated — history there is disposable chat, and
tailored CVs keep their documents regardless of which session produced them.

## Open Questions

- Which model backs the assistant? It needs solid tool calling and a large
  context; the enrichment model is chosen for cheap JSON extraction and is
  probably not the right default. To be set as its own config value.
- Should a tailored CV's stored session id be cleared on migration, so an old
  roy session id does not resolve to a missing assistant session? Cheapest is to
  treat an unknown session id as "start a fresh one", which the workspace
  already does when a CV has no session.
