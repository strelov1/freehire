-- Contribution review queue: a contributed link that resolves to no supported board is no
-- longer rejected. When it is a well-formed http(s) URL we record it here for manual review
-- (status 'review', source/board unset) so a maintainer can check whether the source is
-- ingestable — no AI credit is awarded until that manual check promotes the row.
--
-- Three schema changes:
--   1. source/board become nullable — a review row has neither until a maintainer resolves it.
--   2. the status CHECK gains 'review'.
--   3. a partial unique index dedups the review queue by url (the existing UNIQUE (source,
--      board) can't: NULLs are distinct, so it would allow the same url many times over).
--
-- Applied to a fresh volume by initdb after 0036; on an existing prod volume run these
-- statements manually (SET ROLE hire) BEFORE deploying code that writes review rows.

ALTER TABLE public.link_contributions ALTER COLUMN source DROP NOT NULL;
ALTER TABLE public.link_contributions ALTER COLUMN board DROP NOT NULL;

ALTER TABLE public.link_contributions
    DROP CONSTRAINT link_contributions_status_check;
ALTER TABLE public.link_contributions
    ADD CONSTRAINT link_contributions_status_check
    CHECK ((status = ANY (ARRAY['pending'::text, 'onboarded'::text, 'rejected'::text, 'review'::text])));

-- Review-queue dedup: a given unrecognized url can sit in the queue at most once. Scoped to
-- review rows (source IS NULL) so recognized boards keep using UNIQUE (source, board).
CREATE UNIQUE INDEX link_contributions_review_url_key
    ON public.link_contributions (url) WHERE source IS NULL;
