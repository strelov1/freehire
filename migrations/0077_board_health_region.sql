-- board_health: add region so a provider whose board id repeats across independent regional
-- slices (Adzuna: board = "it-jobs" once per country, region = the country) gets one health row
-- per (provider, board, region) instead of every region's crawl overwriting the same row. Every
-- other provider today is region-invariant per board (region unset or a pure host-selector, e.g.
-- Lever's "eu"), so defaulting existing and future region-less rows to '' needs no backfill.
ALTER TABLE board_health ADD COLUMN region text NOT NULL DEFAULT '';
ALTER TABLE board_health DROP CONSTRAINT board_health_pkey;
ALTER TABLE board_health ADD PRIMARY KEY (provider, board, region);
