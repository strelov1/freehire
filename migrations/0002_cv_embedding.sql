-- Persisted CV embedding for the /my/recommendations feed. Computed through the
-- SAME Meilisearch embedder as jobs, so the vector shares the jobs' space (see the
-- cv-recommendations change). resume_embedding_model records the embedder identity
-- that produced the vector: a model change marks the vector stale (ignored for
-- ranking, recomputed on next upload) so a CV vector is never compared against jobs
-- embedded by a different model. Only the derived vector is stored — never raw CV text.
--
-- Applied to a fresh volume by initdb after 0001; on an existing prod volume this
-- ALTER must be run manually BEFORE deploying code that reads the columns.
ALTER TABLE public.users
    ADD COLUMN resume_embedding double precision[],
    ADD COLUMN resume_embedding_model text;
