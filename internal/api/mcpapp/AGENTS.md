# internal/api/mcpapp

The MCP server freehire's ChatGPT app is built on. Read-only, anonymous, mounted at
`/api/v1/mcp`.

A second, authenticated server (`account.go`) lives in this package too, mounted at
`/api/v1/mcp/account` (freehire#3114): it acts as the signed-in account that approved an
OAuth grant, reusing the in-app assistant's own tools — including writes (save, mark
applied, set a stage/note) — rather than reimplementing them. It has no `Reader` of its own
and shares nothing below the tool registration with the anonymous server beside it; the two
are grouped in one package because both speak MCP over the same SDK, not because they share
a catalogue view. Authentication for it is a Fiber middleware in
`internal/api/handler/mcpapp_account.go`, ahead of this package's own code — see
`AccountUserIDHeader`'s doc comment in `account.go` for why.

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

## Semantic search

`search_jobs` takes `semantic: true` and answers it through the same `semanticSearch` value
(`internal/api/handler/semantic_search.go`) OJCP and `/agent/jobs/search` hold, so the three
cannot disagree about when a query was matched by meaning. The result's `semantic` says
`applied` or `unavailable`; the latter is a keyword-only answer, never an error. It is off
unless asked: description vectors exist only for `is_tech` postings, so for most of the
catalogue the keyword hits are the answer anyway.

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

## Liveness is computed on read, and only in `get_job`

`get_job` publishes `last_seen_at`, `reality`, `fake_freshness`, `ghost_level` and
`ghost_criteria`, computed by the handler's `attachReality` (the same `realityAttacher` the
OJCP surface uses). A search result carries only `first_seen_at`.

- **Not the index's reality class.** It is computed when a document is pushed, and
  `cmd/reindex` pushes only documents whose content hash moved — so a posting indexed at three
  days old would still read "fresh" a quarter later. `first_seen_at` is a fact that cannot go
  stale; the model can compare it with `posted_at` itself.
- **Not ghost on search.** It costs two Postgres reads per page (absence stamps and outcome
  evidence), and `get_job` is one call away for the posting a person actually picked.
- **Absent, not false.** A lookup that fails leaves every signal off the wire. An empty level
  or a `false` flag would read as a clean bill of health nobody issued.

## `check_jobs_open`, `search` and `fetch`

- **`check_jobs_open`** answers open / closed / not_found for up to `maxOpenChecks` (50)
  slugs in one Postgres read (`ListJobLivenessBySlugs`). It reports a CLOSED posting as closed
  — that is the point of the tool — so it does not reuse `publishedToAgents`, which refuses
  closed postings; private and duplicate postings still answer not_found, as on `get_job`.
  Slugs past the cap are named back in `unchecked`, never dropped. A failed lookup is an error,
  never a page of not_found.
- **`search` / `fetch`** exist for ChatGPT's deep-research and company-knowledge connectors,
  which call exactly these names and read exactly these shapes (`{results: [{id, title, url}]}`
  and `{id, title, text, url, metadata}`) from the tool's TEXT content — the SDK fills that
  from the typed output when a handler leaves `Content` empty. They adapt `SearchJobs` and
  `JobDetail` (`research.go`); they are not a second catalogue view and add nothing to the
  `Reader`.

## `similar_jobs`, `salary_stats` and `job_market_stats`

All three read what the website already computes, through functions the website's own
handlers now share, so the two cannot quote different figures:

