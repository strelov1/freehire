## Why

AI agents have begun searching for work on a candidate's behalf, and the catalogue they
reach for was built for browsers. OJCP (Open Job Context Protocol, draft v0.1) is the
first standard that answers this: a manifest at `/.well-known/ojcp.json` plus a small set
of tools an agent can call. Its steering committee holds Workday, Hiring.cafe, LoopCV,
aiApply and the WebMCP co-creator, and exactly **two** providers have shipped an
implementation — so the cost of being early is a few days and the reward is being one of
the first catalogues an agent can actually read.

We are unusually well placed to be third. The projection an agent needs already exists
(`/agent/jobs/search`, `jobview.Job`), and the part of the standard nobody else can fill —
`ApplyPath.required_fields` / `ats_provider` / `supports_agent_submission`, i.e. "what will
this employer's form ask, and can it be answered without a human" — is already sitting in
our `apply_forms` table, captured by `cmd/capture-apply-form`.

This change covers the READ half only. Agent-initiated application (`begin_application`,
`submit_application`, `check_application_status`) is a separate change,
`ojcp-provider-apply`: it touches the auto-apply queue, whose first real employer
submission landed on 2026-09-16, and holding the safe half hostage to the risky half would
delay both.

## What Changes

- Serve an OJCP Job Manifest at `/.well-known/ojcp.json` declaring the provider, the tools
  we implement, our REST feed endpoints, our MCP endpoint, and the rate limits we actually
  enforce.
- Add `internal/api/ojcp`, a pure projection package that translates our domain wire shapes
  into OJCP's: `jobview.Job` → `JobPosting`, the stored apply form → `ApplyPath`, the ghost
  verdict → `agent_notes`. It reads no database and knows nothing about Fiber.
- Expose the three read tools — `search_jobs`, `get_job_detail`, `get_employer_context` —
  over **two transports** that share one implementation:
  - REST under `/ojcp/v1/*` (what the conformance suite exercises),
  - MCP at `/ojcp/mcp` (what ChatGPT and Claude actually connect to today).
- Publish `official_job_url` beside `url` on every posting, so the source employer's own
  link travels with the record and source attribution survives the new channel.
- Register both new packages in `internal/platform/arch/layering/blocks.go`; a package in
  neither table fails the layering guard.

No existing endpoint changes shape, and no existing caller is affected. The visibility
predicate is the one the public search already uses — open, canonical, non-private — so
this channel exposes nothing the catalogue does not already publish through
`/agent/jobs/search` and `openapi.yaml`.

## Capabilities

### New Capabilities

- `ojcp-provider`: freehire as a conforming OJCP read provider — the manifest, the three
  read tools, the projection from our job/company/apply-form shapes into OJCP's schemas,
  and the rule that both transports answer identically.

### Modified Capabilities

None. Existing endpoints, their shapes, and their auth are untouched; this change adds a
parallel surface over the same reads.

## Impact

- **New code:** `internal/api/ojcp` (projection), `internal/api/ojcpmcp` (MCP transport),
  `internal/api/handler/ojcp_*.go` (REST routes), `web/static/.well-known/ojcp.json`.
- **New dependency:** the official Go MCP SDK, mounted into Fiber via `adaptor.HTTPHandler`.
  First MCP server in this repository.
- **Touched:** `internal/platform/arch/layering/blocks.go` (two new package entries),
  `cmd/server` route registration, `web/static/openapi.yaml` (document the REST tools).
- **Operational:** the manifest's `rate_limits` must match what nginx enforces — declaring
  a limit we do not honour breaks a MUST in the spec. No migration, no new worker, no
  reindex.
- **External:** once live, we add ourselves to OJCP's `ADOPTERS.md` (tier: Implementing)
  and open a registry entry. Rollback is deleting the manifest: an agent that cannot fetch
  `/.well-known/ojcp.json` simply does not treat us as a provider.
