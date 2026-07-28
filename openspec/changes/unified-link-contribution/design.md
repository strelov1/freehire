## Context

Four surfaces accept a job link. `POST /api/v1/jobs/resolve`
(`internal/handler/resolve_job.go`) is the most complete: catalog lookup →
`linkimport.Import` → `contribution.Submit` as a fallback. The website form and the
Telegram webhook call `contribution.Submit` directly and never attempt an import. The CLI
has no equivalent at all — its `submit` command writes to `internal/submission`, the
moderation queue for hand-authored job cards, which is a different feature that happens to
share a verb.

Three facts about the existing code shape this design:

- `contribution.RecognizeBoard` (`internal/contribution/board.go`) already turns a URL into
  `(source, board, canonical)` for ~50 ATS hosts, network-free, fail-safe by construction.
- `sources.Source` is `Fetch(ctx, CompanyEntry) ([]Job, error)` with
  `CompanyEntry{Company, Provider, Board, …}` — everything needed to crawl one tenant's
  board is already addressable by `(provider, board)`.
- `linksource/greenhouse.go` already reuses `sources.MapGreenhousePosting` and
  `sources.NamespaceExternalID` so an imported posting carries the identity the ingest
  crawl would have produced. Identity parity is an established requirement, not a new one.

So the work is mostly wiring existing parts into one sequence, plus one genuinely new
adapter.

## Goals / Non-Goals

**Goals:**

- One intake sequence with four outcomes, reached identically from web, Telegram,
  extension, and CLI.
- A vacancy the system *can* read is handed to the user immediately, whatever surface
  they used.
- A board that is not crawled is queued for onboarding even when the vacancy was imported.
- Every intake is attributable to a user and a surface.
- Single-page resolution covers every host the board recogniser knows.

**Non-Goals:**

- Onboarding boards automatically into `sources/*.yml`. The queue stays manual (the
  `onboard-contributions` skill drains it). Automating it is a separate change.
- Importing *all* postings on a board during intake. The board is fetched to find one
  vacancy; writing the rest is a tempting seam but changes what a contribution means and
  what it costs.
- Reworking `internal/submission` (the moderation queue). Only the CLI's misleading
  `submit` entry point is removed.
- Changing the reward amount or the credits model.

## Decisions

### The intake sequence lives in one place and returns one shape

`POST /api/v1/jobs/resolve` stays the single server entry point; the outcome set grows
from three to four:

| Outcome | Meaning | Status | Body |
|---|---|---|---|
| `found` | the catalog already carries this posting | 200 | `public_slug` |
| `tracked` | board is crawled, posting was missing — imported now | 201 | `public_slug`, `company_slug` |
| `imported` | imported, and the board was queued for onboarding | 201 | `public_slug` |
| `queued` | nothing could read the page; link recorded for triage | 202 | `public_slug: null` |

`tracked` and `imported` differ only in what the system knows about the *board*, which is
exactly the distinction the user asked to be told about ("вот есть компания, нужно
подождать"). Both import. The check that separates them is `repo.BoardTracked`, already
written and already used by `contribution.Submit`.

*Alternative considered:* keep three outcomes and let the client infer "tracked" from the
presence of `company_slug`. Rejected — the client would be re-deriving a server-side fact,
and the Telegram reply needs the distinction in words.

### Recording a contribution moves out of the fallback branch

Today `queueForTriage` runs only when `imported == false`. It becomes an unconditional
step after a successful import too, gated on "board not already crawled". Since the import
already resolved `(source, board)` for the board-coverage path, no second recognition pass
is needed in the common case.

This is what forces the schema change: with the board no longer the row identity,
`UNIQUE (source, board)` has to go.

### The board's uniqueness is left to migration 0049

The plan was to drop `UNIQUE (source, board)` so several links to one board could each be
recorded with their own submitter. Mid-implementation, PR #1218 shipped (and was applied to
prod) a narrower fix for the same constraint: a partial unique index over the live statuses,
so a REJECTED board releases its identity and can be contributed again, while a queued or
onboarded one still refuses duplicates.

That settles it, and the other way round from this design's first draft. The reward stays
one-per-board on purpose: paying a later contributor for a board already in the queue buys no
coverage and invites farming one board from several accounts. So the constraint keeps doing
the concurrency work — exactly one insert wins a race — and this change only adds `surface`.

### Board coverage is one adapter over the ingest registry, not ~50 adapters

New `linksource` adapter, appended after the host-scoped ones and before `generic`:

1. `RecognizeBoard(url)` → `(source, board)`; decline if unrecognised.
2. look up `sources.All(client)[source]`; decline if the provider has no ingest adapter.
3. `Fetch(ctx, CompanyEntry{Provider: source, Board: board})`.
4. select the posting whose URL or namespaced external id matches the submitted link;
   `ok=false` if none matches, error only if the fetch failed.

