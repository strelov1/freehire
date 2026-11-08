## Why

The ingest pipeline supports six ATS platforms (greenhouse, lever, ashby, workable,
recruitee, smartrecruiters), but a live audit of open ATS company datasets
(`kalil0321/ats-scrapers`, MIT) shows the reachable long tail is far larger. Several
more platforms expose a public, no-auth posting feed and together cover tens of
thousands of additional companies we cannot ingest today. Adding adapters for them is
the highest-leverage way to widen catalogue coverage without changing the pipeline.

## What Changes

- Register four new `Source` adapters, each verified live against the platform's public
  feed, so boards on these platforms can be listed in `sources.yml`:
  - **personio** — public XML feed (`{board}.jobs.personio.com/xml`); description inline, single request (adds a `GetXML` transport to the shared HTTP client).
  - **pinpoint** — public JSON (`{board}.pinpointhq.com/postings.json`); description inline across HTML sections, single request.
  - **rippling** — public JSON (`api.rippling.com/platform/api/ats/v1/board/{board}/jobs`); list lacks description, per-posting detail fetch (role body; company boilerplate excluded).
  - **bamboohr** — public JSON (`{board}.bamboohr.com/careers/list` → `/careers/{id}/detail`); list carries the remote flag, detail carries the description.
- Extract the bounded per-posting detail fan-out (previously inline in smartrecruiters)
  into a shared `fetchDetails[P]` helper; converge smartrecruiters, rippling, and bamboohr
  onto it.
- Seed a small set of live-validated boards per new provider into `sources.yml` so each
  adapter ingests real postings.

## Capabilities

### New Capabilities
<!-- none — this extends the existing source-ingest capability -->

### Modified Capabilities

- `source-ingest`: add a requirement registering `personio`, `pinpoint`, `rippling`, and
  `bamboohr` as providers, each yielding the normalized job shape with a sanitized-HTML
  description, consistent with the existing adapters (single-request where the list carries
  the body; bounded per-posting detail fetch where it does not).

## Impact

- **Code**: new `internal/sources/{personio,pinpoint,rippling,bamboohr}.go` + table-driven
  `_test.go` per adapter; a `GetXML` method on `HTTPClient`; a shared `fetchDetails[P]`
  helper in `source.go`; one registration line each in `sources.All`; seed entries in
  `sources.yml`.
- **Pipeline**: none — adapters slot into the existing registry/`Source` interface; no
  change to `pipeline`, the write path, or `cmd/ingest`.
- **Dependencies**: personio XML parsing uses the stdlib `encoding/xml`; no new
  third-party dependency.
- **Out of scope (seams), reclassified after live probing**:
  - **join.com** (~23k companies) — extractable but via a GraphQL `candidate-api` /
    Next.js `__NEXT_DATA__`, not a plain feed; its own change (`add-joincom-source`) with a
    captured request and a ToS check.
  - **breezy** — its `/json` list omits the description; the body is in a `JobPosting`
    JSON-LD block on each posting's HTML page. Belongs to a future "open-web JSON-LD source"
    change alongside `gem`, `jazzhr`, `recruiterbox` (all client-rendered / no clean feed).
  - Mass slug harvest + live-probe tooling and splitting `sources.yml` into per-provider
    files remain separate, provider-agnostic changes.
