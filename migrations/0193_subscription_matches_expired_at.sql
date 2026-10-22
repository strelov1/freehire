-- migrate: no-transaction
--
-- A subscription match can now leave the pending set without being delivered: it goes
-- STALE once it is older than the notify worker's freshness window (3 days) or its job has
-- closed, and cmd/notify stamps expired_at on it instead of ever sending it.
--
-- Why a column of its own rather than reusing one of the two terminal states that exist:
--   * notified_at means "a digest the person received carried or counted this match".
--     A stale match was never shown to anyone, so stamping it notified would make the
--     ledger say the opposite of what happened.
--   * failed_at means "delivery was attempted MaxAttempts times and kept failing" — the
--     dead-letter queue an operator reads to find a broken channel. Most stale matches
--     belong to subscriptions that were never deliverable at all (push with no device,
--     an unlinked Telegram chat, a disabled webhook), and filing them as failures would
--     bury the real dead letters under tens of thousands of non-events.
--   * Deleting the row is not available either: the (subscription_id, job_id) primary key
--     is what makes matching idempotent, and a deleted match whose job is still among a
--     query's recent hits would be recorded again, fresh, and mailed.
--
-- Measured on prod 2026-10-22 before this landed: 33k pending matches on daily email
-- subscriptions (10.5k older than 7 days), 27k on push subscriptions with no registered
-- device, 7.6k on unlinked Telegram chats reaching back to 2026-09-02, 4.6k on disabled
-- webhooks. All of it sat in the pending set forever, and NotifyBacklogMetrics reported a
-- 50-day-old "oldest pending match".
--
-- Adding a nullable column with no default is a catalogue-only change.
ALTER TABLE public.subscription_matches
    ADD COLUMN IF NOT EXISTS expired_at timestamp with time zone;

-- The pending predicate gains `expired_at IS NULL`, so the partial index over it has to be
-- rebuilt or the claim's predicate would no longer imply the index's and stale rows would
-- stay in it forever. matched_at joins the key because the claim now reads a
-- subscription's pending matches NEWEST first, and the expiry pass ranges over it.
CREATE INDEX CONCURRENTLY IF NOT EXISTS subscription_matches_pending_matched_idx
    ON public.subscription_matches (subscription_id, matched_at)
    WHERE notified_at IS NULL AND failed_at IS NULL AND expired_at IS NULL;

DROP INDEX CONCURRENTLY IF EXISTS public.subscription_matches_pending_idx;
