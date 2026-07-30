-- A tailored CV is a copy made FOR a vacancy; a plain CV is one the user wrote themselves, and the
-- newest plain CV is what tailoring seeds the next copy from. Until now "tailored" was inferred
-- from `job_id IS NOT NULL` — a link that cmd/prune removes on purpose. `cvs_job_id_fkey` is
-- ON DELETE SET NULL (0024), and the prune query says so outright: "Every other reference to jobs
-- cascades or nulls, so a user's saved job goes with it. That is an accepted cost of the campaign,
-- not an oversight."
--
-- So deleting one junk vacancy turned its tailored copy into a plain CV — and since the base lookup
-- takes the NEWEST plain CV, the freshly-edited orphan beat one the candidate last touched weeks
-- ago. It would then seed the next tailored copy and back the ATS delta's baseline.
--
-- The flag records what the row was CREATED as, which is the fact the foreign key destroys. It is
-- deliberately not an `is_base` flag: users may own several plain CVs (cv-builder: "create, list,
-- read, update, and delete multiple CVs"), so "the base" stays a derived notion — the most recently
-- edited non-tailored CV — and carries no uniqueness constraint.
ALTER TABLE public.cvs ADD COLUMN is_tailored boolean NOT NULL DEFAULT false;

-- Every CV still holding a vacancy link was created as a tailored copy. Rows already orphaned by a
-- prune cannot be recovered by this backfill — they have no link left to read — but production
-- carries none today (checked before writing this: 68 CVs, 8 vacancy-less, zero orphan signatures).
UPDATE public.cvs SET is_tailored = true WHERE job_id IS NOT NULL;
