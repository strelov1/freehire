## Context

See proposal.md for the discovery. HRMOS's listing (`hrmos.co/pages/<board>/jobs?page=N`) is a
standard paginated HTML listing — the shape `internal/ingest/sources/html.go`'s
`crawlAllPagedLinks`/`pagedLinks` already handles — and each job detail page carries a
standard schema.org `JobPosting` `application/ld+json` block, decoded the same way
`herp-source` (this session's immediately preceding change) already does via `ldJobPosting`.

## Goals / Non-Goals

**Goals:**
- Page the listing to exhaustion using the existing shared paging helper, in its
  fail-on-any-gap form so the `fullBoardListing` guarantee is genuine.
- Match job links by an exact, non-loose path shape, applying the lesson `herp-source`'s
  review surfaced: a link one segment deep after the board is not automatically a job.
- Map the one schema.org-standard structured field (`employmentType`) that safely translates
  to freehire's own controlled vocabulary without guessing.

**Non-Goals:**
- `baseSalary` and structured `jobLocation` sub-fields beyond a joined address string. HRMOS's
  ld+json exposes both, but mapping salary correctly needs verifying currency/period shape
  and rounding conventions across more companies than this change's live sample covered —
  left to the pipeline's own LLM-derived guess for now, matching every adapter's documented
  "when uncertain, leave it to enrichment" default. A follow-up change can add it once
  measured.
- A bespoke `cmd/harvest-boards` prober, for the same reason `herp-source` skipped one: no
  cheap count endpoint exists, and `adapterProber`'s fallback already covers a board-keyed
  provider with no bespoke entry.

## Decisions

**`crawlAllPagedLinks`, not `crawlPagedLinks`.** The plain variant returns success with
whatever links it gathered when a LATER page fails, which would let a board's true size shrink
silently — exactly wrong for a `fullBoardListing` adapter, whose whole point is that the
pipeline's unseen-sweep can trust an empty diff as evidence, not as an artifact of a cut-short
crawl. The `All` variant fails the whole `Fetch` instead, matching this adapter's own claim to
the marker.

**Job link shape: `jobs/<jobID>` (two segments), not one.** `herp-source`'s review found that
"one segment after the board" is not a safe definition of "a job" — it also matches a
platform's own single-segment navigation path (there, `/top`). HRMOS's own shape is naturally
narrower (a literal `jobs` segment precedes the id, distinguishing a job link from a bare
board-root link or any other single-segment path HRMOS might add later), so this adapter
requires the literal segment rather than accepting any lone one — closing the same class of
gap deliberately rather than by accident of URL shape.

**`employmentType` mapping is a closed, explicit table, not a general string transform.**
Schema.org's `employmentType` enum (`FULL_TIME`, `PART_TIME`, `CONTRACTOR`, `TEMPORARY`,
`INTERN`, `VOLUNTEER`, `PER_DIEM`, `OTHER`) only partially overlaps
`vocab.EmploymentTypeValues` (`full_time`, `part_time`, `contract`, `internship`,
`fellowship`) — two of eight case-fold cleanly (`FULL_TIME`, `PART_TIME`), two more need a
name change (`CONTRACTOR`→`contract`, `INTERN`→`internship`), and the remaining four
(`TEMPORARY`, `VOLUNTEER`, `PER_DIEM`, `OTHER`) have no freehire equivalent and map to empty
rather than the nearest visually-similar value, since `Job.EmploymentType`'s contract is "the
platform's OWN structured field," not a best-effort guess. `fellowship` has no schema.org
counterpart at all and is simply never produced from this mapping.

## Risks / Trade-offs

- **A company whose listing needs more pages than a sane cap allows would under-count.**
  Bounded generously (50 pages) against the largest live sample measured (CyberAgent Group,
  413 jobs over 5 pages at 100/page) — the same "measure, then cap with headroom" approach
  other paginated adapters in this codebase already use.
