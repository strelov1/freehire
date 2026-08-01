## Why

A candidate's geography is unusable today. The only place it exists is
`users.resume_structured->>'location'` — a raw free-text line the LLM copied off the
CV ("Bogotá, Colombia", "San Francisco, CA", "Remote (GMT+3)"). Nothing normalizes it,
so the candidate's country cannot be filtered, faceted, or matched against a job's
geography, even though jobs have carried derived `countries`/`regions` since `0001`.

The profile was supposed to be the other half of this, and it is quietly broken. The
profile form gates the "where you're based" sub-form on the user accepting on-site or
hybrid work, and **discards `base` at save time** for anyone who accepts only remote —
as if a remote worker had no physical location. Measured on production (2026-08-01):
of 145 profiles only **27 carry a `base.country`**, while 80 users have a location on
their CV and no base at all. Two hard-constraint checks that already exist and already
read `base.country` — "this job does not sponsor visas and is pinned to a country you
are not in" and "this on-site job is in another country" — therefore never fire for
**81% of profiles**. The logic is correct; the input was thrown away.

## What Changes

- Add `internal/location.ParseResidence` — the single entry point for candidate
  location text, deriving where a **person** is rather than where **work** is.
- Add three derived columns on `users` (`resume_countries`, `resume_regions`,
  `resume_cities`), written in the same monotonic statement that persists the
  structured résumé, so the derivation cannot drift from the structure it came from.
- Add `cmd/backfill-resume-geo`, a run-once-and-exit worker that re-derives the
  columns from already-stored structures. It needs only `DATABASE_URL` — no LLM, no
  object storage, no network — so it is safely repeatable after any dictionary change.
- Feed the derived country into hard-constraint evaluation **only where the user has
  stated nothing**: an asserted `base.country` always wins.
- Expose the derived geography read-only on `GET /api/v1/me/profile` so the profile
  form can pre-fill "where you're based" for confirmation instead of asking the user
  to type what their CV already says.
- **Un-gate `base` in the profile form** so "where I am now" is asked of everyone,
  independently of which work arrangements they accept.
- **BREAKING (behavioural, filter seeding):** seeding job-search filters from a
  profile currently folds `base.country` into the `countries` facet and `base.city`
  into `cities`. That was tolerable only while `base` was reachable exclusively by
  on-site/hybrid users. Once `base` is asked of everyone, a remote-only candidate in
  Colombia would silently get their job search filtered to Colombia. The `base`
  contribution to the seeded `countries`/`cities` facets is therefore restricted to
  users who accept physical work.
- Close a dictionary drift: 25 ISO country codes carry a region in `countryToRegion`
  but have no name in `nameToCountry` (`hn`, `rw`, `kh`, `tz`, `ug`, …), so "Honduras"
  and "Rwanda" resolve to nothing. Add the missing names and a guard test that keeps
  the two maps in step.
- Run the existing `cmd/backfill-resume-structured` against the 40 production users
  whose structure is missing (37) or stale (3). No new code — it already exists.

## Capabilities

### New Capabilities
- `candidate-geography`: deriving, storing, and backfilling where a candidate is
  located, from the free-text location line of their CV — including the rule that
  separates a person's whereabouts from a job's reach, and the precedence between
  what the candidate asserted and what was derived for them.

### Modified Capabilities
- `resume-structured-profile`: persisting the structured résumé now also derives and
  stores the candidate's geography in the same stamped, monotonic write; clearing the
  résumé clears it too.
- `hard-constraint-matching`: the location/work-authorization categories may take
  their country evidence from the derived residence when the user has asserted no
  base country, instead of skipping the category.
- `search-profiles`: `base` is reframed as a fact about the user rather than a
  preference, is asked of every user regardless of accepted work modes, and
  `GET /api/v1/me/profile` gains a read-only derived-geography block.
- `filter-modal`: the profile→filters seed no longer contributes `base.country` /
  `base.city` for users who accept only remote work.
- `job-geography`: every ISO country code the dictionary can place in a region SHALL
  also be resolvable from its country name — the invariant that was silently violated
  for 25 codes.

## Impact

**Schema.** Migration `0068` adds three nullable `text[]` columns to `users`. It is
expansive and MUST be applied before the code that reads them is deployed.

**Go.** `internal/location` (new entry point + dictionary names + guard test),
`internal/resume` (write path), `internal/db/queries/users.sql` + regenerated
`internal/db`, `internal/handler` (hard-constraint inputs, `/me/profile` response),
new `cmd/backfill-resume-geo`.

**Web.** `ProfileForm.svelte` (un-gate `base`, pre-fill from derived geography),
`facetModel.ts` (`filtersFromProfile` seeding), `types.ts` contract.

**Production operations.** Two worker runs, both dry-run first:
`cmd/backfill-resume-structured` for the 40 users without a current structure, then
`cmd/backfill-resume-geo` for everyone. No Meilisearch reindex and no job re-derive:
the dictionary change is additive, so existing jobs pick the new names up on the next
scheduled `cmd/backfill-derive`.

**Known limitation, deliberately not fixed here.** The structured résumé still has no
reconciler — a failed background extraction leaves `resume_structured` NULL until the
user re-uploads, which is why 37 users need a manual backfill run. The derived
geography *does* get a reconciler in this change, so the artifact now outlives the
thing it is derived from. That asymmetry is recorded, not resolved.
