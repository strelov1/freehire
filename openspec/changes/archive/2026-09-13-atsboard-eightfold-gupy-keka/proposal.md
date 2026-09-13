## Why

While bulk-seeding freehire's board catalog from an external ATS-company inventory
(kalil0321/ats-scrapers), Keka — a provider freehire already has a working ingest adapter for
— turned out to be **entirely unrecognized** by `internal/ingest/atsboard.Recognize`: 0% of its
inventory URLs resolved. This is a plain gap, not a design choice: Keka serves every tenant
from the vendor's own multi-tenant domain (`<tenant>.keka.com`), the exact shape `atsboard`'s
`subdomain` mode already handles for ~15 other platforms — it was simply never added.

`atsboard` is also the accept-set for the paid crowdsourced board-contribution flow
(`internal/ingest/contribution`, see `openspec/specs/link-contributions/spec.md`'s "Recording a
novel board and awarding AI credits"), so widening it is a deliberate, reviewable decision
rather than a silent side effect of unrelated harvesting work — hence its own change, per the
guard `internal/ingest/atsdetect`'s `TestLocalShapesStayOutsideTheSharedTable` enforces.

**Revision note (post-review):** this change originally also proposed adding Eightfold
(`eightfold.ai`) and Gupy (`gupy.io`) alongside Keka, on the same "same shape as ~15 other
subdomain entries" reasoning. A code review caught that the reasoning was wrong for both:
- `internal/ingest/sources/eightfold.go`'s board id is `"<host>/<domain>"` — a career-site host
  paired with an Eightfold tenant domain configured per board — never a bare `eightfold.ai`
  subdomain label. `internal/ingest/atsdetect` already documents Eightfold as NOT
  URL-derivable for exactly this reason.
- `internal/ingest/sources/gupy.go` is keyed by a **numeric `companyId`**, not the subdomain
  slug — a pasted `https://acme.gupy.io` link would derive board `"acme"`, which Gupy's API
  never accepts.
- Gupy additionally has at least one live, non-tenant subdomain (`portal.gupy.io`, Gupy's own
  multi-employer job-search aggregator) that nothing in `platformLabels` declined — exactly the
  "board that does not exist, paid for" failure mode this package exists to prevent.

Both entries and their test cases were reverted before this change was merged. Keka's adapter
comment confirms its board genuinely is the subdomain, and review found no non-tenant subdomain
issue for it (`app.keka.com` is already covered by the existing generic `"app"` platform
label — a regression test for it was added). What shipped is Keka only.

## What Changes

- Add one `subdomain`-mode entry to `atsBoards`: `keka.com` → `keka`.
- A pasted Keka link now resolves to a board the same way an existing Recruitee/BambooHR/
  Personio link already does — accepted (and rewarded) as a contribution instead of falling
  through to manual review, and usable by `cmd/harvest-boards` seed-based discovery.
- **Explicitly excluded from this change**: Eightfold and Gupy (see revision note above —
  their board-id shapes don't match a bare subdomain label) and Paycom (its URL shape is
  currently owned by `internal/ingest/atsdetect` as one of the five shapes deliberately kept
  outside the shared table; moving it in is its own decision this change does not make).

## Capabilities

### Modified Capabilities
- `link-contributions`: the "Supported-ATS board recognition" requirement's accept-set now
  includes Keka.

## Impact

- `internal/ingest/atsboard/board.go`: one new `atsBoards` row, no new extraction mode.
- No API, schema, or migration changes.
- Downstream effects, all through the existing shared table (per its own package doc — one
  definition, three consumers): `internal/ingest/contribution` (Keka board contributions are
  now accepted and rewarded), `internal/ingest/linksource` (board coverage recognizes the
  host), `internal/ingest/boardresolve` (a company's own careers page embedding Keka is now
  detected).
