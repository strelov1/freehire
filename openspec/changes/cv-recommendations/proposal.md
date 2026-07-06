## Why

Skill-level matching is already cheap and exact via facet filters, so it is not
where embeddings add value. A user's CV carries **unstructured semantic signal**
(experience context, phrasing, domains) that a skill filter cannot capture. With
the `jobs_semantic` index in place, we can recommend jobs by semantic similarity
to the user's CV — a genuinely more capable feature than skill matching.

## What Changes

- Add a dedicated **`/my/recommendations`** page (its own tab/nav entry) showing
  jobs ranked by semantic similarity to the signed-in user's CV. The swipe deck
  is **not** changed.
- Add `GET /api/v1/me/recommendations`: rank `jobs_semantic` by the caller's
  **persisted CV embedding** (a vector search), returning the standard list
  envelope of job views.
- **Persist the CV embedding** on the user, computed **through Meili's own
  embedder** so it lives in the exact same vector space as the job embeddings
  (see design: the CV text is embedded by the same in-engine model and the vector
  is read back and stored). Store the embedder identity alongside it so a model
  change marks the vector stale for recompute — the CV vector is never compared
  against jobs embedded by a different model.
- Compute/refresh the CV embedding when a user **uploads or replaces** their CV
  (hook into the existing résumé upload). No raw CV text is persisted (only the
  derived vector + the S3 blob that résumé-storage already keeps).
- **Graceful degradation:** no CV, no persisted vector yet, unconfigured object
  storage, or unavailable semantic index → the page shows an appropriate empty/
  prompt state and the endpoint returns an empty (not error) result.

## Capabilities

### New Capabilities

- `cv-recommendations`: a signed-in user gets a `/my/recommendations` feed of
  jobs ranked by semantic similarity between their CV and the job catalogue,
  backed by a persisted CV embedding kept in the same vector space as the jobs.

### Modified Capabilities

<!-- none: the swipe deck (job-swipe) is intentionally left unchanged -->

## Impact

- Schema: a persisted CV embedding on `users` (vector + embedder identity) —
  needs a migration (apply on prod before deploy per the migration convention).
- Code: new recommendations handler + route; a CV-embedding step wired into the
  existing résumé upload (`PutResume`), reusing `pdfText` and the object store;
  a Meili "embed-and-read-back" helper in `internal/search` that guarantees the
  CV vector matches the jobs' embedder; a vector search over `jobs_semantic`.
- Frontend: a new `/my/recommendations` SvelteKit page + nav entry + API client
  method.
- Depends on: `jobs_semantic` (full build in progress), résumé storage (S3,
  configured in prod), and the shared embedder settings.
- Out of scope (seams): behavior-based taste vectors, profile-skill semantic
  ranking of the swipe deck, CV-section/summary embedding to beat the model's
  input-length truncation.
