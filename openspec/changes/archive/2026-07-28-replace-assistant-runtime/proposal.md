## Why

The in-app assistant does not run in freehire at all: it runs on the user's own
machine. `/my/assistant` and `/tailor/<slug>` talk over a WebSocket relay to
`freehire-agent` (a Rust fork of roy, deployed as its own vhost and two systemd
units), which spawns Claude Code on a machine the user connects with
`freehire runner`, and whose only tool is a shell whitelist over the `freehire`
CLI binary. That buys zero token cost, and costs: five install steps before the
first message, a dead chat whenever the runner is not running, three repositories
and a separate release path to keep in sync, and a model that drives typed
domain operations by writing CLI strings and reading back human-shaped output.

freehire already owns everything needed to run the agent itself — an
OpenAI-compatible LLM client (`internal/llm` over the litellm gateway, with
Langfuse tracing), the data and services the CLI reaches over HTTP, and an SSE
streaming pattern in the API. Running the agent in-process removes an entire
runtime, an entire protocol, and the credential-minting that only existed
because the agent was somebody else's process.

## What Changes

- **New:** a server-side tool-calling agent in the Go backend
  (`internal/assistant`): a bounded loop over `internal/llm` using langchaingo
  tool calls, with a typed tool registry, per-session history in Postgres, and
  turn streaming over SSE on the existing `/api/v1` surface.
- **New:** typed tools replacing `Bash(freehire:*)`. The surface mirrors the
  `freehire` CLI's commands — `facets`, `search_jobs`, `get_job`, `get_company`,
  `market_fit`, `save_job`, `unsave_job`, `apply_job`, `track_job`, `my_jobs`,
  `cv_context`, `cv_get`, `cv_edit`, `cv_render` — but calls the same internal
  services the HTTP handlers use, as the authenticated caller. Moderator-only
  CLI commands (job authoring, submissions) are out of scope.
- **Changed:** the chat UI keeps its current look and behaviour. Only the
  transport layer under it is replaced — `web/src/lib/assistant/{wire,client,api}.ts`
  move from the roy `ClientCommand`/`ServerEvent` WebSocket protocol to an SSE
  turn stream; `AssistantChat.svelte`'s markup, tool cards, thinking panel,
  session rail, and job-card unfurl stay.
- **Changed:** a tailoring session no longer mints a short-lived `cv`-scoped API
  key. The agent runs as the caller in-process, so `TailorCV` and
  `StartTailorSession` stop returning `cli_token`. **BREAKING** for the
  documented tailoring-bootstrap response shape.
- **Removed:** `freehire runner` / bring-your-own-Claude. `RunnerSetup.svelte`,
  `RunnerBadge.svelte`, `NoDeviceError`, and the `no_device` state disappear —
  there is nothing to install and nothing to connect.
- **Removed:** the `freehire-agent` deployment. `PUBLIC_ASSISTANT_ORIGIN`, the
  `/assistant-api` nginx location and Vite proxy, the `agent.freehire.me` vhost,
  and the two systemd units are retired. Archiving the `freehire-agent`
  repository and dropping the CLI's `runner` command are follow-ups in those
  repositories, not this change.
- **Unchanged:** the public agent surfaces — the `freehire` CLI, the `freehire-mcp`
  server, the Claude Code plugin, and `GET /api/v1/agent/jobs/search` — keep
  working exactly as they do today. This change replaces the *in-app* assistant's
  runtime, not the API those surfaces consume.

## Capabilities

### New Capabilities
- `assistant-agent-runtime`: the in-process agent — session lifecycle, the
  bounded tool-calling turn loop, its streamed turn events, the typed tool
  registry and its two presets (general chat / CV tailoring), and the failure
  and cancellation behaviour of a turn.

### Modified Capabilities
- `assistant-sessions`: sessions become freehire's own resource (owner-scoped
  list, create, delete, transcript replay served by the freehire API from
  Postgres) instead of an external agent backend's `session_meta`; access is
  gated to beta testers again, since inference is now billed to freehire.
- `assistant-job-cards`: the agent is directed to emit canonical
  `/jobs/<public_slug>` URLs by its own system prompt and tool contract rather
  than by the `using-freehire` CLI skill.
- `cv-tailoring`: the requirement that the tailoring agent authenticates with a
  minted, short-lived scoped credential is removed — the in-process agent acts
  as the authenticated caller directly, and CV tools are confined to the
  caller's own CVs by the same owner checks as the HTTP endpoints.

## Impact

**Backend (hire):** new `internal/assistant` package (agent loop, tool registry,
tool implementations); `internal/llm` gains a tool-calling, streaming generate
method alongside `GenerateJSON`; new migration for `assistant_sessions` and
`assistant_messages` plus sqlc queries; new routes under
`/api/v1/assistant/...` wired in `internal/handler`; `cv_tailor.go` drops
`mintTailoringKey` and the `cli_token` field; config gains the assistant's model
and step-cap settings.

**Frontend (web):** `wire.ts` shrinks to the new turn-event union; `client.ts`
becomes an SSE client; `api.ts` points at `/api/v1/assistant`; `RunnerSetup`,
`RunnerBadge` and the runner-status polling are deleted; `AssistantChat.svelte`
loses its runner branches and keeps everything else; the Vite `/assistant-api`
proxy is removed.

**Ops (freehire-ops):** the `agent` vhost, both `freehire-agent-*` systemd units,
`release-agent.sh`, and `PUBLIC_ASSISTANT_ORIGIN` in the web env are retired
after this ships.

**Cost:** token spend moves from the user's Claude subscription to freehire's
LLM gateway. There is no metering in this change — the assistant stays free
while it is tested behind the beta gate, and CV tailoring keeps its existing
per-tailor credit cost at bootstrap. `internal/credits` remains the seam for
metering chat turns later.
