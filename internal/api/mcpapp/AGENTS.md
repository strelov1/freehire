# internal/api/mcpapp

The canonical MCP at `/api/v1/mcp` combines anonymous catalogue reads with
OAuth account tools (`mixed.go`). Anonymous and OAuth sessions discover the same
54 tools; personal calls without authentication never reach business code.
ChatGPT (User-Agent `openai-mcp`) gets an error result carrying `mcp/www_authenticate`,
the in-band challenge its Apps SDK opens sign-in from. Every other host gets HTTP 401 with
`WWW-Authenticate`, because Grok and Claude.ai ignore the in-band challenge and open sign-in
only on the specification's 401 — an anonymous connection there otherwise never sees a way to
sign in. Grok (`grok-connectors-manager`) and Claude.ai (`Claude-User`) go further: they offer
sign-in only while a connector is added, and a 401 later in a conversation opens nothing, so
an anonymous request from them is refused outright (`signsInOnlyAtConnect`) and those two
hosts have no anonymous search. `search_jobs`/`get_job` results carry `_meta["freehire.me/account"].signed_in` so the
widget shows one Connect panel instead of per-card buttons that would fail.
Per-tool `_meta.securitySchemes` declares access.

The eleven catalogue tools retain their public schemas, previews and widgets.
The strict bearer-only `/api/v1/mcp/account` alias (`account.go`) retains its
legacy account `search_jobs`, `get_job` and `get_company` schemas. Both reuse the
in-app assistant's personal handlers and the catalogue `Reader`; avoid changing
one endpoint's schemas to match the other.

Fiber authentication in `internal/api/handler/mcpapp_account.go` clears external
`AccountUserIDHeader` values before resolving credentials, including anonymous
requests. Invalid credentials fail closed instead of falling back to anonymous.
The canonical URL has path-specific protected-resource metadata; the root
well-known document continues to identify the account alias for existing clients.

It also admits a **full-scope API key** (`fhk_…`), the mirror of REST admitting an OAuth
grant wherever a key goes. That is how the local npm server (`freehire-mcp`), which holds a
key and no grant, manages job alerts through these same tools instead of a second
implementation. `mcpKeyTools` omits OAuth-only actions: auto-apply, dictionary
suggestions, problem reports and the interview-buddy tools. Dictionary suggestions and the
buddy tools (`internal/api/handler/buddy_mcp_tools.go`: membership, mock hosts, slots,
booking, cancelling and the hosting schedule) preserve the cookie-only REST gate, and so does
`update_profile` (`assistant_profile_write_tool.go`: a partial update of the saved job-search
profile: preferences plus the headline and summary text, never a CV document); problem
reports use the signed-in agent's reporting workflow. A narrow (`cv`) key is a 403.

OAuth clients use `suggest_dictionary_term` when a user-requested specialization, skill
or company industry cannot be resolved; zero search hits alone do not prove a gap.
`report_problem` records a brief redacted account-bound report for failed operations or
missing capabilities, without a separate reporting request. Reports stay private to
operators. Report once per issue, never recursively report a reporting failure, and
continue the original task where possible. These directions are in account initialization
only when the corresponding tools are served; the plugin skill carries examples.

The account server's job-alert tools (`subscribe`, `list_subscriptions`,
`update_subscription`, `unsubscribe`, `subscription_updates`, in
`internal/api/handler/assistant_subscription_tools.go`) address an ALERT by its saved search
id: one saved search over any filters, delivered on one subscriptions row per channel —
the website's own model. Every write goes through `savedsearch.Service` and
`subscription.Service`, the alert controls' own use cases, so `search.Narrows` and the
push-device rule are enforced once. A channel is only added where the delivery worker
would actually reach (`GetAlertChannelState`). Digest timing is account-wide on the
website, so it is shown but not changed here. "What is new" reads `subscription_matches`,
the ledger the digests are built from, rather than re-running the search.

