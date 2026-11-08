## Context

`internal/sources` is a registry of `Source` adapters keyed by provider; each adapter
fetches one board's postings over the shared `HTTPClient` and yields the normalized
`Job` shape. Adding a platform is a new file + one line in `sources.All` (`source.go`),
with no pipeline change. Six adapters exist today. This change adds six more, each
verified live against the platform's public feed before being specced.

Live probe results (captured June 2026, used as the source of truth for adapter design
and as the basis for test fixtures):

| Provider | Endpoint (verified) | Body in list? | Shape |
|---|---|---|---|
| personio | `https://{board}.jobs.personio.com/xml` | yes | XML `<workzag-jobs><position>` w/ `id, name, office, department, recruitingCategory, jobDescriptions/jobDescription/value` (CDATA HTML) |
| breezy | `https://{board}.breezy.hr/json` | yes | JSON array of postings, body inline |
| pinpoint | `https://{board}.pinpointhq.com/postings.json` | yes | JSON `{data:[{id, …, body html}]}` |
| rippling | `https://api.rippling.com/platform/api/ats/v1/board/{board}/jobs` | no | JSON `[{uuid, name, department{label}, url, workLocation{label}}]` → detail per `uuid` |
| bamboohr | `https://{board}.bamboohr.com/careers/list` | no | JSON `{result:[{id, jobOpeningName, departmentLabel, …}]}` → `/careers/{id}/detail` |
| join.com | GraphQL `candidate-api` / Next.js `__NEXT_DATA__` on `join.com/companies/{board}` | yes | postings embedded; exact request to be captured first |

## Goals / Non-Goals

**Goals:**

- Register six new adapters that ingest real postings, each yielding a sanitized-HTML
  description consistent with existing adapters.
- Reuse existing seams only: `Source` interface, `reg`/`All`, `HTTPClient`,
  `sanitizeHTML`, the shared time/location helpers in `source.go`.
- Seed a few live-validated boards per provider into `sources.yml`.

**Non-Goals:**

- No pipeline, write-path, or `cmd/ingest` change.
- No mass slug harvest / live-probe tooling, and no `sources.yml` → `sources/*.yml`
  split (separate, provider-agnostic changes).
- No `gem`, `jazzhr`, `recruiterbox` adapters — no clean public feed found.

## Decisions

**1. Three single-request adapters (personio, breezy, pinpoint) follow `recruitee.go`.**
The list carries the full body, so one `GetJSON`/`GetXML` call per board suffices. Each
maps platform fields into `Job` and runs the body HTML through `sanitizeHTML`.

- *personio* parses XML. Add a small `GetXML` helper to `HTTPClient` (mirrors `GetJSON`,
  stdlib `encoding/xml`) rather than hand-rolling per adapter. Location = `office`;
  description = concatenated `jobDescriptions/jobDescription/value` CDATA. Remote inferred
  via the shared `isRemote(location)` heuristic (the feed has no remote flag).
- *breezy* / *pinpoint* parse JSON; body field is inline HTML.

**2. Two detail-fetch adapters (rippling, bamboohr) follow `smartrecruiters.go`.**
The list omits the description, so the adapter paginates/lists then fetches each posting's
detail. Reuse smartrecruiters' bounded-concurrency pattern so a single board cannot issue
unbounded concurrent detail requests (the spec requires this bound).

- *rippling* `ExternalID = uuid`, detail at `…/board/{board}/jobs/{uuid}`.
- *bamboohr* `ExternalID = id`, detail at `…/careers/{id}/detail`; location built from
  `detail.result.jobOpening.location` via `joinNonEmpty(city, state, country)`.

**3. join.com is discovery-then-build.** Its postings are not a plain REST endpoint.
Task 1 captures the exact request (prefer the GraphQL `candidate-api` operation; fall back
to parsing `__NEXT_DATA__`) and freezes a fixture. Only then is the adapter written against
that fixture. If neither yields a stable, ToS-clean request, join.com is dropped from this
change (documented) rather than shipping a brittle scraper.

**4. Provider key = stored `source`.** Keys are `personio`, `breezy`, `pinpoint`,
`rippling`, `bamboohr`, `join.com` — matching `CompanyEntry.Provider` and the persisted
`jobs.source`, consistent with existing adapters.

**5. Tests are table-driven against captured fixtures**, mirroring the existing
`*_test.go` files: a stubbed `HTTPClient` returns the recorded response; the test asserts
the mapped `Job` fields and that the description is decoded + sanitized. No network in tests.

## Risks / Trade-offs

- **join.com feed instability / ToS** → gate it behind a capture task; drop it from the
  change if no clean request exists, rather than ship a fragile adapter.
- **Detail-fetch fan-out (rippling, bamboohr)** → reuse smartrecruiters' existing
  concurrency bound; do not introduce a new unbounded path.
- **Personio `.com` vs `.de` host** → some accounts use `.de`. Seed boards are validated
  against `.com` (the probe confirmed `.com` resolves for sampled boards); host selection
  per board is an open question below, kept out of the adapter until a seed board needs it.
- **Seed-board staleness** → boards with zero open postings yield zero jobs without error
  (covered by a spec scenario), so a quiet board never breaks a run.

## Open Questions

- Does any seeded personio board require the `.de` host? If so, allow the host to come from
  the board value or a per-entry hint; otherwise hard-code `.com` and revisit when needed.
- join.com: GraphQL operation vs `__NEXT_DATA__` — resolved by the Task 1 capture.
