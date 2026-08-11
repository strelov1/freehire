-- Portfolio project URL on experience_employments. Import used to drop resumeextract.Project.Link;
-- the seed path needs it to reconstitute Document.Projects. Empty string means unknown — same
-- FillBlanks semantics as location/summary (fill when blank, never overwrite a non-empty value).
--
-- Expansive: the next binary reads this column on every employment list/create/update/import
-- and on CV seed. On an existing prod volume this ALTER must be applied BEFORE that deploy —
-- an unapplied column is a 42703 on those paths, not a degraded feature. A rollback leaves
-- the column harmlessly behind (DEFAULT '').
ALTER TABLE public.experience_employments
    ADD COLUMN link text DEFAULT ''::text NOT NULL;
