-- The notification centre was designed to hold ONE row per delivered event,
-- "independent of which channel(s) carried it" (0090's own opening comment).
-- Nothing enforced that, and the write could not honour it: the row is written
-- by notify's deliverOne, which runs once per `subscriptions` row — and that
-- table is keyed (saved_search_id, channel). A saved search with Telegram,
-- email and push enabled is three subscriptions, three deliveries, and three
-- indistinguishable `My profile — 1 new job` rows in the history, which is what
-- freehire#3020 reported. The messages themselves are correct; only the ledger
-- that was supposed to be channel-agnostic counted them.
--
-- dedup_key names the EVENT a row records, so a second channel recording the
-- same event finds the first channel's row instead of adding its own. The
-- engines that already record one row per event (reminders, nudges, auto-apply)
-- pass NULL and are untouched — hence a PARTIAL unique index, which is also why
-- a NOT NULL default was never an option here.
--
-- Measured on prod 2026-09-18 before the fix: 27,207 subscription_digest rows,
-- of which 322 are a redundant second copy of an event already recorded — 1.2%
-- of the table overall, and 100% of the history of anyone who enabled a second
-- channel. Those 322 are deliberately left alone: which of a pair to keep is a
-- coin toss, both are honest records of a message that really was delivered,
-- and a backfill that guessed would be a worse trade than a ledger that stops
-- growing wrong today.
--
-- The index is built plainly, not CONCURRENTLY, unlike 0086 and the other
-- uniqueness guards this repo adds: the table is 27,459 rows and 37 MB, so the
-- SHARE lock is held for milliseconds. CONCURRENTLY would cost two table passes
-- and, per 0086's own warning, a file that cannot be combined with the ALTER
-- above it plus a hand-run build on the existing volume — all to protect writes
-- to a table that takes a few hundred a day.

ALTER TABLE public.user_notifications
    ADD COLUMN IF NOT EXISTS dedup_key text;

-- Partial, so the engines that pass NULL are exempt rather than all colliding
-- on a single NULL key. (A plain unique index would admit them too — NULLs
-- never conflict — but it would index 27k rows to answer nothing, and would
-- claim a uniqueness rule over rows that have no key.)
-- squawk-ignore require-concurrent-index-creation -- Measured on prod 2026-09-18: 27,459 rows, 37 MB, a few hundred writes a day. The SHARE lock is held for milliseconds, and CONCURRENTLY would cost the split file plus the hand-run build 0086 documents to protect writes that are not there.
CREATE UNIQUE INDEX IF NOT EXISTS user_notifications_user_id_dedup_key_uniq_idx
    ON public.user_notifications (user_id, dedup_key)
    WHERE dedup_key IS NOT NULL;