Coverage then equals the recogniser's host table and grows with it — the property the
spec states explicitly. Ordering matters: a platform with a dedicated adapter (Greenhouse,
Lever, Ashby, Workable) keeps its cheap per-job API, because the registry is consulted in
order and this adapter sits behind them.

*Alternative considered:* add a `FetchOne(board, id)` method to `sources.Source`. Better
per-request cost, but it is a breaking interface change across ~166 adapters to serve one
low-volume endpoint. If board fetches prove too expensive in practice, this is the seam to
take next — as an optional interface the adapter type-asserts, matching how
`StreamingSource`/`HydratingSource` already opt in.

**Vanity domains are covered through the existing network resolver, not by the
recogniser.** `RecognizeBoard` keys on host, so a supported ATS behind a company's own
domain (`careers.peraton.com` on iCIMS, `jobs.ea.com` on Avature) yields nothing — this is
the single largest category in the current review queue. `contribution.Service` already
has an optional `Resolver` (`internal/boardresolve`) that fetches the page and detects the
embedded ATS. Step 1 of the board-coverage adapter therefore tries the recogniser first
and the network resolver second, which is the same order `Service.resolveBoard` already
uses. Known hazard to preserve: `boardresolve` takes the *first* recognised ATS URL in a
page, which is how the platform's own host (`app.teamtailor.com`) was once recorded as an
employer's board — the adapter must not treat a platform apex as a tenant.

### `RecognizeBoard` moves to a shared package

Both `contribution` and `linksource` need it, and `contribution` importing `linksource`
(or the reverse) is the wrong dependency in both directions. It moves to
`internal/atsboard` — the `atsBoards` table, the extraction modes, and the URL parse —
leaving `contribution` with its service logic. `internal/contribution` and
`internal/linksource` both depend on it; nothing depends on them.

### Attribution is a column, not a new subsystem

`link_contributions` gains `surface text` (`web` | `telegram` | `extension` | `cli` |
`unknown`). `submitted_by` already exists. The surface arrives as an explicit request
field; an absent or unknown value records `unknown` rather than refusing the request, so
an older extension build keeps working.

`linkimport.Import` grows a parameter carrying submitter and surface, so the import path
can record the contribution itself rather than the handler doing it out of band.

### Surfaces become thin

- **Web** `/contribute` posts to `/jobs/resolve` and renders four outcomes.
  `POST /me/contributions` is removed; `GET /me/contributions` stays.
- **Telegram** `processTelegramContribution` calls the same intake service the handler
  does, and its replies gain the imported-posting link.
- **Extension** sends `surface: "extension"` and renders `tracked`.
- **CLI** drops `submit`, gains `contribute <url>` and `contributions`; the
  `using-freehire` skill documents them.

## Risks / Trade-offs

- **A board fetch to answer one vacancy is expensive** → it runs behind the existing
  per-user hourly budget and only for recognised hosts; the dedicated adapters take the
  common platforms first. Watch p95 on `/jobs/resolve` after rollout.
- **Dropping `UNIQUE (source, board)` removes a hard guarantee** → the reward becomes
  correct by query rather than by constraint, so it is covered by a build-tagged
  integration test asserting exactly one reward under concurrent submissions.
- **`link_contributions` grows faster** — several rows per board instead of one → the
  onboarding view must group by `(source, board)` rather than assuming one row. The
  `onboard-contributions` skill's query needs updating alongside.
- **Telegram now performs outbound fetches on a webhook path** → the work already runs in
  a bounded background goroutine (`telegramContribTimeout`, 15s); a board fetch may exceed
  it, so the timeout is raised and the reply reports a timeout as "we'll look at it" rather
  than silence.
- **Three repositories move together** → the server change is backward compatible (surface
  optional, `tracked` is a new value clients may treat as `imported`), so clients can ship
  after the server without a flag day.

## Migration Plan

1. Migration: drop `UNIQUE (source, board)` on `link_contributions`, add `surface`
   defaulting to `unknown`. Additive and reversible; no backfill (existing rows keep
   `unknown`).
2. Deploy the server: new outcome, attribution, board-coverage adapter. Existing clients
   are unaffected — they send no surface and receive `tracked` only in a case that
   previously returned `queued`.
3. Ship web, then extension, then CLI.
4. Update the `onboard-contributions` skill's query to group by board.

Rollback: the server change is safe to revert independently; the migration is not
re-applied on rollback (the unique constraint would fail against rows that legitimately
duplicate a board, so re-adding it requires deduping first — noted here so a rollback is
not attempted blindly).

## Open Questions

- Should `found` also record attribution? It performs no fetch and creates nothing, so it
  is the cheapest surface to abuse but also the least harmful. Currently: not recorded.
- Whether importing a vacancy on an already-crawled board should nudge that board's next
  ingest run rather than waiting for the schedule. Out of scope; noted as a seam.
