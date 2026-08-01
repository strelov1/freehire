## Why

The board harvest is bottlenecked on its worklist, not on its machinery. `harvest-ats`
follows companies to their ATS board perfectly well, but the only companies it is ever
handed come from curated collection datasets and a world-universities directory — static
lists that say nothing about whether anyone there is hiring. LinkedIn's public
`jobs-guest` endpoints answer exactly that question for any keyword and market, and every
job page carries two facts we can act on: the employer's own website, and the job's
identifier **in the employer's ATS**. The second is the valuable one — it turns a guess
about a board slug into a fact a provider's own API can confirm.

## What Changes

- A new `cmd/harvest-linkedin` host tool reads a query worklist, pages LinkedIn's public
  job search, and emits the companies behind those postings — name, website, and the
  posting's ATS-native id — as the candidate worklist the existing harvest consumes.
  Companies already in the catalogue are dropped before any extra request is made.
- `harvest-ats resolve` detects boards through `internal/boardresolve` instead of
  `atsdetect.Detect` alone, widening detection from three providers to the full
  `atsboard.Recognize` set plus the self-hosted platforms whose board *is* the careers
  host (Teamtailor, Phenom, Radancy).
- `harvest-ats resolve` additionally emits *candidate* board slugs, derived offline from
  the company's domain, LinkedIn slug and name, for companies whose careers page yields no
  board — today's dead end for JS-only careers pages, which the tool skips outright.
- `harvest-boards` accepts an expected posting id on a seed entry and confirms a candidate
  board by finding that id among the board's live postings, rejecting it otherwise. This
  is what makes an offline-derived slug safe to propose: confirmation is an exact match,
  not a plausible one.
- No new runtime surface, no database, no cron. Every new binary is run-once by hand, and
  only boards confirmed against a provider's official API reach `sources/*.yml`.

## Capabilities

### New Capabilities

- `linkedin-board-discovery`: turning LinkedIn's public job search into a candidate
  worklist of hiring companies — query worklist, company-level de-duplication, catalogue
  filtering, the two facts read from each posting, and the empty-result rule that
  distinguishes "nobody is hiring" from "we have been blocked or the markup moved".

### Modified Capabilities

- `domain-ats-harvest`: board detection during resolution widens from the three-provider
  HTML scan to the full recognizer including self-hosted platforms; resolution gains
  candidate-slug emission for companies whose careers page yields no board.
- `board-harvest`: a seed entry may carry an expected posting id, and a candidate board is
  then kept only when the platform reports a live posting with that id.

## Impact

- New: `cmd/harvest-linkedin`, `harvest/linkedin-queries.yml`.
- Modified: `cmd/harvest-ats` (resolve detection + candidate slugs),
  `cmd/harvest-boards` (seed shape, id confirmation on the providers whose single probe
  request yields the board's complete live posting list: greenhouse, lever, ashby,
  recruitee).
- Reused unchanged: `internal/boardresolve`, `internal/atsboard`, `internal/sources`
  (SSRF-guarded client, retry/backoff, pacer). LinkedIn serves these endpoints under the
  project's own User-Agent, so no request forgery is involved.
- Out of scope: ingesting LinkedIn postings themselves. LinkedIn data is a transient
  worklist; the repository only ever gains boards this project validated itself.
