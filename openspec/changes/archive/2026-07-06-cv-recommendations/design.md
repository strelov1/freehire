## Context

Jobs are embedded by an in-engine Meilisearch embedder (huggingFace MiniLM,
symmetric — CV-as-text and job-as-document land in one space). The résumé
subsystem already stores the CV file in S3 (`users.resume_object_key`) and can
extract text (`pdfText` in `internal/handler/resume.go`), but deliberately does
not persist raw CV text. `PutResume` is the upload entry point. `search.Client`
owns all Meili access; `SearchParams` supports semantic queries.

The hard constraint (user-stated): the CV embedding MUST live in the same vector
space as the jobs — otherwise similarity is meaningless. Meili's in-engine
embedder does not expose a "give me the vector for this text" call, so the only
way to obtain a same-space CV vector is to have Meili embed it.

## Goals / Non-Goals

**Goals:**
- A `/my/recommendations` feed of jobs ranked by semantic similarity to the
  user's CV.
- A **persisted** CV embedding that is guaranteed same-space with the jobs.
- Reuse résumé storage + text extraction; no raw CV text persisted.
- Graceful degradation (no CV / no vector / no storage / no semantic index).

**Non-Goals:**
- Changing the swipe deck (`job-swipe`).
- Skill-based ranking (already cheap via facet filters).
- Behavior/taste vectors; CV-summary embedding to beat input truncation.

## Decisions

- **Same-space by construction (Meili read-back).** On CV upload, embed the CV
  text through Meili's *same* embedder and read the vector back: upsert a
  single-doc into a dedicated `resume_vectors` index whose embedder settings are
  identical to `jobs_semantic`, fetch it with `retrieveVectors:true`, then
  **delete the scratch doc** so no CV text persists in Meili. The returned vector
  is, by construction, in the jobs' space. (No separate model, no drift — this is
  the whole point.)
- **Persist the vector, not the text.** Store the vector on the user
  (`users.resume_embedding float8[]`) plus an **embedder identity**
  (`users.resume_embedding_model text`, e.g. the model name) so a model change
  marks the vector stale. A stale vector is ignored for ranking and recomputed on
  the next upload. This directly enforces "CV vector never compared against jobs
  from a different model."
- **Recommendations = Meili vector search.** `GET /me/recommendations` reads the
  fresh CV vector from Postgres and issues a `jobs_semantic` search with that
  raw `vector` (open jobs only), returning the standard job-view envelope. No
  per-request CV fetch/parse/embed — the persisted vector is the whole input.
- **Compute at upload time.** The embedding step hooks into `PutResume` after the
  blob is stored and text extracted; it is best-effort (a Meili/embedder failure
  leaves no vector and does not fail the upload — matches résumé-storage's
  degrade-not-error stance).
- **Degradation.** Missing CV vector, stale vector, unconfigured storage, or
  absent semantic index → the endpoint returns an empty list (not an error) and
  the page shows an upload prompt / empty state.

## Risks / Trade-offs

- **Read-back is slightly indirect.** Indexing-then-reading-a-vector is less
  obvious than a direct "embed" API, but it is the only way to get a same-space
  vector from the in-engine embedder, which is the non-negotiable requirement.
  Encapsulated behind one `search.Client` helper.
- **Input truncation.** MiniLM truncates to its max input length, so a long CV is
  embedded from its leading section only. Accepted for MVP; CV-summary embedding
  is a noted seam.
- **Coverage-dependent quality.** Ranks only against embedded jobs; improves as
  the `jobs_semantic` full build completes. Fallback keeps the feed non-erroring.
- **Migration on prod.** Adds columns to `users`; must be applied on prod before
  the deploy that reads them (per the migration convention — unapplied migration
  → read errors).
- **Model-swap recompute.** If the jobs embedder is ever switched (e.g. to the
  proxy), every stored CV vector goes stale and must be recomputed; the embedder-
  identity guard makes this safe (stale vectors are simply ignored until refreshed
  on next upload) but users must re-upload or be re-embedded to regain the feed.
