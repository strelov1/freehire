## Why

ChatGPT is already this catalogue's single working acquisition channel — 53.8% of referred
traffic against 0.68% from the next one — and every visit it sends arrives because a model
decided to mention us, not because we published anything it could call. OpenAI's app
directory closes that gap: an app the user installs is one ChatGPT can call directly, with
our filters and our data, and it is listed where people go looking.

We are unusually close to shipping one. The Apps SDK asks for an MCP server over streamable
HTTP at a stable public URL, tools with honest metadata, and structured results. The
transport already exists in this repository and is live: `internal/api/ojcpmcp` serves
`https://freehire.me/api/v1/ojcp/mcp` through the official Go SDK, and it passed OJCP's
conformance suite 9/9.

What is missing is not plumbing. It is that the OJCP surface is the wrong shape for this
audience twice over. It projects our postings into a standard that cannot hold them — the
17.09 measurement found 34 of our fields with no home in the schema, and 18.1% of postings
carry a seniority (`intern`/`staff`/`principal`) the standard's vocabulary does not name —
and it carries none of the metadata OpenAI reads: no tool titles, no `readOnlyHint`, no
`openWorldHint`. Editing a conformance-tested surface to please a second consumer would put
both at risk for the benefit of neither.

## What Changes

- Add `internal/api/mcpapp`, a second MCP server serving the SAME reads through OUR wire
  shapes rather than OJCP's, mounted at `/api/v1/mcp`.
- Expose four read tools — `search_jobs`, `get_job`, `search_companies`, `get_company` —
  each carrying the title, the when-to-use description, the output schema and the
  `readOnlyHint`/`openWorldHint` annotations the Apps SDK expects.
- Publish a curated ~15-parameter subset of the search filter vocabulary as the tool's input
  schema, and report the rest through the existing `ignored_params` reporting rather than
  narrowing silently.
- Carry the source name and the employer's own `official_job_url` on every result, so
  attribution travels with the record through a channel that renders it as prose.
- Register the new package in `internal/platform/arch/layering/blocks.go`; a package in
  neither table fails the layering guard.
- Add the directory submission material: app icon, name, descriptions, support contact, and
  the test cases OpenAI's review exercises.

No existing endpoint changes SHAPE, and the OJCP manifest continues to declare only what it
serves. Three things outside this surface did change, and they are recorded here rather than
left to a commit message:

- **A live defect in the company facet filter, fixed.** `countries=DE,BR` matched a single
  literal value and answered zero where each code alone answered thousands — in BOTH company
  backends, and therefore on the public `/companies` endpoint. The job search has always
  split on commas. Found by review of this change, because `search_companies` joins its
  country list the same way and would have inherited it.
- **`internal/ingest/sources` gains `EmployerURLProviders`.** Which providers publish a URL
  that really is the employer's own page is a fact about sources, and this change made it a
  question TWO surfaces ask. One answer, not a copy each.
- **`internal/api/ojcp/projector.go` calls that shared rule** instead of spelling it out.
  Behaviour identical; its own tests pass unchanged.

## Capabilities

### New Capabilities

- `chatgpt-app`: freehire as an installable ChatGPT app — the second MCP server, its four
  read tools, the curated filter vocabulary they publish, and the rule that a tool's
  declared annotations match what it actually does.

### Modified Capabilities

None.

## Impact

- **New code:** `internal/api/mcpapp` (the server and its tools),
  `internal/api/handler/mcpapp.go` (the route and the reader it injects).
- **Untouched:** `internal/api/ojcpmcp`, the database. No migration.
- **Touched outside this surface**, as argued above: `internal/api/ojcp/projector.go`
  (one helper now calls the shared rule), `internal/ingest/sources` (the shared rule),
  `internal/search/search` and `internal/api/handler/companies.go` (the comma fix, which is
  a behaviour change to the public `/companies` endpoint — a correction, not a feature).
- **Not in scope:** authentication of any kind, and therefore `save_job`,
  `track_application` and anything reading a candidate's CV. Those need freehire to become
  an OAuth 2.1 authorization server with dynamic client registration, which it is not; that
  is a separate change, `chatgpt-app-authenticated`.
- **Also not in scope:** UI widgets (`ui://` resources rendered in ChatGPT's iframe). The
  Apps SDK does not require them, and they are a frontend bundle with its own build and CSP.
  A separate change, `chatgpt-app-widgets`, once developer-mode use shows what people
  actually ask for.
- **Rollback:** stop serving `/api/v1/mcp`. An installed app whose server does not answer
  stops being called; nothing else in the deployment depends on the route.
