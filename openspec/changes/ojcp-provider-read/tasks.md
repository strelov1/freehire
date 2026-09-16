## 1. Groundwork

- [x] 1.1 Vendor OJCP's published JSON Schemas (manifest, job-posting, and the three read
      tools' inputs and responses) into `internal/api/ojcp/testdata/schemas/`, with a short
      README noting the upstream commit they were taken from — they are the oracle every
      projection test validates against, so their provenance must be recorded.
- [x] 1.2 Add a schema-validation test helper that loads those schemas offline and asserts a
      projected value against one. Prove it by feeding it a value that violates a `required`
      field and watching it fail. Turn on `format` assertion (draft 2020-12 leaves it off, so
      a malformed `datePosted` would otherwise pass) and pin the offline guarantee with a
      loader that refuses every URL rather than relying on the library's default.
- [x] 1.3 Register `internal/api/ojcp` in `internal/platform/arch/layering/blocks.go` (the
      `api` block) and confirm the layering guard passes. `internal/api/ojcpmcp` is
      registered in task 5.1, when the package first exists: the guard fails on a table entry
      naming a package that is not on disk.

## 2. JobPosting projection

- [x] 2.1 Project `jobview.Job` into an OJCP `JobPosting`: `ojcp_id` from the public slug,
      `url` from our page, `official_job_url` from the source URL, title, employer,
      `datePosted`, `validThrough`, `employmentType`, `experienceLevel`, `remote_policy`,
      `baseSalary`, `skills_required`. `datePosted` is REQUIRED by the schema, so it falls
      back to `created_at`; `remote_policy` and the salary period are CLOSED enums, so an
      untranslatable value omits the field rather than failing the posting. Seniority
      translates only the exact matches (`middle`→`mid`, `c_level`→`executive`) and emits
      ours verbatim otherwise, rather than collapsing four levels into two.
      (`jobLocation` is deferred to 2.6 — it needs the geography facets, not the flat string.)
- [x] 2.2 Project the ghost verdict into `agent_notes`, omitting the field entirely when the
      posting carries no verdict — including at level `none`, where a "no concerns" note
      would assert a check we never ran. Wording stays hedged; a test forbids the
      accusatory vocabulary outright.
- [x] 2.3 Project a stored apply form into an `ApplyPath` carrying `type`, `ats_provider` and
      `required_fields`. A demographic question is excluded even when the platform marks it
      required — OJCP carries those in its own eeo-data schema, and listing one here would
      say that answering it decides the application. An unlabelled field falls back to its
      opaque platform identifier rather than being dropped.
- [x] 2.4 Derive `supports_agent_submission` from what THIS DEPLOYMENT can submit to, handed
      in via `Projector.Submittable` rather than baked into a package constant. Today that is
      Greenhouse alone (`fillProviders`); Ashby and Workable join it only where the
      cloud-agent fallback is enabled and funded, and Lever never does — it has a working
      fill path that an invisible hCaptcha defeats about seven attempts in eight.
- [x] 2.5 Project a posting with no captured form into a single `external_redirect` path.
      There is always at least one path: omitting the field would read as "there is no way
      to apply", which is never true.
- [x] 2.6 Project the geography facets into `jobLocation`, plus the posting's own
      `description`. The schema shapes `jobLocation` as ONE schema.org `Place`, so a facet
      naming several values is left out rather than resolved by taking the first — the list
      order carries no precedence, and an agent filtering on an arbitrary choice would drop
      the posting everywhere else the employer accepts. The two facets are judged
      independently, so one country with two cities still states the country. Our `Regions`
      facet is NEVER written to `addressRegion`: ours are macro-regions ("europe"), that one
      is a state or province. **Multi-country reach is not expressible in OJCP v0.1** — the
      clearest schema gap found so far, and a candidate for the same RFC as the
      posting-reality field.

## 3. Tool implementations (transport-free)

- [x] 3.1 `search_jobs`: `SearchInput.QueryValues` maps OJCP's input onto this catalogue's
      OWN query vocabulary (the same `url.Values` the public endpoints build a filter from),
      so nothing in the search core changes. `SearchJobsResponse.Finalize` derives `returned`
      and stamps the version; an empty page serialises as `[]`, never null. The page is
      capped at the schema's own maximum of 50.
- [x] 3.2 `QueryValues` returns the input fields it could not honour, and the response
      carries them as an `ignored_params` extension — the standard's response has nowhere
      for it and its extensibility rule permits the addition. `location.state` and
      `location.radius_miles` are reported (we hold no province facet and no coordinates),
      as is an `experience_level` of `director`, which no level of ours means.
- [x] 3.3 `get_job_detail`: `JobDetailResponse` carries one projected posting. The
      not-found path is the error envelope, rendered per transport in groups 4 and 5.
- [x] 3.4 `get_employer_context`: `EmployerContextFrom` projects a company, keyed by the
      same slug a posting publishes as `ojcp_employer_id`, and carries `open_roles_count` —
      the one figure an agent cannot derive from a single posting.
- [x] 3.5 Assert the visibility predicate is the shared one — a test that a private posting
      and a suppressed duplicate are unreachable through every tool.

## 4. REST transport

- [x] 4.1 Add `/ojcp/v1/*` routes wiring each tool to its handler, loading data the way the
      existing handlers do.
- [x] 4.2 Render errors as the OJCP error envelope with the matching HTTP status.
- [x] 4.3 Ignore unrecognised input fields rather than rejecting the call.
- [x] 4.4 Handler tests covering all three tools end to end, plus the two failure paths an
      agent must tell apart (unknown id, search unavailable). They run under the ordinary
      `go test` rather than the `integration` tag: the transport's dependencies are both
      ports (a searcher and a store), so a real database would prove nothing more.

## 5. MCP transport

- [x] 5.1 Add the official Go MCP SDK to `go.mod`, stand up a server in
      `internal/api/ojcpmcp` registering the three tools against the same implementations,
      and register that package in `internal/platform/arch/layering/blocks.go`.
- [x] 5.2 Mount it into Fiber via `adaptor.HTTPHandler` at the path the manifest declares.
- [x] 5.3 Render errors as JSON-RPC errors carrying the OJCP envelope in `data`.
- [x] 5.4 Test that the same call over both transports yields an identical projected payload.

## 6. Manifest

- [x] 6.1 Render the manifest from deployment configuration — provider block, `tools`,
      `feed_endpoints`, `mcp_endpoint`, `auth`, `rate_limits` — and serve it at
      `/.well-known/ojcp.json` with `Content-Type: application/json`.
- [x] 6.2 Test that every name in `tools` resolves to a registered handler on both transports,
      so the manifest cannot advertise a tool we do not answer.
- [x] 6.3 Test that the declared `rate_limits` are read from the same configuration the
      limiter enforces, so the two cannot disagree.
- [x] 6.4 Validate the rendered manifest against the vendored manifest schema.

## 7. Documentation and rollout

- [x] 7.1 Document the REST tools in `web/static/openapi.yaml` (the `artifacts` CI job
      validates it).
- [x] 7.2 Write `internal/api/ojcp/AGENTS.md`: what the package is, the pure-projection rule,
      why `supports_agent_submission` is derived, and where the schemas came from.
- [x] 7.3 Add the OJCP surface to the root `CLAUDE.md` module table.
- [ ] 7.4 After deploy: run OJCP's conformance suite against the live origin and record what
      it reports.
- [ ] 7.5 After a clean conformance run: open the `ADOPTERS.md` PR (tier: Implementing) and
      the provider registry entry.
