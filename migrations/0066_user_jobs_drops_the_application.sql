-- user_jobs gives up the application columns it has stopped being read for.
--
-- This is the contract half of the expand/contract that moved the application into its own
-- table (0064). Since that cutover shipped, `applied_at`, `stage`, `notes` and
-- `followed_up_at` have been written by nothing and read by nothing here — every reader
-- moved in one pass, because there was no correct half-way state.
--
-- Dropping them IS the audit. A reader that was missed fails immediately and visibly on the
-- next request rather than quietly serving a value that stopped being updated, which is the
-- failure mode a "leave them just in case" would have produced.
--
-- What user_jobs keeps is what it was always good at: the marks a person leaves on a
-- catalogue row — viewed, saved, dismissed, voted. Those cascade away with the posting, and
-- that remains correct: what is lost is a bookmark.
--
-- ROLLBACK NOTE. Up to here, reverting the code restored the previous behaviour completely,
-- because the columns were still present and still held their values as of the cutover.
-- After this migration that is no longer true, and a rollback is a restore. That asymmetry
-- is why this is a separate, later deploy rather than a tidy-up in the same one.
--
-- Applied to a fresh volume by initdb after 0065; on an existing prod volume run this
-- manually (SET ROLE hire) AFTER the code that stopped reading these columns is live.

ALTER TABLE public.user_jobs
    DROP COLUMN IF EXISTS applied_at,
    DROP COLUMN IF EXISTS stage,
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS followed_up_at;
