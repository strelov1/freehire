# Unified link contribution

## Why

Four surfaces let someone hand freehire a job link, and they behave differently. The
browser extension asks `POST /jobs/resolve`, which looks in the catalog, tries a
link-source import, and falls back to the contribution queue. The website's `/contribute`
form and the Telegram bot call `contribution.Submit` directly: they only ever record a
*board* for later onboarding, so a user who pastes a link to a vacancy we could read
right now is told to wait instead of being handed the posting. The CLI offers neither —
its `submit` command writes to the unrelated moderation queue (`internal/submission`).

Two gaps make this worse than mere inconsistency:

- **The import path forgets who asked.** `linkimport.Import` takes no user id, so a
  successful import leaves no attribution. Abuse of the endpoint that makes our server
  fetch an arbitrary URL is visible only through a rate limiter and log lines.
- **A successful import ends the story.** Nothing is queued, so we never learn that the
  company's board exists and is worth crawling — we import one vacancy and stay blind to
  the other twenty on the same board.

Coverage compounds it: `linksource.All` carries 8 host adapters plus a generic JSON-LD
resolver, while `contribution.RecognizeBoard` recognises ~50 ATS hosts and
`internal/sources` implements ~166 providers. Most links a user pastes cannot be imported
today, not because the posting is unreadable, but because no *single-page* adapter exists
for a platform we already crawl in full.

## What Changes

- **One intake sequence, four outcomes**, served by `POST /api/v1/jobs/resolve` and used
  by every surface:
  1. the catalog already carries the posting → answer with its slug (`found`), no fetch;
  2. the board is already crawled but this vacancy has not landed yet → import it now and
     say the company is tracked and the rest will arrive on the next crawl (`tracked`);
  3. otherwise import it and record the board in the contribution queue for later
     onboarding (`imported`);
  4. nothing could read the page → record the link for triage (`queued`).
- **A contribution is recorded even when the import succeeds.** Importing one vacancy no
  longer hides the board from the onboarding queue.
- **Every intake is attributed.** The submitting user and the surface it came from
  (`web`, `telegram`, `extension`, `cli`) are stored for every outcome, so repeated or
  abusive use is visible in the data rather than only in logs.
- **`link_contributions` gains a `surface` column.** The board's uniqueness is deliberately
  left alone: PR #1218 (migration 0049) had just narrowed it to the live statuses so a
  rejected board stops claiming its identity forever, which settles the question — one live
  row per board, one reward per board. Paying later contributors for a board already queued
  buys no coverage and invites farming one board from several accounts.
- **Board coverage for single-page resolution.** A new link-source adapter derives
  `(source, board)` from any recognised ATS URL, fetches that tenant's board through the
  *existing* ingest adapter, and returns the posting the link points at. Coverage becomes
  every host `RecognizeBoard` knows and grows with that table, without writing ~50
  single-page adapters.
- **Website and Telegram move onto the unified sequence**, replacing their direct
  `contribution.Submit` calls. A Telegram user who pastes a readable vacancy now gets a
  link to it.
- **CLI:** `freehire submit` (moderation queue) is removed and replaced by
  `freehire contribute <url>`, plus `freehire contributions` to list one's own. Naming is
  aligned across surfaces.

## Capabilities

### New Capabilities

- `linksource-board-coverage`: resolving a single vacancy on any recognised multi-tenant
  ATS board by reusing the ingest adapter for that provider, rather than a bespoke
  single-page adapter per platform.

### Modified Capabilities

- `posting-import-by-url`: the outcome set gains `tracked`; a successful import now also
  records a contribution; every request is attributed to a user and a surface.
- `link-contributions`: the board is no longer the unique key of a row; recording is
  reached through the unified intake rather than a separate endpoint path; the Telegram
  flow answers with an imported posting when one could be read.

## Impact

**Server (`/Users/i_strelov/Projects/hire`)**

- `internal/handler/resolve_job.go` — the intake sequence and its four outcomes.
- `internal/handler/contributions.go`, `internal/handler/telegram.go` — both move onto
  the unified path.
- `internal/linkimport` — accepts the submitter and surface; writes the contribution.
- `internal/linksource` — the board-coverage adapter; a shared board recogniser extracted
  from `internal/contribution` so both packages use one definition.
- `internal/contribution` — reward gating moves off the unique constraint.
- `migrations/` — drop `UNIQUE (source, board)`, add the surface column.
- `internal/db/queries/*.sql` + `make sqlc`.

**Clients (separate repositories, coordinated with this change)**

- `../freehire-cli` — remove `submit`, add `contribute`/`contributions`; document both in
  `skills/using-freehire`.
- `../freehire-extension` — send the surface tag; render the new `tracked` outcome.
- `web/` — `/contribute` posts to the unified endpoint and renders all four outcomes.

**Risk**

The board-coverage adapter fetches a whole tenant board to answer about one vacancy. It
runs behind the existing per-user hourly budget and the SSRF-guarded client, and only for
hosts already on the recognised-ATS list.
