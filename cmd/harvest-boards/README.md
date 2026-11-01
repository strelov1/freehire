# Board harvesting

The normal `harvest-boards` command discovers and validates new ATS boards. It reports by default and inserts pending boards only with `--apply`.

## Feed audit

`-audit-feeds` compares a provider's documented machine feed against the current source adapter. It currently supports **Scalis** and **Teamtailor**. It is read-only and rejects `--apply`.

```sh
# Audit existing pending/active boards (requires DATABASE_URL).
go run ./cmd/harvest-boards -audit-feeds -audit-limit 20 teamtailor > teamtailor-audit.jsonl

# Audit selected boards, including boards already known to the catalog.
# A seed-file audit does not connect to the database.
go run ./cmd/harvest-boards -audit-feeds scalis boards.json > scalis-audit.jsonl
```

The seed uses the usual JSON array of board names or `{ "board": "boldbusiness", "company": "Bold Business" }` objects. Teamtailor accepts a career-site host or a bare vendor subdomain. Explicit seeds select the audit targets; seed `expect_id` is not an acceptance gate in this read-only mode. The result does not authorize adding a board.

Without a seed, all current boards are candidates, including existing entries that the normal harvest skips. Targets are deduplicated, sorted and bounded by `-audit-limit` (default 20; positive). The log reports the selected and audited counts. Use a seed to choose a different subset. Boards run sequentially, each with a deadline covering the feed and baseline together (`-audit-timeout`, default `2m`; positive). For large boards that need one detail request per posting, increase this budget, for example `-audit-timeout 15m`, while retaining the request pace. Requests use the provider's usual egress, or `-rotating`, and share a rate limiter. The audit defaults to one request per second; a positive `-pace` overrides it. `-workers` affects normal harvesting, not this sequential audit.

### Report

Stdout contains one JSON object per board; diagnostics go to stderr. Records identify the provider, board, check time, feed URL, actual supported schema, inventory counts, request counts and status:

- `compared`: both reads completed; `comparison` contains observed differences. This is not a migration recommendation.
- `feed_unavailable`: the initial endpoint returned 404/410. This does not mean the board is empty or dead.
- `feed_failed`: fetching, schema validation, identity validation or pagination failed. Any reported feed counts are partial evidence.
- `baseline_failed`: the feed was read but the current adapter could not provide a verified baseline. Feed evidence remains available, with no comparison claim.

Failures produce a nonzero exit status after emitting available reports. Unavailable feeds and measured differences are valid audit outcomes. A broken output stream stops the run immediately.

`feed_requests` and `adapter_requests` count calls into the shared client, excluding retries and redirects inside that client. They measure this full comparison, not the steady-state hydrating crawl cost.

`comparison` reports missing/extra native IDs, changed titles, normalized body differences, field presence counts and fields lost on matched IDs. Bodies are compared after HTML/entity and whitespace normalization; full bodies and recruiter contact fields are not emitted. Field presence is not value equivalence: for example, a populated country or work-mode field still needs a mapping check. Adapter fields describe what the adapter yields before pipeline derivation, so a country embedded in its location text is not counted as an explicit country field. Inventory can change between reads; discrepancies need investigation.

### Provider details

- **Scalis:** `/api/job-listings/feed`, custom `jobs/job` XML. Several location rows can share one vacancy. The audit groups them by the native ID in the canonical vacancy URL, unions field-presence observations and rejects conflicting bodies/titles. It does not use the location-expanded `reference` as job identity. Publication dates were absent from the researched feed; the comparison exposes that loss.
- **Teamtailor:** `/jobs.rss`, RSS with location metadata. The audit retains numeric IDs from posting URLs rather than the RSS GUID. It requests 500 items per page, requires a short page to establish termination, rejects duplicate IDs and fails after 20 full pages. A missing later page is a failed walk, not an unavailable feed. Some custom sites have no RSS endpoint.

The baseline runs the real adapter with the same paced transport. Any observed transport failure makes the baseline unverified, even when an adapter treats a detail request as best-effort and otherwise returns successfully. The audit never changes ingestion, catalog rows, published identities or the adapter selected for a board.
