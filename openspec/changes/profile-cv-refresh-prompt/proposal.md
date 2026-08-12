## Why

Editing the experience bank (on profile or in the tailor workspace) updates the seed
source of truth but leaves open base and tailored CVs unchanged until the candidate
explicitly hits Reset from résumé. That disconnect is easy to miss: they just added a
role or achievement and the CV they are looking at still shows the old content. After a
successful bank edit, the product should ask whether to refresh the CV from the current
seed, and only rewrite when they agree.

## What Changes

- After a successful experience-bank mutation (create/update/delete of an employment or
  atom, including merges), the web UI SHALL offer a confirm prompt to refresh CV content
  from the current seed.
- **On the tailor workspace:** if they agree, call the existing
  `POST /api/v1/me/cvs/:id/reset-from-resume` for the open tailored CV (same path as
  History → Reset Changes).
- **On `/my/profile` (Experience tab):** if they agree, refresh the base CV from the
  current seed. Expose a cookie-only base reseed endpoint if Reset remains tailored-only
  (today base refresh is only a side effect of tailored reset).
- Dismiss / decline MUST leave documents untouched (no silent rewrite).
- Session-scoped dismiss so repeated edits in one sitting are not nagged after a "No"
  (optional but preferred).
- **Out of scope this change:** profile Skills tab / `userprofile.skills` edits (those
  feed search/match, not the CV document); auto-rewriting every tailored copy for every
  job; inventing a second apply path besides seed → Reset/reseed.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `experience-bank`: after a successful bank mutation in the web UI, the product SHALL
  offer to refresh CV content from the seed when the candidate agrees.
- `cv-tailoring`: agreeing to refresh from a tailor context SHALL run reset-from-résumé
  on the open tailored CV; agreeing from profile SHALL reseed the base CV from the same
  seed composition Reset uses.

## Impact

- Web: `ExperienceBankView` (and/or its callers on `/my/profile` and `/tailor/[slug]`),
  confirm UX aligned with existing Reset copy.
- Go (if needed): thin `POST` to reseed base only when profile agrees and no tailored
  context exists — reuses `reseedBaseFromSeed` / `applySeedContent`.
- No change to bank write APIs themselves; prompt is client-orchestrated after success.
- Respects `reset-preserve-seed-sections` keep-if-empty behaviour on reset/reseed.
