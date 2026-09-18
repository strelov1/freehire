## Context

OpenAI's Apps SDK asks a provider for four things: an MCP server speaking the streamable
HTTP transport at a stable public URL, tools whose names and descriptions match what they
do, correct safety annotations, and structured results the model can read. It does not ask
for a UI, and it does not ask for authentication when the data is public.

This repository already answers the first of those. `internal/api/ojcpmcp` builds an
`mcp.Server` from the official `modelcontextprotocol/go-sdk` and mounts it through Fiber's
`adaptor.HTTPHandler` at `/api/v1/ojcp/mcp`; a probe against production on 2026-09-18
returned a well-formed `initialize` result on protocol `2025-06-18`. The three tools behind
it load through a `Reader` interface the REST handlers satisfy, so both transports answer
from one implementation.

The shape those tools answer in, however, is OJCP's, and OJCP is a lossy target by
construction. The measurement recorded when that surface shipped: 34 fields of
`jobview.Job` have no home in the standard's `JobPosting`, and 18.1% of open postings carry
a seniority the standard's `experience_level` enum cannot name. A chat user asking "what
does it pay and is it staff-level" is asking for two of the fields that fall out.

## Goals / Non-Goals

**Goals:**

- Be an installable ChatGPT app: an MCP server ChatGPT can scan, call, and be listed for.
- Answer in our own wire shapes, so nothing a visitor could see on the site is withheld from
  someone who asked through ChatGPT.
- Carry attribution — source name and the employer's own URL — on every result.
- Leave the OJCP surface and its 9/9 conformance untouched.
- Keep the tool implementations testable without a database, a network, or Fiber.

**Non-Goals:**

- Authentication. No candidate data, no saved jobs, no tracking, no CV. Those need an OAuth
  2.1 authorization server this deployment does not have.
- UI widgets. Not required by the SDK, and a frontend bundle is its own change.
- A third transport. REST already exists for anyone who wants plain HTTP.

## Decisions

### A second MCP server, not a widened first one

`internal/api/mcpapp` is new, and `internal/api/ojcpmcp` is not edited.

The alternative — add titles and annotations to the OJCP server and submit that — is a few
hours of work and wrong in two directions at once. ChatGPT would permanently see the
OJCP-shaped answer, which is the lossy one; and every future edit made to satisfy an
OpenAI reviewer would land on a surface whose value is that it passes somebody else's
conformance suite. Two consumers with different contracts get two adapters over one core,
which is the arrangement REST and MCP already have here.

What is shared is everything below the adapter: the `Reader` the new server takes is
satisfied by the same handler struct that serves `/jobs/search`, calling
`search.FilterFromValues` on `url.Values` exactly as `ojcpHandlers.SearchJobs` does. The
visibility predicate — open, canonical, non-private — is therefore not re-decided here. It
cannot be: this surface never builds a query of its own.

### The route is `/api/v1/mcp`, and it cannot be `/mcp`

nginx routes `/api/` to the Go process and everything else to the SvelteKit front end. A
route registered at a bare path never receives a request — which is exactly how the OJCP
manifest 404'd in production while the API path beside it answered 200, and why that
manifest is proxied by the SPA rather than served from Go. The app's declared MCP endpoint
is `https://freehire.me/api/v1/mcp`, and no prettier URL is available without an nginx
change on a host whose configuration lives outside this repository.

### Four tools, and search does not return full descriptions

`search_jobs`, `get_job`, `search_companies`, `get_company`.

`/agent/jobs/search` rehydrates every hit's full description from Postgres, because a
programmatic consumer reading the whole body is the case it was built for. That behaviour
is wrong here and must not be copied: a chat turn holding ten full job descriptions spends
the context window on text the model will summarise to one line each. `search_jobs`
therefore serves the index's truncated preview, and `get_job` — one posting, chosen by the
user — is the only tool that reads a full body.

