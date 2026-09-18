# internal/api/mcpapp

The MCP server freehire's ChatGPT app is built on. Read-only, anonymous, mounted at
`/api/v1/mcp`.

## Why there are two MCP servers

`internal/api/ojcpmcp` serves the Open Job Context Protocol. This one serves OpenAI's Apps
SDK. They share the reader, the search core and the visibility predicate, and differ in
everything above them — which is the reason there are two rather than one with a flag.

| | `ojcpmcp` | `mcpapp` |
|---|---|---|
| Audience | an agent that branches on error codes | a language model that reads prose |
| Wire shape | OJCP's schema | ours (`jobview.Job`, flattened) |
| Failures | `*jsonrpc.Error` carrying the OJCP envelope | `CallToolResult` with `IsError` and a sentence |
| Seniority | translated into `experience_level`, losing what it cannot name | passed through |
| Bound by | a published conformance suite (9/9) | OpenAI's app review |

The OJCP projection drops 34 of `jobview.Job`'s fields, and 18.1% of open postings carry a
seniority (`intern`/`staff`/`principal`) its vocabulary cannot name. Widening the OJCP
surface to please ChatGPT would put a conformance-tested surface at risk for an audience it
was not built for; narrowing this one into OJCP's schema would publish less than the website
shows. Neither is a trade worth making.

**Do not make them share a tool registration.** What they must share — and do — is the
`Reader`, so they cannot disagree about what is in the catalogue.

## What the tools must not do

- **Return a full description from `search_jobs`.** `/agent/jobs/search` rehydrates every
  hit's body from Postgres, deliberately, because a programmatic consumer reading whole
  bodies is what it is for. Here ten of those in one turn is the context window spent on
  text the model reduces to a line each. `summaryMaxChars` bounds the preview here rather
  than trusting the caller, because the bound is a property of this tool and not of whoever
  filled the field.
- **Publish a link we cannot vouch for.** `official_job_url` is filled only for a provider in
  `sources.EmployerURLProviders()` — an ATS or a company's own careers site. That set is
  resolved ONCE, in `NewProjector`, and the shape of the API is why: the rule used to be a
  per-provider predicate, and each call rebuilt all 223 adapters (35µs, 394 allocations),
  which a ten-result page paid ten times over. An
  aggregator's stored URL points at the aggregator, and this channel renders it as a link a
  person clicks expecting the employer. The tag is stripped too (`outboundurl.Untag`):
  `jobview` stamps `utm_source` on everything it serves, and this field is what a consumer
  deduplicates and domain-verifies against. Our own `url` keeps the tag.
- **Narrow on a value the catalogue cannot filter on.** Measured against production on
  2026-09-18: `/jobs/search?category=ai` answers 0 results and reports nothing ignored. Over
  REST that is a client's problem; here the client summarises the zero as "freehire has no
  AI jobs" and says it to a person. So a value outside a closed vocabulary is DROPPED and
  named in `ignored_params`, and the free text still runs.

## Annotations are load-bearing

Every tool declares `readOnlyHint: true`, `destructiveHint: false`, `openWorldHint: true`.
Incorrect annotation of these three is a named rejection reason in OpenAI's submission
guidelines. The block is written once (`readOnly()`) and a test walks the REGISTERED tools
rather than a list beside them — checked by removing one annotation and watching it fail.

A tool that writes anything belongs behind authentication, which this surface does not have.

## Testing

Build fixtures with `jobview.FromRow(db.Job{...})`, never as a `jobview.Job` literal — the
same rule `internal/api/ojcp/AGENTS.md` records, and for the same reason: the literal skips
`outboundurl.Tag` and the facet normalisation. Three assertions in this package were green
against a literal and failed the moment the fixture went through a real read path.

**The dictionary columns win.** `seniority`, `category`, `employment_type`, `english_level`
and `education_level` are folded over the model's enrichment by `jobview`, always. A fixture
that sets them inside the enrichment JSON sets nothing at all.

Drive the server through the SDK's in-memory transport, not by calling handler functions: a
tool that registers but cannot be called is invisible to a direct call.

## Measured, not assumed

- The SDK **rejects an argument the input schema does not declare** before the handler runs
  (`isError`, handler never reached) — it derives the schema from the Go type with
  `additionalProperties: false`. An invented parameter is therefore not a case this has to
  report; a value outside our vocabularies is. The spec's requirement is written around that
  measurement rather than around the guess that preceded it.
- Resolving `EmployerURLProviders()` costs **35µs and 394 allocations** — it constructs all
  223 adapters. Once per projector, never per posting.
- Meilisearch filter comparison is **case-insensitive**: `countries=de` and `countries=DE`
  both answered 59,942 on 2026-09-18. The vocabulary check folds case to match, because a
  check stricter than the thing it guards would report a working filter as unsupported.
