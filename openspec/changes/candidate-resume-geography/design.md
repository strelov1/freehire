## Context

Jobs have carried derived `countries`/`regions` since migration `0001`; candidates carry
nothing. The only record of where a candidate is sits in
`users.resume_structured->>'location'`, a free-text line the LLM lifted off the CV.

Measured on production, 2026-08-01:

| | |
|---|---|
| users / with a stored CV | 400 / 169 |
| structured résumé present / current | 132 / 129 |
| non-empty `location` line | **100** |
| → resolves to a country through the existing dictionary | **91 (91%)** |
| → resolves to a region only (`EU / Remote`) | 1 |
| → resolves to a city only | 2 |
| → resolves to nothing | 6 |
| profiles / with `location_preferences` | 145 / 70 |
| profiles stating a `base.country` | **27 (19%)** |
| users with a CV location and no stated base | **80** |

Every one of the 91 resolves to exactly **one** country — multi-country derivations do
not occur in the current data, but the code must still decide what to do with one.

Three constraints shape the design.

**The same string means opposite things on the two sides.** `location.Parse` was written
for an ATS posting, where the location answers "where is the work". A CV's location line
answers "where is the person". The divergence is not academic: a bare `Remote` is
correctly globalized to the `global` region for a job, and that same expansion is a lie
about a human being.

**A fact and a wish already live in different places, and the boundary leaks.**
`user_profiles.location_preferences` is what the user wants; `resume_structured.location`
is where they are. The leak was found in three separate layers during design, all with
the same shape — see the first decision below.

**The structured résumé has no reconciler.** A failed background extraction leaves
`resume_structured` NULL until the user re-uploads; 37 production users are in that state
today. Anything derived from the structure inherits that hole unless it gets a reconciler
of its own.

## Goals / Non-Goals

**Goals:**

- A deterministic, dictionary-only country/region/city derivation for candidates, with a
  rule that cannot silently inherit job semantics.
- Storage that distinguishes "not known" from "stated but unresolvable", so the
  dictionary's coverage gap stays measurable on live data.
- A reconciler for the derived geography, re-runnable after any dictionary change.
- Ask every user where they are, not only those who accept office work — and pre-fill it
  from their CV so confirming is cheaper than typing.
- Revive the two hard constraints that already read a candidate country and currently
  never fire.

**Non-Goals:**

- **No reconciler for the structured résumé itself.** The systemic hole is named and left
  open; this change adds a manual run of the existing worker, not a scheduled one.
- **No re-derive or reindex of jobs.** The dictionary change is additive; existing jobs
  pick the new country names up on the next scheduled `cmd/backfill-derive`.
- **No geocoding, no LLM, no new external dependency.** The dictionary stays curated and
  silent on what it cannot resolve.
- **No candidate-geography search facet or employer-facing candidate search.** The columns
  are derived and queryable; exposing them as a product surface is separate work.
- **No change to `location.Parse` behaviour for jobs.** Only additive dictionary entries.

## Decisions

### 1. A separate `ParseResidence` entry point, not a flag on `Parse`

`internal/location` gains `ParseResidence(string) Residence{Countries, Regions, Cities}`.
It is `Parse` minus the two things true of a job and false of a person: the `WorkMode`
hint, and the `global` region.

The `global` exclusion is deliberately stated as **"a person is never located globally"**
rather than "do not inherit the bare-remote fallback". Measurement showed `global` reaches
a candidate through **two** independent paths: the fallback at `location.go:140` (a remote
marker that resolved no place) *and* the dictionary's own `"worldwide"/"anywhere"/"по
всему миру" → global` entries at `dictionaries.go:348`. A rule phrased against the
fallback alone would have let `REMOTE · WORLDWIDE` through. 4 of the 100 live rows hit
this.

*Alternatives considered.* A boolean flag on `Parse` — rejected: a flag can be defaulted
wrong at a call site, and the whole failure mode being designed against is one meaning
being used where the other was expected. A distinct return type makes that a compile
error. Post-filtering `Parse`'s output at each call site — rejected: it puts the rule in
every consumer instead of one place, which is exactly how the leak below happened.

