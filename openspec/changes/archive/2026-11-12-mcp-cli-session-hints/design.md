## Context

`MixedHandler` (`internal/api/mcpapp/mixed.go`) builds a fresh stateless `*mcp.Server` per
HTTP request via `newMixedServer`, which composes the instructions from the tools the
session is served. The handler already branches on User-Agent (`openai-mcp`,
`signsInOnlyAtConnect`). The freehire CLI's HTTP client sends `User-Agent:
freehire-cli/<version>` on every request (`internal/client/client.go` in `freehire-cli`), so
the `freehire mcp` proxy will carry it without extra work.

## Goals / Non-Goals

**Goals:** an agent driving freehire through the CLI learns, once per session, that the
CLI is on its shell for disk-bound work.

**Non-Goals:** the proxy itself; the account alias `/api/v1/mcp/account` (the proxy
targets the canonical endpoint only); per-result `next` hints; any change to what tools
return.

## Decisions

- **Instructions, not `next`.** "This agent has a shell" is a fact about the session, not
  about one result. `next` is replayed into context with every search and job result;
  the instructions are read once. The article's per-result hint idea costs more tokens for
  the same information.
- **User-Agent prefix, no new header.** The CLI already identifies itself; a dedicated
  header would need a CLI change to say what the User-Agent already says. Spoofing it
  gains nothing — the sentence grants no capability, it only names a public tool.
- **One sentence for anonymous and signed-in sessions.** `freehire search` needs no
  account; `cv render` does, and the CLI itself answers "run `freehire auth login`" when it
  is missing — which is the right pointer, so the server does not branch.
- **Own budget (256 chars), outside the 720-char account budget.** The existing budget test
  guards what every host reads; the CLI sentence is appended only for the CLI, and Claude
  Code's 2048 cut leaves ample room.

## Risks / Trade-offs

- [The CLI's command names change] → the sentence would point at a missing command. The
  CLI repository is ours; its `search`/`cv render` are documented on `/cli`. A test pins the
  sentence text so a change here is deliberate.
- [Another tool sends a `freehire-cli/` User-Agent without a shell] → it receives one
  harmless extra sentence.
