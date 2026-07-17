-- A prefix-search index for the board-tracked check in link contributions
-- (internal/contribution). external_id is "<board>:<id>", so "does this board already have
-- any crawled job" is `external_id LIKE '<board>:%'`. The default-collation
-- (source, external_id) unique index cannot serve a LIKE-prefix (nor starts_with()), so this
-- text_pattern_ops index does — turning a whole-source sequential filter (37s over greenhouse's
-- ~300k rows) into an index range scan.
--
-- On a fresh initdb volume this plain CREATE INDEX is fine; on the live prod DB it was applied
-- manually as CREATE INDEX CONCURRENTLY (the jobs table is large and serving traffic).
CREATE INDEX jobs_source_extid_pattern_idx ON public.jobs (source, external_id text_pattern_ops);
