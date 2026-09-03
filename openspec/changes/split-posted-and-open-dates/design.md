## Context

Every job row carries two dates:

| Column | Meaning | Served as | Filterable today |
|---|---|---|---|
| `posted_at` | what the source claims | `posted_at` (after `EffectivePostedAt`) | yes — `posted_within_days` → `posted_ts` |
| `created_at` | when ingest first wrote the row | `created_at` | **no** |

`jobview.EffectivePostedAt` already falls back to `created_at` when the source date is
missing or in the future, so an undated board's two dates are the same number. The gap
this change addresses is the opposite case: a board that keeps *rewriting* `posted_at`,
which `jobreality.Classify` detects as `fake_freshness` (old by `created_at`, fresh by
`posted_at`) and reports only as a badge.

The search index is the only place a jobs query is answered. Meilisearch range
operators require a number, which is why `posted_ts` exists as unix seconds alongside
the RFC3339 `posted_at`. `created_at` reaches the index as a sortable string only.

## Goals / Non-Goals

**Goals**

- Let a reader bound the feed by *how long a posting has been open*, on a date no
  source can rewrite.
- Keep `posted_within_days` working, spelled the same, meaning the same — shared
  links, saved searches and the AI filter dialog all write it today.
- Make one modal pane the single answer to "how current is this posting", instead of
  a slider on one tab, three chips on another, and a button above the list.
- Ship without a 500 window and without a silently-empty filter.

**Non-Goals**

- Teaching `searchintent` the new bound. "Posted this week" is a sentence about the
  posting date; remapping it is a prompt change with its own evaluation.
- A *minimum* age bound ("open at least N days"). Nothing has asked for it.
- Changing what `reality` classifies or how. Only where its control lives.
- Reconciling with the in-flight `freshness.ts` badge work. Different surface, and
  this change adds no client-side date arithmetic to duplicate.

## Decisions

### 1. A new `created_ts` document field, not a filter over `reality.age_days`

`reality.age_days` is already an integer inside every indexed document, which makes
`age_days <= N` look free. It is not: `reality` is computed at **index time** from the
indexer's clock (`jobview.ClassifyReality(j, now, …)`) and then frozen into the
document. A document written sixty days ago still reports `age_days: 1`. The error is
unbounded and grows with exactly the postings this filter exists to catch — the stale
ones nothing has re-indexed.

`created_ts` is an absolute instant. The comparison happens at query time against the
request's clock, so it cannot go stale in storage. This is the same reason `posted_ts`
is seconds-since-epoch rather than a precomputed "days ago".

*Alternative considered:* re-deriving `reality` on every drain push. Rejected — it
would make every document's content depend on the clock, so `content_hash` would move
constantly and the incremental drain would degenerate into a continuous full rebuild.

### 2. `created_ts` is set in `search.FromJob`, the one document builder

All three indexers (`cmd/reindex`, `cmd/search-drain`, `internal/ingest/linkimport`)
build documents through `search.FromJob`. `posted_ts` is derived there; `created_ts`
joins it. No indexer changes.

Unlike `posted_ts` there is no fallback rule to apply — `created_at` is written by the
database on insert and is never absent. The field is set unconditionally.

### 3. A new parameter, not a redefinition of `posted_within_days`

`posted_within_days` is in `web/static/openapi.yaml` (the integration contract), in
saved searches, and in what the AI filter dialog emits. Redefining it would silently
change the meaning of links people already hold. `open_within_days` is additive; both
compose as AND, like every other scalar filter.

Both must be listed in `scalarFilters` (`query_params.go`). That list is what
`UnknownParams` checks: a working filter absent from it is reported back to the caller
as a param nobody reads — a lie in the response body, and one a test already guards
against in the other direction.

### 4. The control ships dark behind `PUBLIC_OPEN_WITHIN`

