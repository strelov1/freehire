# Design — jobstash-source

## Context

JobStash exposes `GET https://middleware.jobstash.xyz/jobs/list?page=&limit=`
(no auth). The response is `{page, count, total, data: [job]}`. Each job carries
the full posting inline (no per-posting detail call needed): `title`, `summary`,
`description`, `responsibilities[]`, `requirements[]`, `benefits[]`, `url`,
`access` (`public`/`protected`), `shortUUID`, `location`, `locationType`
(`ONSITE`/`REMOTE`/`HYBRID`/null), `timestamp` (epoch ms), and `organization`
(an object whose `name` is the employer). A probe confirmed ~3058 postings; a
public posting carries the downstream ATS link in `url`, while a **protected
posting's `url` is null** on this feed (~18% of postings — the gated apply link
is omitted). For a null `url` the adapter synthesizes the JobStash detail page
`jobstash.xyz/jobs/{shortUUID}/details` (what `/public/jobs` serves server-side),
so a protected vacancy keeps a working link rather than an empty URL.

This fits the existing single-stage adapter pattern (like Sber/AlfaBank: list
endpoint carries the body inline, walk pages to `total`, no detail fan-out).

## Goals / Non-goals

- **Goal:** one `jobstash` adapter yielding all postings across companies, kept
  filterable in the source facet.
- **Non-goal:** cross-source dedup against the direct ATS crawls (accepted
  overlap), enrichment changes, or any schema/migration change.

## Decisions

### Adapter shape (single-stage, paginated)

`Fetch` ignores `e.Board` (boardless) and loops `page=1..` with a fixed
`limit=200`, accumulating `data` until `page*limit >= total` (or `data` is
empty). Context cancellation is honoured between pages. The list body is
complete, so there is no `fetchDetails` fan-out.

### Field mapping (`sources.Job`)

| Job field    | Source                                                                 |
|--------------|------------------------------------------------------------------------|
| `Title`      | `title`                                                                 |
| `Company`    | `organization.name`                                                    |
| `URL`        | `url` when set (public = ATS link); else synthesized JobStash page from `shortUUID` (null on protected) |
| `ExternalID` | `shortUUID` (stable; also the id in the protected URL)                  |
| `Location`   | `location`                                                             |
| `WorkMode`   | `locationType` via the shared `workplaceTypeMode` (lowercases + maps)   |
| `Remote`     | `WorkMode == "remote"` OR `isRemote(Location)`                          |
| `Description`| compose `description` + responsibilities + requirements + benefits      |
| `PostedAt`   | `timestamp` (epoch ms) via `parseEpochMillis`                          |

`workplaceTypeMode` already lowercases and maps `remote`/`hybrid`/`onsite`, so
`REMOTE`/`HYBRID`/`ONSITE` map cleanly; null/unknown → `""`.

The description is assembled into sanitized HTML consistent with the other
adapters: the prose `description`, then `<h*>`/`<ul>` sections for
Responsibilities, Requirements, and Benefits when present. A posting missing all
of these still yields (empty body is acceptable, as elsewhere).

### `external_id` and dedup

The pipeline namespaces as `"<board>:<ExternalID>"`. With an empty board the
stored `external_id` is `:<shortUUID>` — consistent with every existing boardless
adapter (`ozon`, `uber`, …). `shortUUID` is unique across the whole JobStash
catalogue, so this is collision-free. Overlap with the direct ATS crawls is by
design (different `source`), and is accepted.

### `boardless` vs `aggregator` (the marker split)

`boardless` currently does double duty: "needs no board id" AND "single company,
so excluded from the source facet". JobStash breaks the second half. The surgical
fix keeps `boardless` meaning only "no board id" and introduces a second opt-in
marker:

```go
type aggregator interface{ aggregator() }
```

`FilterableProviders` changes from "exclude boardless" to "exclude boardless that
is NOT an aggregator":

```go
if bl, ok := src.(boardless); ok {
    if _, agg := src.(aggregator); !agg { continue } // single-company boardless: skip
}
```

`JobStash` and the already-merged `tecla` marketplace implement `aggregator()`
(both are `boardless()` multi-company feeds); `tecla` was wrongly excluded from
the facet before this marker existed, so it adopts it here. The single-company
boardless adapters are untouched and stay excluded. Board-based adapters never
matched the boardless branch, so they stay listed. Regenerating the web contract
also surfaces `globalpayments`, which had drifted out of the stale `SOURCE_VALUES`.

## Risks / Trade-offs

- **Undocumented endpoint.** `/jobs/list` is the site's own frontend API, not the
  smaller documented `/public/jobs` (278 postings). It is richer and 10× the
  coverage; the trade is a contract that could shift. Mitigation: the adapter
  decodes only the handful of fields it maps and tolerates missing fields, so a
  field addition is harmless and a removal degrades one field rather than failing
  the board.
- **Catalogue growth from dupes.** Accepted per the proposal; no mitigation in
  scope.

## Migration / Rollout

No schema or data migration. After merge, add the cron entry for
`sources/jobstash.yml` and regenerate the web contract (`make gen-contracts`) so
`jobstash` appears in the source facet. First ingest run populates the new
`source = "jobstash"` jobs; the normal enrichment/reindex pipeline picks them up.