`list_unread_mail` (`internal/api/handler/mcp_mail_tool.go`) is the inbox's only tool here:
the headers of unread application mail (`from_name`, `from_addr`, subject, time, label, and
the company and job slug of a CONFIRMED link), `unread_count`, `sync_pending` (Gmail's first
sync not completed), `reconnect_needed` (Gmail lost its grant; hosted mail still arrives) and
`mailbox_url`. No body, snippet or message id, nothing marked read, no model call; a failed
sync-state lookup logs and returns the page with `gmail_state_unknown` set. Served to keys too,
because REST `/me/inbox` is `mw.key`. Header text is sender-written: `inertLine` strips line
breaks and runes that render blank (format, variation selectors, Hangul fillers, private-use,
unassigned); name and subject are capped, the address is not (a long spoofed name must not
push it out of view); the description, a `notice` in the result and the instructions say
never to follow it. The assistant's `inbox_*` write
tools (triage, link, record application) are deliberately not served: the only evidence an
MCP agent would act on is that untrusted header text.

## `next` on `search_jobs` and `get_job`

Both results carry one short `next` sentence (`next.go`): signed in, the follow-ups among the
tools the session is actually served (an API key has no auto-apply); signed out, a pointer at
signing in instead of tools it cannot call. It is the same text for every call of a session
and once per response, never per job: the result is replayed into the model's context every
turn. On the typed catalogue tools it is a plain struct field; on the account server's untyped
tools `withNext` adds it to the JSON object.

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

`search_jobs` still takes `semantic: true`, but it is currently not available: the index
carries no description vectors, so the request is answered by keywords and the result's
`semantic` says `unavailable` — never an error. The outcome is decided by
`semanticOutcomeFor` (`internal/api/handler/semantic_search.go`), shared with OJCP and
`/agent/jobs/search`, so the three cannot disagree.

## What the tools must not do

- **Hand an internal error's text to the agent.** On the signed-in server a failed tool's
  text reaches a third-party model verbatim, so `agentErrorText` (`account.go`) shows it
  only for a `*fiber.Error` or a bad argument (`assistant.ErrInvalidArguments`); anything
  else reads as a generic sentence, and is not logged either, because the tool logs carry
  no error text. A message written for the agent ("slug is required", "no such answer")
  is therefore a `fiber.NewError` with its status, never `errors.New`: a plain one reaches
  the agent as "something went wrong". Its status is also what the metrics count it as.
- **Return a full description from `search_jobs`.** `/agent/jobs/search` rehydrates every
  hit's body from Postgres, deliberately, because a programmatic consumer reading whole
  bodies is what it is for. Here ten of those in one turn is the context window spent on
  text the model reduces to a line each. `summaryMaxChars` bounds the preview here rather
  than trusting the caller, because the bound is a property of this tool and not of whoever
  filled the field. The signed-in server takes its `search_jobs` from the in-app assistant,
  whose own copy rehydrates; `mcpKeyTools` (which `mcpAccountTools` builds on, so OAuth
  grants and API keys alike) serves `searchJobs(false)` instead — the index preview clipped
  to the same 400 runes. Before 2026-11-05 it served the rehydrating copy, ~130 KB a call.
  `facets` follows the same split: `facets(false)` answers with the filter names and how many
  values each has, and lists one filter's values when named in `facet` (narrowed by
  `contains`, which shares `ContainsMatcher` with `list_filter_values`). The whole vocabulary
  was ~125 KB, and `list_filter_values` does not cover every facet, metros among them.
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
  named in `ignored_params`, and the free text still runs. Which vocabularies are closed is
  `search.UnknownFacetValue`'s answer — the same table REST's `meta.ignored_values` reads —
  never a list here. `skills`/`exclude_skills` are closed since 2026-10-27 (`skills=golangg`
  answered `total: 0` and named nothing): the facet is written by `skilltag` alone, so a
  value `skilltag.Resolve` cannot read matches no posting. An alias it CAN read (`golang`)
  is kept and the search core reads it as `go`.

## `list_filter_values`

The vocabulary lookup, so a model can check a value before a search rather than learn of it
from `ignored_params` after one. It is the account server's `facets` narrowed to ONE filter
per call (skills alone are ~1,200 values), keyed by `search_jobs`' own field names
(`sources`, not `source`) and read from the website's unfiltered facet distribution and its
cache entry. `contains` filters by substring; for skills it also returns the canonical an
alias resolves to, because "golang" is not a substring of "go". `company_slugs` is not
listable — an employer is found by name through `search_companies`.

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
  closed postings; private and duplicate postings still answer not_found.