Declaring a filterable attribute does not retro-fill 1.36M documents. Until a full
rebuild lands, `open_within_days` matches only what has been re-indexed since — a thin
feed, not an error, so nothing alerts. This is precisely the hazard
`internal/search/search/AGENTS.md` records for the match sort, and the flag follows
that precedent exactly: default OFF, unparseable values OFF, and the URL parameter
honoured whether or not the flag is set, so the filter can be verified on production
before anyone can click it.

### 5. The `reality` facet moves into the `Posted` pane

The age bound and the reality classes answer one question, and the age is most of what
makes a posting read as evergreen in the first place. Splitting them across two rail
tabs plus an above-list button asked the reader to know which of three controls held
the version of "recent" they wanted.

The rail is the only route into a job facet from the UI — a param with no entry is
still parsed and still sent, just unreachable, which is how `reality` sat invisible
before it got a row. `filterSections.test.ts` guards that cover, so hosting `reality`
in another pane is recorded in its `HOSTED_ELSEWHERE` map rather than simply dropped.

### 6. The above-list select bounds `open_within_days`

One select, two candidate bounds. It takes the one a board cannot rewrite, because
that is the question the control's placement implies ("don't show me old things") and
the one that was quietly failing. `posted_within_days` stays reachable in the modal.

## Risks / Trade-offs

**A binary asking for an undeclared filterable attribute 500s every filtered search.**
Meili answers `invalid_search_filter` (400) and the handler maps any Meili error to
500, so the page breaks rather than degrading. → Patch the **live** index settings
before rolling the image. Settings updates are cheap and documents lag harmlessly;
the reverse order is a hard outage for ~26 min. Recorded in
`internal/search/search/AGENTS.md`.

**The filter is silently near-empty until a full rebuild writes `created_ts`.**
The incremental drain only pushes documents whose `content_hash` moved, and the new
field is not in that hash — the same trap `is_tech` and `requires_clearance` both fell
into. → `PUBLIC_OPEN_WITHIN` keeps the control hidden until someone has confirmed the
rebuild landed.

**A full rebuild is no longer the ~26 min the docs quote.** It now also writes skill
vectors (~10 GB, HNSW graph construction), and it refuses to start with under 45 GB
free — silently, from the service's point of view, while the timer still reads active.
→ Check disk first, `systemctl stop freehire-reindexw.timer` before starting, and
budget hours rather than minutes.

**Two date filters is two things to explain.** On the great majority of postings they
agree, so the second control earns nothing and adds a choice. → Each slider is
labelled for *whose* date it is, and they sit in one pane so the difference is read
once rather than hunted for.

**Removing the `Hide evergreen` toggle costs a one-click action.** It becomes two
clicks on a chip inside the modal. → Accepted: the toggle wrote one sign of one value
of a facet, which is a filter wearing a button's clothes, and it was the widest control
in a toolbar that overflowed a 390px viewport.

## Migration Plan

No data migration — `created_at` is already populated on every row.

Deploy order, each step verified before the next:

1. Patch the **live** index settings to declare `created_ts` filterable.
2. Deploy the binary. The API accepts `open_within_days` from this moment; results are
   incomplete until step 4.
3. `systemctl stop freehire-reindexw.timer`, and confirm ≥45 GB free.
4. Full `make reindex` (`REINDEX_DEDUP` unset). This writes `created_ts` into every
   document.
5. Verify by hand: `?open_within_days=3` against a posting known to carry a rewritten
   posting date.
6. Set `PUBLIC_OPEN_WITHIN=1`, restart web, re-enable the reindex timer.

**Rollback:** unset the flag and restart web — the control disappears and the URL
parameter stops being written. Removing a filterable attribute needs no dance at all:
the binary stops asking first, and a stale declaration in the live index is harmless
until the next rebuild.

## Open Questions

None blocking. One to revisit after the change lands: whether `searchintent` should
map any phrasing to `open_within_days` — "still open", "not an old posting" — which
needs prompt evaluation rather than a decision here.
