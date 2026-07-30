## Why

A profile's `skills` set had no cardinality bound: the service only required it to be
non-empty and SQL only carried `cardinality(skills) > 0`, while the neighbouring
`specializations` was capped at 5 in both places. That asymmetry mattered because the set is
not inert storage. The coverage verdict expands it into one `skills != "<skill>"` AND group
per element (`search.AndNotSkills`), so a single `PUT /me/profile` of ~10^5 skills turned
every later, cheap `GET /me/resume/verdict` into a vast Meilisearch filter — against the same
index that serves public `/jobs/search`, on a route with no throttle. The 8MB body limit
admits that many distinct values with room to spare.

Two sibling entry points for the same list — the stateless `/market/coverage` and the
assistant's coverage tool — already cap a supplied list at 100, each with the reasoning
written out in a comment. The profile, the one entry point that *persists* the list and so
lets one write amplify unboundedly many reads, was the one without a cap.

The bound shipped at 100 and is being raised to 200 in the same change, because prod data
contradicted the assumption behind 100: of 131 profiles, 89% list 50 skills or fewer, but the
largest lists **90** — ten short of the bound — and the CV-autofill path unions extracted
skills into whatever the form already holds, so that user could cross it and be refused a
save. 200 bounds the filter just as effectively; the guard is against a list of 10^5, not
against a long CV.

## What Changes

- Both the wanted `skills` and the avoided `excluded_skills` sets are capped at 200 entries;
  a set past the cap is rejected with `400`, mirroring how `specializations` past its cap of 5
  is rejected.
- A single value longer than 64 characters is **dropped**, not rejected — the same treatment
  blanks and duplicates already get in that normalizer. A per-value problem does not fail an
  otherwise valid save, and the longest canonical form the skill dictionary emits is 30
  characters, so no real skill is affected.
- A SQL backstop (`cardinality(...) <= 200` on both columns) is added `NOT VALID`: it guards
  every future INSERT and UPDATE without a validation scan, so it takes no lengthy lock and
  cannot fail the migration on a row written before the application cap existed. Such a row
  can only shrink — the service normalizes every write before SQL sees it.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `search-profiles`: the skills-validation requirement gains a cardinality bound on both skill
  sets and states what happens to an over-long value. The non-empty rule is unchanged.

## Impact

- **Code:** `internal/userprofile/userprofile.go` (`maxSkills`, `maxSkillLen`,
  `normalizeSkillList`, `ErrTooManySkills`) and the error mapping in
  `internal/handler/me_profile.go`. `normalizeExcludedSkills` was renamed to
  `normalizeSkillList` — it is the shared core of both sets and now returns an error, so the
  old name no longer described it.
- **Data:** none. No existing row exceeds the bound (prod max is 90), so the constraints
  admit the whole table as it stands.
- **Migrations:** `0056_user_profile_skills_cap.sql` (adds the constraints at 100) and
  `0057_user_profile_skills_cap_200.sql` (drops and re-adds them at 200 — Postgres has no
  ALTER CONSTRAINT for a CHECK expression). Widening can never reject a row the narrower bound
  admitted, so 0057 cannot fail on existing data.
- **Risk:** a caller who legitimately holds more than 200 skills would be refused a save. The
  largest profile on prod holds 90, so the margin is a factor of two over the observed
  maximum.
