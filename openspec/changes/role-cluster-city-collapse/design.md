## Context

`role_fingerprint = sha256(company_slug ⨝ normalize(title) ⨝ normalize(description))`
(`internal/jobhash/rolefingerprint.go`), computed at `UpsertJob` time and stored on
`jobs.role_fingerprint`. It keys three features: the collapse (`duplicate_of`,
`RecomputeRoleDuplicatesForCompany`), the `/copies` list (`ListRoleClusterCopies`),
and the reality signal's repost/mass counts (`job-reality-signal`). `normalizeRoleText`
only lower-cases and collapses whitespace, so an ATS that appends the city to the
title (`"… Engineer, Krakau"`) yields a distinct fingerprint per city and the cluster
never forms.

A spike over 3.98M open prod jobs established: (1) the trailing title clause is often
not a city (specialty, shift, employment-type, brand, store code); (2) the description
— already in the key — reliably separates genuinely-different tech roles (speechify:
one description per specialty, ~129 city variants each); (3) so a generic suffix strip
is safe for the tech catalogue, and a fragile city dictionary is unnecessary (and would
miss exonyms like `Krakau`≠`Kraków`).

The collapse also removes non-canon rows from the search index, so a multi-country
cluster collapsed to one country's canon would vanish from the other countries' filters
unless the canon's geography is widened.

## Goals / Non-Goals

**Goals:**
- Per-city title variants of one role collapse to one canonical card and one `/copies`
  cluster.
- The canonical card stays findable by every city/country/region its cluster is open in.
- Existing catalogue rows collapse without waiting for a re-crawl (backfill).

**Non-Goals:**
- Fuzzy/semantic dedup for cases where the city is embedded in the *description* (out of
  scope; descriptions differ → no collapse, unchanged).
- Perfect handling of non-tech retail mass-posters whose title suffix is a shift /
  employment-type over a templated description (accepted over-collapse).
- Aggregator→ATS cross-source suppression (separate, existing capability — its own
  suffix/subset logic is untouched).

## Decisions

### D1 — Single city-agnostic `role_fingerprint` (not a second column)
Strip the trailing clause inside `normalizeRoleText`, so all three consumers share one
fingerprint. Chosen over a separate `cluster_fingerprint` column: simpler (one key, no
migration, no dual backfill), and counting per-city variants together in the reality
signal is *more* correct — 5 cities of one role is a genuine mass-posting. Accepted
side effect: `repost_count`/`mass_posting_count` rise for such clusters; likely-evergreen
still needs convergence, so counts alone don't reclassify.

### D2 — Generic separator strip, description as the guard (no city dictionary)
Strip the last clause after ` , ` / ` | ` / ` @ ` / spaced ` - ` ` — ` ` – `. Suffix only
(prefix preserved → seniority safe). Guard: keep the original if stripping leaves < 2
words. Safety comes from the description remaining in the key, not from proving the tail
is a city — the spike showed this holds for tech. A shared separator helper is extracted
so the (existing) aggregator-suppression suffix logic and this one can converge over time.

### D3 — Geo union is a full-reindex property
Add `RoleClusterGeoAll` (one aggregate pass, `(company_slug, role_fingerprint) →
union(countries,regions,cities,work_mode)` over open rows), mirroring `RoleClusterCountsAll`
/ `buildRealityLookup`. `cmd/reindex` builds the lookup once and `search.FromJob` merges
the cluster union onto the canonical document. The incremental ingest push sees only its
own row, so a freshly-pushed canon may be geo-incomplete until the next full reindex —
accepted, exactly as the reality counts already behave.

### D4 — Backfill by recompute, not migration
`role_fingerprint` is only written at ingest. Provide a one-shot backfill that recomputes
it for open rows using the new function; then the standard reindex recomputes `duplicate_of`
and re-indexes canons with unioned geography. RESOLVED: a dedicated
`cmd/backfill-role-fingerprint` (role_fingerprint is a jobhash concern, not a jobderive
one, so folding it into `cmd/backfill-derive` would muddy that command's scope; a dedicated
one-shot matches the existing `cmd/backfill-*` family).

### D5 — Parentheses are never a separator; strip one clause only
A read-only survey of prod tech titles settled two edges of the strip rule:
- **Parentheses are excluded.** A trailing `(...)` is almost never geography — it is
  `(all genders)`, `(w/m/x)`, `(m/w/d)`, `(GCP)`, `(MAKO)`. Stripping it would merge
  distinct specialties/variants, so `(` is deliberately NOT in the separator set (and
  `(m/w/d)` in the Towa titles is correctly retained).
- **Only the last clause is stripped.** The trailing clause is a MIX across all separators
  — geography (`USA`, `TX`, `Latin America`, `Chennai`), specialty (`Backend`, `Data
  Science`, `Platform`), grade (`Senior`, `Vice President`, `AVP`), work-mode (`Remote`,
  `Hybrid`), brand (`Inc.`). No separator reliably means "city", so safety rests on the
  description staying in the key (distinct specialties carry distinct descriptions), not on
  proving the tail is geographic. Stripping only one clause keeps the rule conservative.

## Risks / Trade-offs

- **Over-collapse of non-tech shift/type variants** (whataburger `Day/Evening/Overnight`,
  macy's `Full/Part Time` with templated descriptions) → Mitigation: out of IT scope; merge
  is same-role/different-hours, not harmful; the `/copies` list still exposes each posting.
- **Reality-signal count jump** for city clusters → Mitigation: intended and more correct;
  evergreen classification requires convergence, so no spurious reclassification from counts.
- **Geo-incomplete canon between reindexes** (incremental push) → Mitigation: transient;
  the next full reindex fills the union; same contract as reality counts today.
- **Backfill cost** (regex over the open catalogue) → Mitigation: run once, off-peak; the
  spike's equivalent full scan is heavy but bounded, and the per-company recompute already
  scopes locks.
- **Multi-part geographic suffix under-collapses** (`Analyst - Dallas, TX`, `… - Remote,
  Latin America`): only the last clause is stripped (`, TX`), so the city remains and the
  per-city variants do NOT cluster. This is a safe MISS (under-collapse), not an over-merge;
  the common single-suffix case (the Towa `…, Krakau` pattern) still collapses. Extending to
  multi-clause stripping is deferred — it is more aggressive and the description guard would
  be doing more of the work.
- **Suffix grade / work-mode tail** (`X, Senior`, `X - Remote`): the prefix-only seniority
  guard does not cover a trailing grade or work-mode, so such a tail is stripped and could
  merge grades/modes → Mitigation: bounded by the description-in-key guard (distinct grades
  carry distinct JDs); observed rare in the tech catalogue.

## Migration Plan

1. Ship the fingerprint change + geo union + backfill command behind no flag (pure
   derivation change).
2. On deploy: run the `role_fingerprint` backfill once, then a full `reindex` (which runs
   `RecomputeRoleDuplicatesForCompany` and re-indexes canons with unioned geography).
3. Rollback: revert the code; `role_fingerprint` reverts on the next ingest/backfill and the
   next reindex recomputes `duplicate_of`. No schema change to undo (no new column).

## Open Questions

- Backfill home: extend `cmd/backfill-derive` vs a dedicated one-shot — resolve during
  implementation based on whether `backfill-derive` already rewrites `role_fingerprint`.