- **`similar_jobs`** — `similarViews` (handler/similar.go) over the precomputed
  `similar_job_ids`. The SOURCE posting is checked against `publishedToAgents` first, or a
  private slug would answer with neighbours while an unknown one answers not found. Neighbours
  pass the same predicate (the website's rule plus duplicates). Empty for postings whose
  neighbours `cmd/similar-backfill` has not computed yet.
- **`salary_stats`** — `salaryBands` (handler/insights.go), the `/insights/salary` read,
  including its per-seniority breakdown for a category alone. Unknown scope values are dropped
  and named, as on `search_jobs`.
- **`job_market_stats`** — `cachedFacetCounts` + `facetView` (handler/facets.go) over the
  filter `search_jobs` would build for the same input, sharing the website's facet cache. The
  projector keeps the top `marketTopValues` of the facets in `marketFacets`; cities are left
  out, their long tail is noise in a chat turn.

Trends (`/insights/companies`, `/insights/skills`) are deliberately not exposed yet: the
company ranking is led by aggregator boards rather than employers.

## Prompts

`prompts.go` registers three prompts — `jobs_for_my_stack`, `check_my_shortlist`,
`salary_check` — templates a PERSON picks in their client (Claude lists them as commands); the
model never calls them. Each renders one plain user request and names no tool: which tool
answers it is the server instructions' job. A missing required argument is an error; an
omitted optional one leaves no trace in the text.

An argument whose values come from a closed dictionary (`seniority`, `role`) declares it in
its spec's `vocabularies`, and `completion/complete` offers the dictionary's values as the
person types — the vocabulary itself, never a copy. A free-text argument (`stack`,
`postings`) offers nothing: a client replaces the whole field with what it picks, so a
suggestion for one word of "Go and Kubernetes" would erase the rest.

## The widget

`search_jobs` and `get_job` render in an MCP Apps widget (`widget.go`, `widget/jobs.html`):
one self-contained page an MCP Apps host — ChatGPT, Claude — draws the result in as cards,
instead of the model retelling it as prose. The tools' contracts are untouched; a client
that ignores `_meta.ui` sees exactly what it saw before.

- **It is hand-written HTML, not a `web/` build, on purpose** — see
  `openspec/changes/archive/2026-10-20-chatgpt-app-widgets/design.md`. If it ever grows
  forms, sign-in or its own requests, that is the point to move it.
- **The URI is the page's hash.** Hosts cache by URI; editing the file changes it. Never
  hard-code the URI anywhere else.
- **Wire data lives only in `result`, `job` and `req`** inside the page.
  `TestTheWidgetReadsOnlyFieldsTheWireShapeHas` greps those names and checks every field
  against `wire.go`, so a renamed field fails the build instead of rendering as nothing.
  Reading wire data through any other name blinds it.
- **Text only, no requests.** `TestTheWidgetCannotInjectMarkupOrMakeRequests` bans
  `innerHTML` and friends and every network API; the CSP admits only
  `https://logo.freehire.me` images. A URL is used only when it is `https:`.
- **Only a tool whose result the page can draw may name it** — a host draws the widget for
  every tool that does.
- **The handshake's field names are the specification's** (`appInfo`, not `clientInfo`),
  and a test holds them. The reference `AppBridge` accepts the wrong one without complaint,
  so a manual check cannot catch it.
- **Checking it by eye**: bundle the official host, `AppBridge` from
  `@modelcontextprotocol/ext-apps/app-bridge`, feed it real `tools/call` results from
  production, and drive it in a real browser (`agent-browser`). Connect the bridge BEFORE
  setting the iframe's `srcdoc` — the page's `ui/initialize` is its first act, and a bridge
  connected after load never hears it. Headless Chrome's `--virtual-time-budget` drops the
  iframe's `postMessage` traffic and renders an empty frame; do not use it.

## Descriptions say what a tool returns, never which tool to call

Anthropic's directory policy rules out tool descriptions that instruct the model about other
tools ("call get_job for the full text" was in three places). A tool or field description
states what comes back; cross-tool guidance, where needed, goes in `serverInstructions`.
`TestNoToolDescriptionDirectsTheModelToAnotherTool` walks the registered tools and both
schemas to hold the line.

## Annotations are load-bearing

Every tool declares `readOnlyHint: true`, `destructiveHint: false`, `openWorldHint: true`.
Incorrect annotation of these three is a named rejection reason in OpenAI's submission
guidelines. The block is written once (`readOnly()`) and a test walks the REGISTERED tools
rather than a list beside them — checked by removing one annotation and watching it fail.

A tool on THIS (anonymous) server that writes anything belongs behind authentication, which
this surface does not have — that is what `account.go`'s separate, authenticated server is
for; do not add a write tool here instead of there.

The account server's annotations come from each tool's own `assistant.Effect` —
`EffectReads`, `EffectAdds` (nothing already held is lost: `save_job`, `apply_job`) or
`EffectOverwrites` (`unsave_job`, and `track_job`, which REPLACES the note). The zero value
is `EffectOverwrites` on purpose: a tool that declares nothing is announced as one that may
lose data. `TestMCPAccountTools_DeclareTheirEffect` walks the set the mount actually serves,
so a tool joining it must be classified there.

The account server also serves auto-apply and the screening answers
(`internal/api/handler/assistant_auto_apply_tools.go`, openspec/changes/auto-apply-over-mcp):
start, list, get, review, retry and cancel an attempt, plus the answer bank and the six typed
screening facts. The four that ACT on an attempt refuse unless the account turned on
`users.agents_may_apply`, which only a browser session can write — reading never needs it.
An answer saved here is the CANDIDATE's (`answerbank.AuthorCandidate`, so it is sent): this
server only ever acts for the person who approved the connection. `accountServerInstructions`
spells out the order the tools only work in.

`present_jobs` is NOT served there (`mcpAccountTools`). It draws cards in our own chat UI and
its description tells the model never to write a job link; in a client with no card renderer
a model obeying it showed the person no vacancies at all.

Every server — both here and the OJCP one — introduces itself through `Identity`, which adds
the site's logo and website link when the origin is known.

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
