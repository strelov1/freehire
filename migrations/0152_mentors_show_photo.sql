-- show_photo is the mentor's own opt-in to serve their account's stored CV headshot on
-- their public directory card and profile page. Off by default: the mentor-profile spec
-- originally deferred an avatar entirely because the account's headshot is a job-search
-- photo a mentor may not want reused here, and that concern still holds — this column is
-- what lets a mentor decide, rather than the product deciding for them.
--
-- NOT NULL DEFAULT false is additive on Postgres 11+ (a constant default needs no table
-- rewrite), so this needs no backfill step.
ALTER TABLE mentors ADD COLUMN show_photo boolean NOT NULL DEFAULT false;