**Why this matters beyond the parser.** The same inversion was found in three layers, and
they are one unproven seam rather than three bugs:

| Layer | The leak |
|---|---|
| `location.go:140` + `dictionaries.go:348` | a person's location expands to "everywhere" |
| `ProfileForm.svelte:263` | "where you are" is asked only of people who want office work |
| `facetModel.ts:filtersFromProfile` | "where you live" is seeded as "where you want jobs" |

### 2. Three nullable `text[]` columns on `users`, prefixed `resume_`

```sql
ALTER TABLE users
    ADD COLUMN resume_countries text[],
    ADD COLUMN resume_regions   text[],
    ADD COLUMN resume_cities    text[];
```

**Naming.** The `resume_` prefix on `users` already means "derived from the stored CV"
(`resume_structured`, `resume_structured_model`, `resume_embedding`,
`resume_ats_analysis`). Reusing it separates the **fact** from the **wish**
(`location_preferences`) with the project's existing vocabulary rather than a new one.
Bare `countries`/`regions` — mirroring `jobs` exactly — were rejected: on `users` they
would be ambiguous precisely where the fact/wish boundary must be sharpest.

**`text[]`, not JSONB.** Mirrors `jobs.countries`/`regions`, and makes the acceptance
criterion ("show the country distribution in one query") a plain `unnest` + `GROUP BY`.

**Nullable, with no `DEFAULT '{}'` — a deliberate divergence from `jobs`.** A job always
has a location string and its derivation is synchronous on ingest, so `'{}'` unambiguously
means "the dictionary was silent". A user has a third state a job does not: no CV, no
structure yet, or a stale structure. Therefore:

- `NULL` — **unknown** (no CV, no current structure, or a structure stating no location);
- `'{}'` — the CV **stated** a place and the dictionary resolved nothing.

This is the trap the task called out ("do not confuse *we don't know* with *nowhere*"), and
it buys a permanent live-data coverage metric: `count(*) WHERE resume_countries = '{}'`.
It follows the same reasoning as `jobderive.Derived.IsTech *bool` — "never coerced, so the
coverage gap stays measurable".

**`resume_cities` is the one debatable column.** It earns its place through the profile
pre-fill (`base.city` is free text and the derivation supplies a canonical name) and
because a city is the only signal for locations the country dictionary cannot place.
Without the profile work it would be premature, and it should be dropped if that work is
ever cut.

### 3. Derive synchronously inside `resume.Store.SetStructured`

`SetStructured` is the **only** writer of the structured résumé, and both producers —
the on-upload background extraction (`handler.extractStructuredResume`) and
`cmd/backfill-resume-structured` — go through it. Deriving there means:

- the geography lands in the **same** `UPDATE … WHERE id = $1 AND resume_uploaded_at = $4`,
  so the existing monotonic guard covers it for free. A separate write would have to
  duplicate the guard, and duplicated invariants drift;
- the two producers cannot diverge, because there is one path;
- there is no window in which a user has a structure but not the geography derived from it.

Background derivation was rejected: `ParseResidence` is microseconds of map lookups with
no I/O, so deferring it buys nothing and adds a state to reason about. Deriving in the
handler was rejected: it would miss the backfill worker.

`ClearUserResume` clears the three columns alongside the structure.

### 4. `cmd/backfill-resume-geo` needs only `DATABASE_URL`

The reconciler re-reads already-stored structures and re-runs the dictionary. It requires
no LLM, no object storage, and no PII service — which is the point: it is cheap enough to
re-run after **any** dictionary change, the same role `cmd/backfill-derive` plays for jobs.
Contrast `cmd/backfill-resume-structured`, which needs all three and costs an LLM call per
user.

It processes only users whose structure currently describes their stored CV; deriving from
a superseded structure would route around the staleness rule.

### 5. Read-time precedence: asserted beats derived

