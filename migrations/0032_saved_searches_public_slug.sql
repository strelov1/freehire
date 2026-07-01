-- Public saved-search "boards": a signed-in user can publish one of their saved
-- searches under a stable, shareable slug. `public_slug IS NULL` means the set is
-- private (the default, matching every existing row); a non-NULL slug means it is
-- shared and readable by anyone at GET /api/v1/boards/<slug> and /b/<slug> in the
-- web app. `author_label` is an optional, self-contained attribution string shown on
-- the public board (empty/NULL renders anonymously) — deliberately NOT derived from
-- users.email, so publishing exposes no account PII. Both columns are additive and
-- nullable, so this is safe to apply to an existing volume/prod manually (the
-- versioned-migration-runner seam from AGENT.md remains open). Applied automatically
-- by Postgres on first volume init and also serves as schema source for sqlc.

ALTER TABLE saved_searches
    ADD COLUMN IF NOT EXISTS public_slug  TEXT,
    ADD COLUMN IF NOT EXISTS author_label TEXT;

-- One slug ⇒ one board. A plain UNIQUE tolerates many NULLs in Postgres, so private
-- (unshared) rows are unconstrained; only published slugs must be distinct. This index
-- also backs the public read GET /api/v1/boards/<slug>.
CREATE UNIQUE INDEX IF NOT EXISTS saved_searches_public_slug_idx
    ON saved_searches (public_slug);
