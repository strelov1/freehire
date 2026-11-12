## Why

An agent reached through MCP holds every tool result in its context, so the canonical
endpoint deliberately serves bounded results (400-rune previews, no PDFs). An agent that
also has a shell does not need that compromise: the freehire CLI can write whole result
sets and rendered CVs to disk, where the agent reads them in pieces. The upcoming
`freehire mcp` command (stage 1 of the CLI+MCP merger, in the separate `freehire-cli`
repository) runs the MCP session through the CLI, so the server knows such an agent has a
shell — but the agent does not know the CLI is there unless the server says so.

## What Changes

- When the canonical MCP endpoint (`/api/v1/mcp`) is reached by the freehire CLI
  (User-Agent `freehire-cli/<version>`, which the CLI already sends on every request), the
  server instructions end with one sentence naming the CLI as available on the shell for
  work that belongs on disk: whole search results with full descriptions
  (`freehire search … --json > file`) and the CV PDF (`freehire cv render … --out`).
- Every other host receives exactly the instructions it receives today.
- No tool description changes; the sentence lives only in the session instructions.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `mcp-account-server`: adds a requirement that a session reached through the freehire
  CLI is told the CLI is on its shell.

## Impact

- `internal/api/mcpapp` (`mixed.go`: the canonical handler and server instructions) and its
  tests; `internal/api/mcpapp/AGENTS.md`.
- No API, schema, migration or dependency changes. The `freehire mcp` proxy itself and the
  porting of the CV/experience tools (stage 2) are separate changes.