`buildHardConstraintInputs` takes `loc.Base.Country` when set, and falls back to the
derived country only when it is empty. Exactly one derived country is used; **more than
one yields none** rather than an arbitrary pick — an ambiguous derivation is not a fact,
and the whole dictionary discipline is never to guess.

This is a transplant of the existing `jobview.geoFacet` seam ("the dictionary wins when it
pins a place, and the LLM fills only the unpinned bucket"), not a new pattern.

The fallback is what actually revives the two dead constraints for the 80 users who have a
CV location and no stated base; un-gating the form alone would only help users who come
back and edit their profile.

### 6. Front end: un-gate `base`, and pay the coupled cost

`ProfileForm.svelte:263` currently computes
`base: wantsPhysical ? {country, city} : {}` — for a remote-only user the base is
**discarded at save time**, not merely hidden. It becomes an unconditional question,
pre-filled from the derived geography when the user has stated nothing.

That change forces a second one. `facetModel.ts:filtersFromProfile` folds `base.country`
into the seeded `countries` facet and `base.city` into `cities`. This was survivable only
because `base` was unreachable for remote-only users. Un-gate one without the other and a
remote-only candidate in Colombia gets their job search silently filtered to Colombia.
`base` therefore contributes to the seeded `countries`/`cities` only when the user accepts
physical work — where "where I live" and "where I want the job" genuinely coincide.

The gating predicate is extracted into a plain `.ts` module so it can be unit-tested;
`web/src/lib/profileFilters.test.ts` is the existing seam.

### 7. Dictionary: add 25 missing country **names** only

25 ISO codes carry a region in `countryToRegion` but have no entry in `nameToCountry`:
`ad ao bn ci cm hn kh la li ly mm mn mo mz ni ps rw sm sn sv tz ug ye zm zw`. The country
is placeable once identified, but can never identify itself — so `Honduras` and `Rwanda`
resolve to nothing.

Only **names** are added, never codes. Names are full words (`"laos"`, `"mongolia"`,
`"macao"`), so the known two-letter collisions are untouched: `resolveSubdivision` runs
before the bare-code branch in `resolveGeoToken`, and `LA`→Louisiana, `MN`→Minnesota,
`MO`→Missouri keep winning. This makes the change safe by construction rather than by
inspection.

The lasting part is the **guard test**: every code in `countryToRegion` must have a name.
The drift was silent — an unresolvable country and an out-of-scope country look identical
from the outside — so it needs a test, not review.

**One name is deliberately withheld.** `palestine` is also a city in Texas, so the bare
name would read `Palestine, TX` as the Palestinian territories; only the unambiguous long
forms are listed. This turned out to match an existing unwritten policy: `georgia` is
absent from `nameToCountry` for exactly the same reason (the US state), and
`Tbilisi, Georgia` still resolves to `ge` — through the *city*, not the country name.
The policy is now written down here and in `internal/location/AGENTS.md`.

**Adding country names is safe; adding country codes is not.** Names are full words, and
`resolveSubdivision` runs before the bare-code branch. A code would be consulted against
the US/Canada subdivision table's same-spelled entries and turn `Baton Rouge, LA` into
Laos.

## Risks / Trade-offs

- **Un-gating `base` regresses the profile→filters seed** → the coupled `facetModel.ts`
  change ships in the same change with its own test; the spec delta records the rule so it
  cannot be re-introduced later by someone reading only the old requirement.
- **The hard-constraint fallback changes match scoring for real users** → the fallback
  fires only where the field was previously empty, so it can turn a silently-skipped
  category into an evaluated one but can never override a user's own statement. The
  never-guess rule for multi-country keeps it from manufacturing evidence.
- **A wrong LLM-extracted location now has consequences** it did not have while the string
  sat unread → mitigated by the asserted-beats-derived precedence and by the profile
  pre-fill being a confirmable default rather than a silent commit. `resumeextract`'s
  schema work already removed the known systematic error here (the model filling the
  candidate's location with the last employer's office).
- **The dictionary change reaches jobs without a re-derive** → geography for existing jobs
  stays as-is until the next scheduled `cmd/backfill-derive`; the change is additive, so no
  job loses geography, some simply gain it later than CVs do. Accepted deliberately to
  avoid a 2.5M-row re-derive plus reindex, which carries its own operational hazards.
- **A reconciler for the derivative but not for its source** → geography can be rebuilt at
  will, yet the structure it derives from still dies silently on extraction failure. The
  asymmetry is recorded here and in the proposal; it is the strongest remaining argument
  for scheduling `backfill-resume-structured`, which is deliberately out of scope.
- **`resume_cities` may prove unused** if the profile pre-fill is cut → drop the column
  with the pre-fill; nothing else reads it.

## Migration Plan

1. **Apply migration `0068` before deploying the code.** It is expansive (three nullable
   columns, no rewrite, no default backfill), so it is safe to apply ahead of the deploy;
   deploying first would produce `42703` on every profile read.
2. Deploy. New uploads derive geography from that moment on.
3. `cmd/backfill-resume-structured --dry-run`, review the list, then run it — 40 users
   (37 with no structure, 3 stale). This needs `LLM_*`, `S3_*`, and `PII_FILTER_URL`.
4. `cmd/backfill-resume-geo --dry-run`, then run it for every user with a current
   structure. Needs only `DATABASE_URL`.
5. Verify: country distribution in one query, and a second `backfill-resume-geo` run that
   changes nothing.
6. **Rollback:** revert the deploy. The columns are additive and read-only to everything
   except the derivation, so they can be left in place; no data migration is undone. The
   front-end changes revert with the deploy.

**No Meilisearch reindex and no `cmd/backfill-derive` run** are part of this. Candidate
geography is a Postgres column, not a search facet.

## Open Questions

- Should the derived geography eventually become an employer-facing candidate search
  facet? Out of scope here; the columns are shaped to allow it (`text[]`, mirroring jobs)
  without committing to it.
- Should `backfill-resume-structured` gain a scheduled run so the 37-user hole cannot
  recur? Named as the systemic cause, deliberately not fixed in this change.
- **Should the country dictionary be generated rather than hand-maintained?** Measured
  during this change: `countryToRegion` covers **133** countries; GeoNames knows 244 with
  a city over 15k, and ISO 3166-1 has 252 — so **111 countries resolve to nothing**
  (Afghanistan, Cuba, Haiti, Jamaica, Syria, Madagascar, DR Congo, Namibia, … plus a long
  tail of dependencies). Deliberately deferred, for three reasons:
  1. **Regions cannot be generated.** GeoNames supplies a *continent*; this project's
     regions are a business taxonomy (`uk` split from `eu`, `cis` spanning the Caucasus
     and Central Asia, `mena` cutting across Africa and Asia). A continent→region mapping
     would misplace Turkey, Egypt, Kazakhstan, the UK, and Russia. The generator could
     supply 111 names but not one region, and a name without a region fails
     `TestDictionariesStayInVocabulary`. The valuable half of the work is human.
  2. **Wholesale import would silently break the collision policy** recorded above:
     `ga` is Gabon *and* the Georgia state code, `na` is Namibia, `ms` is Montserrat and
     Mississippi.
  3. **It would not have helped the measured data.** The 100 production CV locations named
     38 distinct countries, all already covered; none of the 8 unresolved strings fail for
     a missing country name. They fail on tokenization (`·` is not a separator, the
     LinkedIn `Greater X` prefix) and on non-US subdivisions (`subdivisionToCountry` covers
     only the US and Canada, so Indian states do not resolve). That is where the next
     coverage gain actually is.

  The shape if it is ever taken up: extend `cmd/gen-cities` into a `gen-geo` following the
  same committed-output pattern, generate names from GeoNames `countryInfo.txt`, keep
  regions curated, and let the existing guard test enumerate what a human still has to
  classify. Add an explicit denylist for the subdivision collisions.
