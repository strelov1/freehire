-- Delete the board_health rows that describe a board under a casing nothing crawls any
-- more, when the same board has a fresher row under another casing.
--
-- WHAT HAPPENED. A board id used to reach this table lowercased and now reaches it with
-- the provider's own casing — the catalogue's move out of YAML into `boards` changed where
-- the id comes from. Nothing reconciled the old rows, so one board became two records:
--
--     smartrecruiters/atlas4   failures 3  last success 2026-07-18   nothing writes this
--     smartrecruiters/ATLAS4   failures 2  last success 2026-09-16   crawled today
--
-- WHY IT IS NOT COSMETIC. The abandoned row ages, and every net that measures age reads it
-- as a board unreachable for months. Measured 2026-09-16, close-chronic-boards reported 353
-- chronic boards and 20,106 jobs it would close; 325 of those records are these twins —
-- smartrecruiters 209, ashby 99, paycom 9, workday 4, greenhouse 3, ukg 1 — whose live
-- counterparts crawled the same day. Arming that worker before this ran would have closed
-- the postings of boards that are working, and the label on the closure would have said
-- `board_unreachable` about a board reached that morning.
--
-- The report is the other casualty: a curator reading 353 lines to find the dozen real
-- failures reads none of them.
--
-- THE PREDICATE IS DELIBERATELY NARROWER THAN THE PROBLEM. It deletes a row only when all
-- of the following hold, so a board that merely looks similar to another is never touched:
--
--   * a twin exists under the same (provider, region) whose board id differs ONLY by case;
--   * the twin's own newest evidence is strictly fresher;
--   * the row being deleted has been silent for over 60 days — the same window the
--     safety-net closer uses to call a board chronic, so this removes exactly what that
--     worker would have acted on and nothing else.
--
-- A row with no fresher twin stays, whatever its casing: it is the only record of that
-- board and may be a real failure. Idempotent — a second run matches nothing.
--
-- Not a code change, because the writer is already correct: it records the id the crawl
-- used. What was wrong is history, and history is what a migration is for. The guard that
-- stops a future rename recreating this lives in ListChronicBoards.
DELETE FROM board_health AS stale
USING board_health AS live
WHERE stale.provider = live.provider
  AND stale.region = live.region
  AND lower(stale.board) = lower(live.board)
  AND stale.board <> live.board
  AND coalesce(live.last_success_at, live.last_error_at)
      > coalesce(stale.last_success_at, stale.last_error_at)
  AND coalesce(stale.last_success_at, stale.last_error_at, stale.first_seen_at)
      < now() - interval '60 days';
