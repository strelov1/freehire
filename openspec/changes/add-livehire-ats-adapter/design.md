## Context

LiveHire (www.livehire.com) powers branded career sites at `/careers/<segment>` (e.g.
`/careers/flybuys`). The HTML pages sit behind a Cloudflare challenge, which is why the platform
first read as "no public API" during triage. The SPA's own backing API does not:

- `POST /careers-api/talent-job-search` with body
  `{"keywords":null,"categories":[],"segmentCodes":["<segment>"],"workTypes":[],"flexibleWorkOptions":[],"page":1,"pageSize":100}`
  → `{"jobs":[...],"totalCount":N,"hasMoreResults":bool}`. Verified live: `flybuys` lists 6
  postings; a body without `segmentCodes` answers 500; a non-existent segment answers
  `totalCount: 0`.
- `GET /careers-api/job/<segment>/<urlCode>/<advertUrlCode>` → full posting detail:
  `descriptionHtml` (the body), `workType` enum (`FullTimePermanent`, …), `flexibleWorkOption`
  (`Hybrid`, `Remote`, …), `physicalLocation` + `locationCountryIsoCode`, `minSalary`/`maxSalary`/
  `currencyIsoCode`, `postedFrom`/`applicationsCloseAt`, `applicationsClosed`/`closedAt`.

Both verified to answer 200 to a plain `curl` from a datacenter IP — no challenge, no token, no
key. The anonymous bearer-token endpoints the SPA fetches before searching are NOT required for
these two calls.

One behavioural quirk verified live: the listing endpoint ignores the requested `page` and
re-serves the whole result set on every call (page 2 at pageSize 2 returned all 6 postings
again). The adapter therefore dedups across pages by external id and stops on `hasMoreResults`
or an unchanged set — the same defence `crawlPagedLinks` applies to sources that re-serve rows.

Existing conventions this design follows (see `internal/ingest/sources/AGENTS.md`,
`internal/ingest/atsboard/AGENTS.md`): the `Job` struct and `HydratingSource` interface
(`internal/ingest/sources/source.go`), the board-catalog `(provider, board)` model, and the
keyed-segment recognizer mode.

## Goals / Non-Goals

**Goals:**
- Crawl any LiveHire-hosted career site (one board = one tenant segment) into the catalogue with
  full posting detail, following the existing hydrating-adapter shape (getro.go).
- Recognize LiveHire board/posting URLs for board-contribution intake without misfiring on the
  rest of the vendor's shared product host.

**Non-Goals:**
- Discovering LiveHire tenants automatically (no public tenant directory exists; boards are
  added one at a time via `board_submissions`/`cmd/add-board`).
- Submitting applications through LiveHire (out of scope; the apply flow lives behind the
  challenge, and auto-apply is a separate subsystem).
- Modeling the `keyPoint*`/video fields the detail carries — freehire's `Job` shape has no place
  for them and they are rarely populated.

## Decisions

**Adapter shape: `HydratingSource`, modelled on `getro.go`.** The listing carries rich
structured facets but no body; the body exists only on the per-posting detail call. That is
exactly getro's two-step shape (`FetchNew` lists, then `fetchDetails` hydrates the unseen ids
under the shared worker pool, marking seen postings `SeenRefresh`). LiveHire follows it rather
than inventing a second hydration pattern. A `Fetch` list-only fallback is kept for callers that
do not drive hydration, as getro does.

**Board id = the career-site segment (`flybuys`).** It is the only stable, human-assignable id:
it is the path segment of the career site AND the `segmentCode` every API call carries. The
detail call additionally needs `urlCode` + `advertUrlCode`, which come from the listing — the
external id is `<segment>_<urlCode>_<advertUrlCode>`, the same three parts the platform's own
canonical URL and internal id string carry.

**Recognizer: `modeKeyedSegment` with key `careers` and a lower-case-slug board pattern.**
The board follows a fixed key segment (`/careers/<segment>…`) and has an enforceable shape
(`^[a-z0-9]+(?:-[a-z0-9]+)*$`), which is exactly what `modeKeyedSegment` exists for (cf.
Workstream's 8-hex). The pattern declines the vendor's other paths (`/careers-api/...` fails the
fixed-key match's shape check since `api` would match but the FULL first segment after key is
what's checked — `/careers-api` on the host is not under `/careers/`; underscore or mixed-case
segments fail the pattern) and every non-`/careers` path on the shared host (`/pricing`,
`/jobs`, `/talent/community/...`) is declined by the key mismatch. One entry covers the board
listing, `/jobs`, and the three-segment posting URL, collapsing them all to
`https://www.livehire.com/careers/<segment>`.

**Employment type: explicit enum map, unknown → unset.** `workType` is a compound enum
(`FullTimePermanent`, `PartTimeCasual`, `FixedTerm`, …). Only the spellings the platform is
known to use are mapped onto `vocab.EmploymentTypeValues`; an unrecognized value leaves the
field empty for the pipeline's dictionaries rather than guessing. A compile-time guard pins the
mapped values inside the vocabulary.

**Salary only from structured detail fields.** `minSalary`/`maxSalary`/`currencyIsoCode` are
read only when both bounds are present and sane (`min > 0`, `max >= min`); the display-only
`salaryRange` string is ignored, per the structured-only compensation contract.

**Closure signals.** The detail's `applicationsClosed` or a non-null `closedAt` marks the
posting `Removed`, so the pipeline closes it by identity instead of leaving it to the unseen
sweep — the detail is the only call that can see the flag, so the hydrating path owns it.

## Risks / Trade-offs

- The listing endpoint's page-ignoring quirk makes `livehireMaxPages` mostly theoretical; the
  dedup-and-stop walk is correct under both behaviours (paged or re-serving).
- Cloudflare fronts the HTML host; if the vendor ever extends the challenge to `/careers-api`,
  crawls fail visibly (board health), and the scrapling tier is the established escalation path —
  out of scope for this adapter.
- Segment-vs-company naming: the board id is the segment, so `Company` is set from the segment
  slug the same way other slug-named boards do; a display-name mapping can be added later via
  the catalog's company field (`cmd/add-board --company`).

## Migration Plan

Additive only: a new adapter, one registry line, one recognizer row. No schema changes, no
backfill. The triggering posting's board (`flybuys`) can be added with `cmd/add-board` after
merge.

## Open Questions

- Whether other LiveHire tenants exist at meaningful scale (the challenge hides the surface from
  casual discovery; submissions will tell).
- Whether `salaryType` values other than the bounds-with-currency shape occur in practice.
