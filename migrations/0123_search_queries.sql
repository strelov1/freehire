-- What visitors search the catalogue for, so the suggestion dictionary can rank by
-- demand rather than only by supply.
--
-- The key is the NORMALISED query — the same normalisation cmd/build-suggestions
-- applies to mined posting titles (internal/search/suggest.Title) — so a typed query
-- and the title it names land on the same row. Two keys would mean the demand a
-- visitor generates never reaches the suggestion they were reaching for.
--
-- It identifies nobody: no user id, no session, no address. The table records what the
-- catalogue is ASKED FOR, not who asked. Measured over five days of production access
-- logs the site served 71,174 searches across 8,340 distinct queries, so it is small by
-- construction and needs no retention policy yet.
CREATE TABLE IF NOT EXISTS search_queries (
    query      text PRIMARY KEY,
    count      bigint      NOT NULL DEFAULT 0,
    last_seen  timestamptz NOT NULL DEFAULT now()
);

-- The builder reads this in one pass, busiest first, to join demand onto the
-- dictionary. Without the index that pass is a full sort of the table on every build.
CREATE INDEX IF NOT EXISTS search_queries_count_idx ON search_queries (count DESC);
