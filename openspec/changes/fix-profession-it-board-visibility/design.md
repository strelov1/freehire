## Context

See proposal.md for motivation. Key facts the approach relies on, all verified
in the current code (not from the issue's own analysis):

- `internal/job/jobderive.Input` already has a documented precedence pattern:
  a structured signal from the source adapter wins outright over the
  dictionary for `WorkMode`, `Seniority`, `Category`, `EmploymentType`,
  `ExperienceYearsMin`, `EducationLevel`, `EnglishLevel` (`jobderive.go:21-72`).
  `is_tech` has no such input today — it is purely `deriveIsTech(category,
  title)` (`jobderive.go:186,272-282`).
- `internal/ingest/sources.Job` is one flat, adapter-agnostic struct
  (`source.go:47-155`) shared by ~40+ adapters; every field on it is either
  populated by an adapter that has that structured data or left zero for the
  dictionary to fill. There is no per-source side-channel.
- Profession's `detail()` (`profession.go:310-343`) is reachable only after
  `list()` (`profession.go:222-242`) succeeds, and `list()` refuses (returns an
  error, no postings) for any board other than `itdev`/`itops`
  (`ProfessionCrawlsCategory`, `profession.go:295-302`). So every `Job` ever
  returned by `detail()` is, structurally and unconditionally, from one of the
  platform's two IT boards — no runtime board check is needed inside `detail`.
  This mirrors a precedent already in the same file: the ingest-time
  non-tech-title filter is skipped for these two boards on the same evidence
  (`profession.go:24-27,289-294`, `internal/ingest/sources/AGENTS.md:127`) —
  "the platform's own facet picks the slice" is an established pattern here,
  just not yet threaded through to `is_tech`.
- `search.CategoryUnresolved` (`document.go:148-157`) is a `db.Job → bool`
  pure function called identically from both the incremental drain
  (`cmd/search-drain/indexer.go:110`) and the full reindex
  (`cmd/reindex/main.go:514`), so a single change there reaches both paths.
- The persisted `external_id` for Profession already carries the board as a
  prefix (`itdev:2975203`, `itops:2976684` — see `pipeline.go:1076-1078`,
  `externalid.Namespace(e.Board, ...)`), which is what makes an SQL-only
  backfill possible without re-crawling or re-deriving anything else.

## Goals / Non-Goals

**Goals:**
- New Profession `itdev`/`itops` postings get `is_tech = true` at ingest, so
  they enter the enrichment queue instead of stalling forever.
- Existing stuck Profession `itdev`/`itops` rows (`is_tech` unknown today) get
  the same correction without a re-crawl.
- A confirmed-technical job (`is_tech = true`) is never excluded from search
  solely for lacking a specific category — closing the gap generally, since
  `CategoryUnresolved` cannot distinguish which evidence produced `is_tech`
  once it is persisted as a single tri-state column.

**Non-Goals:**
- Not touching `internal/ingest/sources/infojobs.go`, which the investigation
  found has the same latent defect shape (single dedicated-IT-category source,
  no structured category signal). Out of scope for this issue; the mechanism
  added here (`Input.IsTechHint`) is available for a follow-up on that adapter
  without further plumbing, but wiring it up is not part of this change.
- Not attempting to make enrichment assign a more specific category for these
  postings — the issue's own expected behavior explicitly accepts staying
  without one.
- Not adding a general is_tech-provenance column (e.g. "which evidence set
  this true") to distinguish source-hinted from dictionary-hinted jobs at the
  search layer. See Decisions below for why the broader `CategoryUnresolved`
  carve-out is preferred over that alternative.

## Decisions

**1. New structured input, not a source-specific special case in `jobderive`.**
Add `IsTechHint bool` to both `sources.Job` and `jobderive.Input`, following
the exact shape of the existing structured-signal fields, rather than having
`jobderive` (layer `job`) special-case "source == profession" — which it
cannot do anyway without importing `internal/ingest` (a layering violation:
`ingest` sits above `job`). `deriveIsTech` gains a third parameter and checks
the hint first, before `TechEvidence` and before the non-tech detector: a
source that crawled a posting specifically because it filed it as IT is
better evidence than a generic title dictionary, matching how the other
structured scalars fully replace the dictionary rather than merely seeding it.

**2. `CategoryUnresolved` trusts persisted `is_tech`, not a per-hint provenance
flag.** Once `is_tech` is a bare tri-state column, the search layer cannot
tell whether it became `true` via the source hint or via `classify.IsTech`
matching the title with no sub-category. Two ways to close the reported gap:
  - (a) Widen `CategoryUnresolved` to admit any `is_tech = true` job, from any
    source.
  - (b) Add a second column/flag carrying "source asserted this, not just the
    dictionary" and only that flag bypasses the gate.
  We pick (a). The `tech-classification` spec already requires the
  tech-title dictionary to be conservative and never guess (`internal/dict/classify/AGENTS.md`,
  spec's "MUST NOT contain generic terms dominated by non-software roles"), so
  a dictionary-only `is_tech = true` with an unresolved category is already a
  confident claim, not a guess — excluding it from search was never
  justified by `CategoryUnresolved`'s own stated purpose (keeping out
  undifferentiated bulk), only an accidental side effect of the two facets
  being derived independently. (b) would carry that inconsistency forward
  indefinitely for every source but Profession, add a column purely to encode
  a distinction the rest of the system doesn't need, and contradict the "no
  overengineering" project guidance for a gap this narrow.

**3. Backfill is a direct SQL `UPDATE`, not a `jobderive`-based re-derivation
pass.** The board is already recoverable from the stored `external_id` prefix
(`itdev:`/`itops:`) for `source = 'profession'`, so no re-crawl or
`jobderive.Derive` call is needed — same shape as `backfill-clearance`, which
also computes its target column directly rather than through the general
derivation path. The affected row count is bounded (two boards, low
thousands at most), so — unlike `backfill-slug-folded`'s chunked pass over a
multi-million-row table — a single `IS DISTINCT FROM`-guarded statement is
enough; chunking would be complexity with no row count to justify it.

**4. No outbox/reindex automation added.** Like `backfill-clearance` and
`backfill-company-type-hint`, the backfilled column is not part of
`content_hash`, so the change needs a follow-up full `make reindex` to reach
Meilisearch. This is documented as an operational step, not automated,
consistent with how those two precedents are run.

## Risks / Trade-offs

- **[Risk]** Widening `CategoryUnresolved` (Decision 2) could surface
  previously-hidden jobs from other sources whose title matched
  `classify.IsTech` with no sub-category, changing search result composition
  beyond Profession. → **Mitigation**: the tech-title dictionary is curated
  and conservative (word-boundary match, no generic terms) per its own spec
  contract; a false positive there is already a defect in that dictionary
  regardless of this change. The set this newly surfaces is exactly "confident
  tech title, not yet or never further categorized" — squarely inside what
  the site's `is_tech` facet already presents to users as "Tech" today
  (`is_tech` is a filterable, user-facing facet per `tech-classification`'s
  existing "is_tech search facet with filter" requirement), so nothing new is
  being asserted to users, only made findable via the general search index.
- **[Risk, caught by review — fixed]** An initial draft of the backfill hardcoded
  `external_id LIKE 'itdev:%' OR LIKE 'itops:%'` as SQL literals — a second,
  driftable answer to "which boards" (`profession.go`'s `professionITBoards`
  being the first), and case-sensitive against a board whose stored casing
  `externalid.Namespace` never normalizes. Fixed by exporting
  `sources.ProfessionITBoardNames()` and building the predicate from
  `externalid.BoardPattern` (the same helper `ExistingExternalIDsByBoard`/
  `BackfillBoardCompany` already use for a board-scoped `external_id` match)
  with `ILIKE ANY(...)` — a third IT board, or a differently-cased one, reaches
  the backfill with no second edit.
- **[Risk, caught by review — fixed]** `cmd/backfill-derive` re-derives every
  job's facets, including `is_tech`, from `jobderive.Derive` on a bare title
  and category — it never re-crawls, so it never saw `IsTechHint`. Its routine
  pass (AGENTS.md: run after a dictionary change, followed by a reindex) would
  have silently rewritten every Profession itdev/itops row this change fixed
  back to `is_tech` unknown, since neither its title nor its category resolves
  on their own. Fixed by adding `sources.ProfessionConfirmsTech(source,
  externalID)` — recovers the board from the row's own stored `external_id`
  namespace prefix, the same way the backfill does — and wiring it into
  `deriveRow`'s `IsTechHint`.
- **[Trade-off]** `infojobs.go`'s identical defect shape (Non-Goals) is left
  unfixed. Accepted to keep this change scoped to the reported issue; the new
  `IsTechHint` mechanism, and `cmd/backfill-derive`'s now-general awareness that
  a structured hint can survive re-derivation, are both ready for it without
  further plumbing changes.

## Migration Plan

1. Ship the code changes (`jobderive`, `sources.Job`, `profession.go`,
   `pipeline.go`, `search.CategoryUnresolved`) — safe on their own: they only
   add a new input that nothing sets yet except Profession's adapter, and
   widen a search filter.
2. Deploy.
3. Run the new one-off backfill against production (`DATABASE_URL` only).
4. Run a full `make reindex` to reach Meilisearch with both the corrected
   `is_tech` values and the widened `CategoryUnresolved` logic.
5. Verify via the issue's own repro steps (the five named `itdev`/`itops`
   external IDs should now appear in `GET /api/v1/jobs/search?source=profession`).

No rollback complexity: the backfill only ever sets `is_tech` from unknown to
`true` on rows scoped to two boards (`IS DISTINCT FROM`-guarded, safe to
re-run), and the search-side change only ever widens what is included.
