## Context

The user profile (`user_profiles`, one row per user) already stores `skills`,
`specializations`, and `location_preferences`. The jobs filter already supports
excluding facet values end-to-end: `FacetState` holds separate `include`/`exclude`
sets, `filtersToParams` serializes excludes as `?<param>_exclude=…`, and the Meili
filter builder (`internal/search/query_filter.go`) turns them into `skills != "X"`.
The **Apply my profile** action (`filtersFromProfile` in `web/src/lib/facetModel.ts`)
seeds only the *include* side today.

The gap is a place to record which skills a user wants to avoid, and one line of
seeding to push them into the existing exclude machinery. Nothing in the search
layer changes.

## Goals / Non-Goals

**Goals:**
- Persist an optional `excluded_skills` set on the profile, normalized exactly like
  `skills` (canonical lowercase, trimmed, deduplicated), empty allowed.
- Guarantee the wanted and excluded sets never overlap (a skill in both is dropped
  from excluded), so a committed filter never emits contradictory `skills = X AND
  skills != X`.
- Make **Apply my profile** seed excluded skills into the `skills` facet's exclude
  set.
- Let the user edit excluded skills in the profile form via a dictionary-constrained
  control mirroring the wanted-skills control.

**Non-Goals:**
- Feeding excluded skills into AI match analysis or the résumé verdict (future seam).
- Any change to the search/filter/exclude serialization or Meili filter builder.
- Excluding by category, company, or any facet other than `skills`.

## Decisions

**1. Store on `user_profiles`, not a new table.** `excluded_skills` is one-per-user,
same lifecycle as `skills`; a `text[]` column mirrors the existing shape. Alternative
(separate table) adds joins and lifecycle for zero benefit.

- Migration: `ALTER TABLE user_profiles ADD COLUMN excluded_skills text[] NOT NULL
  DEFAULT '{}'::text[];` — no cardinality CHECK (empty is valid, unlike `skills`).
  Standalone migration file; **applied manually to prod before deploy** (Postgres
  initdb only runs migrations on first volume init — the 0010 precedent).

**2. Overlap resolved by silent subtraction, wanted wins.** On save, after both sets
are normalized, `excluded_skills := excluded_skills \ skills`. Rationale: include and
exclude land in the *same* `skills` facet; overlap would self-cancel the filter. A
user who lists a skill as both wanted and avoided most likely means to keep it — and a
silent fix is friendlier than a 400 for a non-destructive conflict. Alternative
(reject with 400) was considered and rejected as user-hostile for a resolvable case.

**3. Normalization reuses the `skills` shape, but empty is allowed.** A dedicated
`normalizeExcludedSkills` mirrors `normalizeSkills` (trim/lower/dedup) minus the
non-empty requirement, keeping the two rules independent so future divergence is cheap.

**4. Seed on the existing `skills` facet.** In `filtersFromProfile`, after seeding the
include side from `profile.skills`, seed the exclude side from `profile.excluded_skills`
on the same `skills` facet (via the existing `facetSetSign(..., 'exclude')` /
exclude-set path). No new facet, no URL-format change.

**5. UI: a separate dictionary-constrained control.** The profile form gains a "Skills
to avoid" control that reuses the same canonical-skill typeahead source as the wanted
skills, so every excluded value matches a real `skills` facet token (a free-text value
could silently match nothing). The two controls exclude each other's current selections
from their own options to keep the sets visually disjoint; the backend subtraction is
the authoritative guarantee.

## Risks / Trade-offs

- **[Prod migration not auto-applied]** → The `ALTER TABLE` must be run manually on
  prod before the deploy that reads the column, per the 0010 header convention; note it
  in the migration file and the finish step.
- **[UI dictionary constraint frustrates power users]** → Excluded skills are limited
  to known skill tags. This is intentional (free text wouldn't filter anything) and
  matches the existing wanted-skills control; acceptable.
- **[Stale profiles pre-migration]** → Existing rows get the `DEFAULT '{}'`, so fetch
  returns an empty set and Apply-my-profile seeds no excludes — safe, no backfill.

## Migration Plan

1. Ship migration file; apply the `ALTER TABLE` manually on prod before deploying the
   backend that references `excluded_skills`.
2. Deploy backend (tolerates empty set for all existing profiles).
3. Deploy frontend.

Rollback: the column is additive and defaulted; reverting the app code leaves the
column unused and harmless. Drop the column only if fully abandoning the feature.

## Open Questions

None — the overlap rule (silent subtraction, wanted wins) is settled per brainstorming.
