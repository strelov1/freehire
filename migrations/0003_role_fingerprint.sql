-- role_fingerprint is the repost-identity of a job: a narrow content fingerprint
-- over company + normalized title + normalized description, DELIBERATELY excluding
-- volatile fields (posted_at, url, public_slug) so a role reposted under a new
-- external_id with a refreshed posted date clusters to one fingerprint. It is the
-- input to the job-reality signal's repost/mass-posting counts (see the
-- job-reality-signal change). It is distinct from jobs.content_hash, which is the
-- incremental-index CHANGE signal and INCLUDES posted_at — the two have opposite jobs.
--
-- The index backs the two per-(company_slug, role_fingerprint) counts the reality
-- signal computes: distinct external_ids of any status (repost history) and of open
-- jobs only (concurrent mass-posting).
--
-- Applied to a fresh volume by initdb after 0002; on an existing prod volume this
-- ALTER must be run manually BEFORE deploying code that writes/reads the column, and
-- role_fingerprint backfilled for existing rows so the counts are meaningful.
ALTER TABLE public.jobs
    ADD COLUMN role_fingerprint text;

CREATE INDEX jobs_company_role_fingerprint_idx
    ON public.jobs USING btree (company_slug, role_fingerprint);
