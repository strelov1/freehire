-- board_health.first_seen_at: when this (provider, board, region) row was first created.
--
-- It exists so a board that has NEVER had a successful crawl can still be measured against
-- the chronic-board window (see openspec/changes/close-chronically-unreachable-boards). For a
-- board that has succeeded at least once, last_success_at already answers "how long has it
-- been broken"; a board with last_success_at still NULL has no such anchor, since last_error_at
-- is overwritten on every failed run rather than recording the first one.
--
-- NOT NULL DEFAULT now() backfills every existing row to the migration's execution time in the
-- same statement, deliberately RESETTING the chronic clock for never-succeeded boards rather
-- than guessing a historical first-failure date we do not have (design.md Decision 2) — the
-- safe direction, since it only delays flagging a board, never wrongly closes one early. A
-- board that has already succeeded once is unaffected: it is measured from last_success_at, not
-- this column.
--
-- board_health carries one row per (provider, board, region), not per job, so this is a small
-- table; the ALTER is inexpensive regardless of whether Postgres takes the fast metadata-only
-- path for a now()-defaulted column.
ALTER TABLE public.board_health
    ADD COLUMN IF NOT EXISTS first_seen_at timestamptz NOT NULL DEFAULT now();
