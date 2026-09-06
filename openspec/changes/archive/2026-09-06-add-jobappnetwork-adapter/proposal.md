## Why

Issue #2080 measured 1181 sampled apply links an aggregator carried to boards `atsdetect.FromURL`
could not resolve. `apply.jobappnetwork.com` was the single largest unrecognized host — 26 hits,
more than the next three combined — and its board id (a numeric client id) sits right in the URL,
so it is one of the four platforms the issue judges worth an adapter at all (the other 337
unresolved links are either a correct refusal or a platform whose board cannot be derived from a
public URL). Onboarding it lets freehire crawl these employers' own postings directly instead of
only ever seeing them secondhand through whichever aggregator happened to carry the link.

## What Changes

- New `internal/ingest/sources` adapter, provider key `jobappnetwork`, for the ATS behind
  `apply.jobappnetwork.com` (the vendor is talentReef; the domain is its applicant-facing brand).
  A board is one numeric `clientId` — one employer's whole talentReef account, matching how the
  aggregator apply links already name it.
- Fetch is a single keyless `POST` per page to talentReef's own listing endpoint
  (`prod-kong.internal.talentreef.com/apply/proxy-es/search-en-us/posting/_search`, verified live
  2026-09-06 — see design.md for the full request/response shape and traps), filtered to the
  board's `clientId` and to `internalOrExternal: externalOnly` so an internal-transfer-only
  posting never reaches a public board. The listing carries each posting's whole body, so this is
  not a `HydratingSource`.
- New row in `internal/ingest/atsboard`'s `apiBoards` table so `atsdetect.FromURL` resolves an
  `apply.jobappnetwork.com/clients/<clientId>/posting/<id>/` link to `(source: jobappnetwork,
  board: <clientId>)` — the same mechanism that already resolves Ashby/Greenhouse/Lever API-host
  links, reused here because the shape (fixed prefix segment, board is the next one) is identical.
- One line in `sources.All` (registry.go) wiring the adapter over the shared keyless JSON client.
- `internal/ingest/sources/AGENTS.md` gains a "jobappnetwork traps" section recording what was
  verified live, in the same style as the Dayforce/Workstream/EDJOIN entries already there.

Not in scope (per the issue's own ordering — "do them one at a time, biggest first, and let the
yield decide"): `dayforcehcm` (already onboarded, `internal/ingest/sources/dayforce.go`),
`gr8people`/`workgr8.com`, and `werecruit.io`.

## Capabilities

### New Capabilities
- `jobappnetwork-source`: crawling one talentReef/jobappnetwork employer board (a numeric client
  id) through its public listing API, filtered to external, catalogue-eligible postings, and
  recognizing the platform's public URL shape for board onboarding.

### Modified Capabilities
(none — no existing capability's requirements change)

## Impact

- `internal/ingest/sources/jobappnetwork.go` (+ `_test.go`): new adapter.
- `internal/ingest/sources/registry.go`: one new `registry["jobappnetwork"] = ...` line.
- `internal/ingest/atsboard/board.go`: one new `apiBoards` row.
- `internal/ingest/sources/AGENTS.md`: one new "traps" section.
- No schema change, no new env var (keyless), no worker change — the new provider is picked up by
  the existing `cmd/ingest <provider>` / scheduler / board-catalog machinery once boards exist for
  it (`cmd/add-board --provider=jobappnetwork --board=<clientId> --company=... --apply`).