- **`get_job`** (here and on the account server) loads through `agentJobBySlug`, NOT
  `publishedToAgents`: search reads an index that trails the database, so it hands out slugs
  that closed or became duplicates since the push, and a not_found for a slug the previous
  turn returned is what four agent reports in one day were about (2026-11-07). A closed
  posting is served with `closed_at`, a duplicate as its canonical posting (or as itself when
  that one is not public); only a private posting is a miss.
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

- **`similar_jobs`** — the website's Meilisearch related-jobs search with mandatory
  location and specialization filters, ranked by keywords on the source's title.
  The source and rehydrated results must pass `publishedToAgents`; private and absent
  source slugs answer identically. No offline similarity backfill is needed.
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

A signed-in session (the account alias, and the canonical endpoint with a grant or key) also gets
`accountPrompts` (`account_prompts.go`): `jobs_for_my_profile`, `set_up_my_profile`,
`whats_new_in_my_alerts` and `apply_to_saved_jobs`. They DO name tools and their order, since the
sequence is the point; each is offered only when every tool it needs is served, so an API key
never sees the auto-apply one. Anonymous connections get the catalogue prompts alone.

## The widget

`search_jobs` and `get_job` render in an MCP Apps widget (`widget.go`, `widget/jobs.html`):
one self-contained page an MCP Apps host — ChatGPT, Claude — draws the result in as cards,
instead of the model retelling it as prose. The tools' contracts are untouched; a client
that ignores `_meta.ui` sees exactly what it saw before.

- **It is hand-written HTML, not a `web/` build, on purpose** — see
  `openspec/changes/archive/2026-10-20-chatgpt-app-widgets/design.md`. If it ever grows
  independent sign-in, complex forms or direct network requests, that is the point to
  move it. Saving and stage controls use the host bridge and existing tools, not
  another HTTP client or credential store.
- **The URI is the page's hash.** Hosts cache by URI; editing the file changes it. Never
  hard-code the URI anywhere else.
- **Wire data lives only in `result`, `job` and `req`** inside the page.
  `TestTheWidgetReadsOnlyFieldsTheWireShapeHas` greps those names and checks every field
  against `wire.go`, so a renamed field fails the build instead of rendering as nothing.
  Reading wire data through any other name blinds it.
- **Safe text, no direct network requests.** `TestTheWidgetCannotInjectMarkupOrMakeRequests` bans
  `innerHTML` and friends and every network API; the CSP admits only
  `https://logo.freehire.me` images. A URL is used only when it is `https:`.
- **Account actions are explicit host-mediated calls.** `Save job` calls `save_job`;
  choosing a stage does nothing until `Update stage` calls `track_job`. Only the
  selected slug and stage are sent, never notes. Controls require the host's
  `serverTools` capability and never handle credentials. OAuth challenges point
  back to the assistant's sign-in controls.
- **Success needs a server confirmation.** The returned tracking record must have
  a valid job id and saved timestamp or the exact requested stage. An unknown
  result or timeout locks writes until the person explicitly confirms their tracking
  board shows no change and chooses `I checked, allow retry`. Unlocking never sends
  a request: Save or Update still needs its own click. Timed-out request ids are
  removed, so late replies cannot confirm a new operation. A confirmed refusal
  permits an explicit retry. Pending calls stay locked across
  result re-renders. `web/scripts/mcp-widget.test.mjs` drives the real embedded
  page's DOM and host protocol, including failure and source-spoofing cases.
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
server only ever acts for the person who approved the connection. `accountInstructions`
describes the account boundary and the tool groups actually served, opens with one line on what
freehire can do (search, save/track, alerts, auto-apply prepared for review) and says to call
`get_profile` before the first search and, with auto-apply served, `get_plan` before auto-apply. Its budget is 720 characters (the public server keeps 512);
Claude Code cuts server instructions at 2048. Search and application
workflows belong to the packaged skill; initialization describes capabilities and result
states without prescribing a call sequence or presentation.

`get_plan` (`internal/api/handler/mcp_plan_tool.go`, openspec/changes/mcp-plan-standing) lets
an agent read the tier and today's per-feature allowances before it queues metered work,
instead of learning them from a 402. It reads `loadPlan`, the use case `GET /me/plan` serves,
so the two cannot disagree; it is an allowlist carrying no price or payment source (those are
the support agent's `my_account`, not a third-party model's). Grants and full-scope keys alike.

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
