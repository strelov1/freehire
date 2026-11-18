-- Persist consecutive explicit board-gone evidence separately from generic crawl failures.
-- This lets the safety-net close require repeated platform confirmation without treating
-- 403/429, timeouts, adapter errors, or an empty successful listing as disappearance.
ALTER TABLE board_health
    ADD COLUMN consecutive_board_gone_failures bigint NOT NULL DEFAULT 0,
    ADD COLUMN board_gone_since timestamptz;
