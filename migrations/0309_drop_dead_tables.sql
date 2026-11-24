-- Drop four tables that nothing reads or writes any more.
--
-- credit_ledger / credit_balances (0031) were the monthly AI-credit balance. 0121 replaced
-- them with usage_ledger / usage_daily and said they would be dropped in a later change;
-- the only reader left since then was account deletion, which reached them by cascade.
--
-- dictionary_gap_daily (0188) and problem_report_daily (0234) were per-account daily write
-- counters for the gap and report endpoints. #4160 moved HTTP quotas to the operations
-- gateway and removed their last writers; the rows left are stale counters (last 2026-11-04).
--
-- Archive the rows on the host before releasing (pg_dump --column-inserts -t <table>),
-- because a DROP is the one step nothing else can undo.
--
-- CASCADE is deliberately NOT used: if something still depends on a table here, the drop
-- should fail and say so rather than quietly take the dependant with it.
--
-- The rule below fires on every DROP TABLE, because dropping one breaks existing clients.
-- None of the four has had one since the changes named above; the only references removed
-- in this same change are two seed rows in a test.
-- squawk-ignore ban-drop-table
DROP TABLE IF EXISTS public.credit_ledger;
-- squawk-ignore ban-drop-table
DROP TABLE IF EXISTS public.credit_balances;
-- squawk-ignore ban-drop-table
DROP TABLE IF EXISTS public.dictionary_gap_daily;
-- squawk-ignore ban-drop-table
DROP TABLE IF EXISTS public.problem_report_daily;
