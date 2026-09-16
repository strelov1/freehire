## Context

OJCP is a draft v0.1 standard for agent-consumable job data, governed by a nine-seat
committee (Workday, Hiring.cafe, CrossCountry, aiApply, scale.jobs, Tink, LoopCV, Recruitics,
and the WebMCP co-creator). Its conformance bar for a provider is small — serve a manifest
at `/.well-known/ojcp.json`, implement `search_jobs`, return JSON, honour your own declared
rate limits — and everything else is SHOULD. Two providers have shipped; the registry holds
one entry.

What we already have maps onto it almost exactly. `/agent/jobs/search` is the same query as
the public search with full descriptions rehydrated from Postgres. `jobview.Job` is the
single public projection of a posting and already carries `skills`, `ghost` and
`auto_apply_available`. `apply_forms` (migration 0072), filled by `cmd/capture-apply-form`
for greenhouse/ashby/workable/lever/recruitee, holds exactly what OJCP's `ApplyPath` asks
for: the ATS provider and the form's required fields.

The repository has no MCP server of any kind today, and `cmd/server` is Fiber v2, not
`net/http`.

## Goals / Non-Goals

**Goals:**

- Be a conforming OJCP read provider: manifest plus `search_jobs`, `get_job_detail`,
  `get_employer_context`.
- Reach the agents that exist now. That means MCP, not only REST — our
  `.well-known/ai-plugin.json` is a format nothing dials any more.
- Publish `ApplyPath.required_fields` and an honest `supports_agent_submission`, which is
  the part of the standard our data uniquely answers.
- Keep the projection testable without a database, a network, or Fiber.

**Non-Goals:**

- Agent-initiated application. `begin_application`, `submit_application`,
  `check_application_status` and the API-key scope they need belong to `ojcp-provider-apply`.
- Manifest signing, the `verified` registry trust tier, and identity verifiers. All are
  SHOULD-level and none can be exercised until we are listed at all.
- WebMCP (the in-page browser-agent binding). Our extension is a separate surface with its
  own protocol; folding it in here would double the change for no adoption.
- Candidate context. Reads are anonymous; we accept no PII on this surface in this change.

## Decisions

### One projection package, two transport adapters

`internal/api/ojcp` holds the projection and nothing else: `jobview.Job → JobPosting`,
stored apply form → `ApplyPath`, ghost verdict → `agent_notes`, search result →
`SearchJobsResponse`. It takes already-loaded domain values and returns structs. No pgx, no
Fiber, no I/O.

`internal/api/handler/ojcp_*.go` (REST) and `internal/api/ojcpmcp` (MCP) both load data the
way the existing handlers do and then call the same projection functions.

*Why:* the spec's own conformance language treats the two transports as interchangeable
renderings of one answer, and the cheapest way to guarantee they never drift is to give them
no separate code to drift in. It also means the bulk of the change is unit-testable against
OJCP's published JSON Schemas with no fixtures beyond a domain value.

*Alternative rejected:* implement MCP only and let the conformance suite talk to it. The
suite exercises REST, and REST is what a plain HTTP client (including our own integration
tests) can hit without a protocol library.

### MCP via the official Go SDK, mounted through Fiber's adaptor

`github.com/modelcontextprotocol/go-sdk` (stable 1.x) provides the streamable-HTTP server
handler; Fiber's `adaptor.HTTPHandler` mounts a `net/http` handler on a Fiber route.

*Why:* it is the reference implementation of the protocol we are adopting, and a
hand-rolled JSON-RPC layer would be ours to keep current against a spec that is still moving.
The adaptor is Fiber's documented seam for exactly this, so no second server or port.

*Alternative rejected:* run the MCP server as its own binary on its own port. That adds a
deploy unit, a second TLS surface, and an unbounded gap between what the API knows and what
the MCP server knows.

### `ojcp_id` is the public slug

*Why:* it is already the stable public identifier a posting's page is served under, so an
`ojcp_id` an agent stores keeps resolving. A numeric row id would leak catalogue size and
break on any future re-keying.

### `agent_notes` carries the ghost verdict

OJCP has no field for "this posting may not be a real opening". `agent_notes` is the spec's
free-text channel to an agent, so the verdict goes there for now.

*Why not wait for a spec field:* we would be publishing nothing meanwhile. Shipping it in
`agent_notes` first also gives the follow-on RFC a working implementation to point at, which
is a stronger proposal than a field request.

### `supports_agent_submission` is derived, never declared

It is true only for postings whose source is in auto-apply's supported set AND whose ATS is
not one we park on. Lever reports `false` — its challenge is a coin toss in practice, and
telling an agent otherwise would send it down a path that fails eight times in nine.

*Why:* this flag is the one an agent plans around. An optimistic value here is worse than
no value.

### Visibility is the existing predicate, called — not re-derived

The OJCP handlers go through the same search core as `/jobs/search` rather than assembling
their own query.

*Why:* a second predicate is a second place for a private posting to leak, which has already
happened once on this codebase (private postings reaching `/similar`). Sharing the core means
a future visibility rule lands on both surfaces at once.

### Manifest is a served document, not a static file

Rendered from the deployment's own configuration — rate limits, base URL, the registered
tool list — rather than hand-maintained in `web/static/`.

*Why:* the spec makes declared `rate_limits` binding and `tools` a claim about what answers.
A hand-edited file drifts from the router silently; a rendered one can be asserted against
the router in a test.

## Risks / Trade-offs

- **The standard may not get traction** (two providers, one registry entry) → The read half
  is small and self-contained, and rollback is removing one route: an agent that cannot fetch
  the manifest simply does not treat us as a provider. Nothing else depends on it.
- **First MCP server in the repo; new dependency** → Confined to `internal/api/ojcpmcp`,
  which holds no logic. If the SDK proves unsuitable the projection and REST surface survive
  untouched.
- **Declared rate limits become a conformance obligation** → Derive them from the same
  configuration the limiter reads, and assert in a test that the manifest cannot declare a
  figure the deployment does not enforce.
- **Publishing the catalogue through a new machine channel** → It is the same predicate and
  the same data `/agent/jobs/search` and `openapi.yaml` already publish, and
  `official_job_url` (added to the OJCP schema by RFC 0002) carries source attribution with
  every record — better than the current agent surface does.
- **Spec is a living draft; v0.2 may move fields** → The projection is one package; a schema
  change is a diff there plus a re-run of schema-validated unit tests. `ojcp_version` is in
  every response, so an agent can tell what it received.

## Migration Plan

1. Ship the projection and REST surface; the manifest is not served yet, so nothing
   discovers us.
2. Add the MCP transport.
3. Serve the manifest — this is the switch that makes us a provider.
4. Run OJCP's conformance suite against production and fix what it reports.
5. Only then: open the `ADOPTERS.md` PR (tier: Implementing) and the registry entry.

Rollback at any point after step 3 is un-serving the manifest.

## Open Questions

- Does the conformance suite require a reachable public host, or can it run against a
  staging origin? Decided at step 4, and it only affects when we run it, not what we build.
