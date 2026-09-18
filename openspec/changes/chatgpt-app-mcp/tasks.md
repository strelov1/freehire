## 1. Groundwork

- [x] 1.1 Register `internal/api/mcpapp` in `internal/platform/arch/layering/blocks.go` (the
      `api` block) and confirm the layering guard passes with the package still empty. A
      package in neither table fails the guard, so this comes first rather than last.
- [x] 1.2 Create the package with its `Reader` interface — the four tool answers, already
      projected — and nothing else. Prove the interface is satisfiable by a test fake before
      any handler implements it.

## 2. Wire shapes

- [x] 2.1 Define the search result shape the tools answer in: our fields, the index's
      truncated description, `source`, `official_job_url` and the freehire page URL. Assert
      a projected posting carries all three URLs/attribution fields.
- [x] 2.2 Define the job-detail shape: the same fields plus the full description and the
      stored apply form's required fields when one exists.
- [x] 2.3 Define the company shapes for `search_companies` and `get_company`, each carrying
      the company's freehire page URL and its open-posting count.
- [x] 2.4 Define the search input: the curated ~15 parameters named in the spec, each with a
      `jsonschema` description written for a model, not a developer. Every field optional
      except free text; a pointer where absent and false differ (work mode, visa).

## 3. Tool implementations (transport-free)

- [x] 3.1 `search_jobs`: map the input onto `url.Values` and hand it to
      `search.FilterFromValues`, the same path `ojcpHandlers.SearchJobs` takes. Assert the
      mapping for each published parameter — one test per parameter, because a filter that
      maps to the wrong key silently widens the answer.
- [x] 3.2 Carry unreadable parameters through into the result's ignored list. Test that an
      unknown filter widens and is named, rather than narrowing.
- [x] 3.3 Enforce the page maximum, and report the total separately from the returned count.
- [x] 3.4 `get_job`: one posting with its full body by public slug; an unknown slug yields
      the recoverable error result.
- [x] 3.5 `search_companies` and `get_company` against the existing company reads.
- [x] 3.6 Assert the visibility predicate is the shared one: a private posting, a
      non-canonical duplicate and a closed posting are each unreachable through every tool,
      and each answers identically to an unknown slug.

## 4. MCP transport

- [x] 4.1 Build the `mcp.Server` with a server name distinct from `freehire-ojcp` and
      instructions under 512 characters. Assert the length in a test — the limit is a
      number nobody re-checks by eye.
- [x] 4.2 Register the four tools with title, description, input schema, output schema and
      the three annotations. Write one test that walks every registered tool and requires
      `readOnlyHint: true`, `destructiveHint: false`, `openWorldHint: true` — it is the
      guard against a future write tool inheriting a read-only claim.
- [x] 4.3 Render failures as `CallToolResult` with `IsError` and a plain sentence, never as
      `*jsonrpc.Error`. Test both the absent case and the internal-failure case, and assert
      the second discloses no internal detail.
- [x] 4.4 Drive the whole server through the SDK's in-memory client in tests — `initialize`,
      `tools/list`, and one call per tool — the way `ojcpmcp_test.go` does.

## 5. Route

- [x] 5.1 Add `internal/api/handler/mcpapp.go`: implement `mcpapp.Reader` on the existing
      search handlers and mount `api.All("/mcp", limit, adaptor.HTTPHandler(...))` with the
      same `agentSearchLimiter` the OJCP routes use.
- [x] 5.2 Integration test: the route answers `initialize` over HTTP, and the OJCP route
      still answers with its own server name and error shape.
- [x] 5.3 `go vet -tags=integration ./...` and `go test -tags=integration ./internal/api/...`
      before pushing — `internal/api/handler` holds 78 build-tagged test files that plain
      `go test` never compiles.

## 6. Ship

- [x] 6.1 Open the PR, let CI go green, merge, deploy, and verify against production with a
      real `initialize` + `tools/list` against `https://freehire.me/api/v1/mcp`.
- [x] 6.2 Document the surface in `internal/api/mcpapp/AGENTS.md`: why a second MCP server
      exists, why its error shape differs from the OJCP one, and the annotation rule.
- [x] 6.3 Decided AGAINST listing the endpoint on the site: discovery for a ChatGPT app
      happens through OpenAI's directory, not through our own pages, and a listing nobody
      reads is the thing this task was written to avoid.

## 7. Directory submission (after the server is live)

- [ ] 7.1 Complete developer verification in the OpenAI Platform dashboard. A submission
      from an unverified account fails outright, so this gates everything below.
- [ ] 7.2 Connect the server in ChatGPT developer mode (Settings → Apps → Advanced →
      Developer mode → Create app), run Scan Tools, and exercise all four tools in a real
      chat.
- [ ] 7.3 Write the listing copy: name, short description, long description. State plainly
      that this is an aggregator that attributes every posting to its source. No promotional
      language in tool descriptions — it is a named rejection reason.
- [ ] 7.4 Produce the app icon and the screenshots. The screenshots must show the app
      running inside ChatGPT in developer mode, not the freehire site.
- [ ] 7.5 Write the test cases, and confirm each passes on ChatGPT web AND mobile. No
      credentials are needed, since this surface is anonymous.
- [ ] 7.6 Supply the support contact address and confirm `freehire.me/privacy` covers what
      the guidelines require: categories of data collected, purposes, recipients, retention,
      and user controls.
- [ ] 7.7 Submit, record the Case ID, and answer any clarification. Only one version may be
      under review at a time.
