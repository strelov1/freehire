-- CV ids become random UUIDs (see the opaque-cv-ids change).
--
-- A CV was addressed by a bigint identity — in /my/cvs/<id>, in nine API paths,
-- and in two published clients. Ownership already confined every read (a foreign
-- id answers 404 exactly as a missing one does), so the number was not a
-- capability. It still published how many CVs the platform holds, and it meant a
-- single missing owner check on any future endpoint would be walkable — over
-- résumés, the most sensitive data here. A random id does not replace the
-- ownership check; it makes forgetting one survivable.
--
-- The swap is a primary-key change with two foreign keys hanging off it:
-- referral_requests.cv_id (the CV attached to a referral, ON DELETE SET NULL) and
-- assistant_sessions.cv_id (the CV a tailoring conversation is bound to, ON
-- DELETE CASCADE). Both are converted here, in the same transaction, by mapping
-- through the OLD key before it is dropped — so a failure anywhere leaves the
-- schema exactly as it was.
--
-- The old numeric ids are NOT retained. Nothing outside the database stores one
-- except the CLI and the MCP server, which are released alongside this change and
-- refuse a number afterwards.
--
-- Applied to a fresh volume by initdb after 0044; on an existing prod volume run
-- this file manually (SET ROLE hire) BEFORE deploying code that reads it.

BEGIN;

-- 1. The new key, generated for every existing row.
ALTER TABLE public.cvs ADD COLUMN new_id uuid DEFAULT gen_random_uuid() NOT NULL;

-- 2. Carry each reference across while the old key still exists to map through.
ALTER TABLE public.referral_requests ADD COLUMN new_cv_id uuid;
UPDATE public.referral_requests r SET new_cv_id = c.new_id FROM public.cvs c WHERE c.id = r.cv_id;

ALTER TABLE public.assistant_sessions ADD COLUMN new_cv_id uuid;
UPDATE public.assistant_sessions s SET new_cv_id = c.new_id FROM public.cvs c WHERE c.id = s.cv_id;

-- 3. Retire the old references.
ALTER TABLE public.referral_requests DROP CONSTRAINT referral_requests_cv_id_fkey;
ALTER TABLE public.referral_requests DROP COLUMN cv_id;
ALTER TABLE public.referral_requests RENAME COLUMN new_cv_id TO cv_id;

ALTER TABLE public.assistant_sessions DROP CONSTRAINT assistant_sessions_cv_id_fkey;
ALTER TABLE public.assistant_sessions DROP COLUMN cv_id;
ALTER TABLE public.assistant_sessions RENAME COLUMN new_cv_id TO cv_id;

-- 4. Swap the primary key itself. Dropping the column takes its identity sequence
--    with it, which is the point: there is no counter left to read.
ALTER TABLE public.cvs DROP CONSTRAINT cvs_pkey;
ALTER TABLE public.cvs DROP COLUMN id;
ALTER TABLE public.cvs RENAME COLUMN new_id TO id;
ALTER TABLE public.cvs ADD CONSTRAINT cvs_pkey PRIMARY KEY (id);

-- 5. Re-establish what the dropped column took with it. DROP COLUMN silently drops
--    every CHECK that mentions the column, so referral_requests' cv_kind rule —
--    "an original CV never carries a cv_id" — vanished with the old cv_id. The
--    foreign keys were dropped explicitly above; this one goes quietly, which is
--    exactly why it needs restating. (Caught by the integration suite, not by
--    reading the DDL.)
ALTER TABLE public.referral_requests
    ADD CONSTRAINT referral_requests_cv_kind_check CHECK (
        ((cv_kind = 'original'::text) AND (cv_id IS NULL)) OR
        (cv_kind = 'built'::text)
    );

-- 6. Re-establish the references against the new key, with the same delete rules.
ALTER TABLE public.referral_requests
    ADD CONSTRAINT referral_requests_cv_id_fkey FOREIGN KEY (cv_id) REFERENCES public.cvs(id) ON DELETE SET NULL;

ALTER TABLE public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_cv_id_fkey FOREIGN KEY (cv_id) REFERENCES public.cvs(id) ON DELETE CASCADE;

-- 7. A referencing column needs its own index: Postgres indexes only the referenced
-- side, so without these, deleting a CV scans both tables.
CREATE INDEX IF NOT EXISTS referral_requests_cv_id_idx ON public.referral_requests (cv_id);
CREATE INDEX IF NOT EXISTS assistant_sessions_cv_id_idx ON public.assistant_sessions (cv_id);

COMMIT;