Company tools are included rather than deferred because company pages are measurably this
site's strongest asset (Bing holds 255k of them, and their highest-impression queries have
the shape "<company> careers"). A user asking ChatGPT about an employer is the query we
already win elsewhere.

### A curated filter vocabulary, not all fifty

`/jobs/search` accepts around fifty parameters. Publishing all of them as a tool's input
schema would put that schema in front of the model on every turn, for filters a
conversational user does not ask for (`skills_mode`, `role_type_exclude`,
`open_within_days`).

The tool publishes the subset a person actually says out loud: free text, country, city,
work mode, seniority, category, skills, employment type, salary floor with currency,
visa sponsorship, English level, company, source, posted-within-days, and page bounds. The
rest stay reachable through REST and the OpenAPI document.

A parameter the tool cannot honour is reported, never dropped in silence: the result carries
the existing `ignored_params` reporting, which is this repository's standing answer to a
filter that would otherwise widen an answer without saying so.

### Errors are tool results, not protocol errors

`ojcpmcp` renders a failure as a `*jsonrpc.Error` carrying the OJCP error envelope, because
the standard asks for that. This server does the opposite: a failure is a `CallToolResult`
with `IsError` set and a plain sentence the model can act on ("no posting with that id" →
try a search).

The difference is the audience. A conforming agent branches on an error code; a language
model reads a sentence and recovers. A JSON-RPC protocol error tends to surface to a ChatGPT
user as a generic failure, which is one of the behaviours OpenAI's review names — clear
error messaging and fallback behaviour are a stated requirement, and a wrong annotation or a
dead-end error is among their commonly cited rejection reasons.

Not-found stays distinguishable from broken, as it is on the OJCP side: the first names what
was missing, the second says only that this deployment could not answer.

### Annotations are load-bearing, not decoration

Every tool is `readOnlyHint: true`, `destructiveHint: false`, `openWorldHint: true`. All
four read; none writes; all reach a catalogue that changes outside this conversation.

OpenAI's submission guidelines name incorrect annotation of exactly these three hints as a
common rejection reason. A test asserts the annotations on every registered tool, so an
added write tool cannot inherit a read-only claim by copy-paste.

### Attribution travels with the result

Each posting carries `source` (which board it came from) and the employer's own
`official_job_url` beside our `url`. The OJCP surface already publishes both.

This is a correctness decision before it is a review one. The channel renders results as
prose, where a link is easy to drop; a result that names its source cannot be repeated as
though we were the employer. That it is also the honest answer to a reviewer asking whether
an aggregator has the right to this data is a consequence, not the reason.

## Risks / Trade-offs

**Review may read an aggregator as a scraper.** OpenAI's rejection list names unauthorised
third-party API integration and scraping. freehire crawls 264 sources. Mitigation is in the
product rather than the pitch: per-result source attribution, the employer's own link on
every posting, and a listing description that says plainly what this is. Being one of three
implementations of OJCP — a standard whose committee seats Workday — is corroboration, not
a defence. This risk cannot be eliminated from our side; it can only be answered honestly.

**Two MCP servers can drift.** They share a `Reader` and a search core, so they cannot
disagree about what is in the catalogue. They can disagree about what they publish, which is
intended — the whole reason for the second server. The invariant worth testing is narrower:
that both refuse the same postings.

**A curated filter subset hides capability.** A user asking for something the tool does not
take gets a wider answer plus an `ignored_params` note, not a refusal. That is the standing
trade in this repository; the alternative breaks saved searches elsewhere and is worse.

## Migration Plan

None. No schema change, no data change, no existing caller affected. The route is additive
and the rollback is to stop serving it.

Submission to the directory is sequenced after the server is live and exercised in ChatGPT's
developer mode, because the required screenshots must show the app running inside ChatGPT,
and the required test cases must pass on web and mobile.

## Open Questions

None blocking. The listing copy (name, short and long description, icon) is a writing task
carried in `tasks.md`, not a design decision.
