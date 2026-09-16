-- Marks that this account has already received the one-time welcome email sent on its
-- first-ever transition into a paying tier (pro or ultra). See the welcome-pro-subscribers
-- change.
--
-- NULL is the ordinary state, for every account that has never paid and for every one that
-- has paid but not yet been welcomed. It is stamped once, by cmd/pro-welcome-mail, right
-- after a successful send, and never cleared — a renewal, a cancellation, or a later
-- resubscription must not re-trigger the email, because a returning subscriber is not a new
-- one.
--
-- Deliberately not derived from users.pro_until/ultra_until: those are re-derived whole on
-- every billing sync (see internal/identity/billing/AGENTS.md), so nothing about "has this
-- account been welcomed" can live there without being erased by the very same sync.
--
-- No backfill: the one subscriber that existed before this column was welcomed by hand,
-- outside this system, and is left NULL rather than backdated to a send that never happened
-- through this path.
ALTER TABLE users ADD COLUMN pro_welcome_sent_at timestamptz;

COMMENT ON COLUMN users.pro_welcome_sent_at IS
    'When the one-time welcome email for this account''s first paying tier was sent. NULL '
    'until sent; never cleared once set, so a later renewal or resubscription is not '
    're-welcomed.';

-- cmd/pro-welcome-mail's candidate query (ListNewlyPayingUsersMissingWelcomeEmail) filters
-- on pro_welcome_sent_at IS NULL AND (pro_until > now() OR ultra_until > now()), on a
-- 10-minute cadence — tighter than every other reconciling worker in the codebase. Neither
-- until column carries an index today (no existing query filters on them directly;
-- ListSubscribersNearProExpiryStripe leans on stripe_customer_id's own index instead), so
-- without one this becomes a sequential scan of the whole table every 10 minutes.
--
-- Two single-column partial indexes, not one composite: an OR across two different columns
-- is what a BitmapOr plan is for, and a composite (pro_until, ultra_until) index only helps
-- a range scan on its LEADING column — the ultra_until side of the OR would still be a
-- sequential scan through it. Partial on pro_welcome_sent_at IS NULL because that predicate
-- only ever SHRINKS relative to the table as accounts get welcomed and drop out of it,
-- unlike now()-relative predicates on pro_until/ultra_until themselves, which cannot appear
-- in a partial index at all — Postgres requires an immutable predicate, and now() is not one.
--
-- Both builds are non-concurrent, and both ignores below carry the same justification:
-- users held 2986 rows when this was written (measured on prod, not estimated), so each
-- build is milliseconds and blocking writes for it is cheaper than CONCURRENTLY's failure
-- mode — see migration 0129's own note on the same trade-off for the same table.

-- squawk-ignore require-concurrent-index-creation -- users held 2986 rows when this was written (measured on prod, not estimated); see the paragraph above.
CREATE INDEX users_pending_pro_welcome_pro_until_idx
    ON users (pro_until) WHERE pro_welcome_sent_at IS NULL;

-- squawk-ignore require-concurrent-index-creation -- users held 2986 rows when this was written (measured on prod, not estimated); see the paragraph above.
CREATE INDEX users_pending_pro_welcome_ultra_until_idx
    ON users (ultra_until) WHERE pro_welcome_sent_at IS NULL;
