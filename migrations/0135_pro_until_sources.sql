-- The plan stops being a column somebody writes and becomes one derived from its sources.
-- See the add-store-purchases change.
--
-- 0120 added users.pro_until as one nullable timestamp with one writer, and that was right
-- for one payment provider. It does not survive a second. The stores require in-app
-- purchase for anything consumed in an app, so a phone cannot use the Stripe checkout, and
-- RevenueCat becomes a second origin of the same plan. Point both at one column and they
-- erase each other: an account that bought Pro in the App Store has no Stripe subscription,
-- so the next Stripe sync derives "nothing" and writes it over what Apple was paid for.
-- Silently, and in the direction of taking away what somebody bought.
--
-- Three sources, then, each with exactly one writer, and a derived column with none:
--
--   pro_until_stripe      the web subscription, written by the Stripe sync
--   pro_until_revenuecat  the App Store or Play subscription, written by the RevenueCat sync
--   pro_until_granted     given by hand, and by add-invites later; no provider may clear it
--
-- GENERATED, not a trigger and not a rule every writer must remember. The point is not that
-- GREATEST computes the right answer — it is that the wrong answer becomes UNWRITABLE.
-- `UPDATE users SET pro_until = ...` now fails with 428C9 rather than quietly revoking a
-- subscription, so the hazard is caught by Postgres instead of by review.
--
-- GREATEST skips NULLs and returns NULL only when every argument is NULL, which is exactly
-- the semantics wanted: no source is the free plan, one source confers, two compose to the
-- longer reach. A subscriber who holds both a web and a store subscription keeps the further
-- of the two, which is the only answer that never takes away time already paid for.
--
-- Every reader is untouched. `pro_until` keeps its name, its type and its meaning, so
-- plan.TierOf, GetProUntil and /me/plan carry on reading one column, and the metered request
-- path still reads a column rather than an API.
--
-- The rewrite is cheap, and that is measured rather than assumed: 0129 recorded 1397 rows in
-- users on prod on 2026-09-03, "measured on prod, not estimated". 0120 speaks of 8 million
-- accounts three days earlier and gives no measurement; where the two disagree the measured
-- one is the one to plan against. Dropping a column and adding a STORED generated one each
-- rewrite the table, and at 1397 rows that is milliseconds — which is why the honest schema
-- is affordable here and a trigger, which would hide the rule from anyone reading \d users,
-- is not worth what it saves.
--
-- Applied to a fresh volume by initdb in name order, after 0134. On the fleet it is applied by
-- the RELEASE, not by hand: deploy/bin/release.sh builds cmd/migrate and runs it before the
-- new colour starts, so that colour never serves a request against an older schema, and a
-- migration failure aborts the release with the live colour untouched.
--
-- That leaves one gap worth naming, because it is this file's own doing. Between the migration
-- and the flip, the OLD binary is briefly serving against the NEW schema, and its only write to
-- this column — SetProUntil — now fails with 428C9. It fails CLOSED: a Stripe delivery in that
-- window goes unacknowledged and is retried, and cmd/billing-sync repeats a sync that changes
-- nothing. Nothing is written wrongly, which is the whole reason the column refuses assignment
-- rather than accepting a value nobody meant.
--
-- BEFORE the release, look at the accounts the split cannot decide from evidence alone:
--
--   SELECT id, pro_until FROM users WHERE pro_until IS NOT NULL AND stripe_customer_id IS NOT NULL;
--
-- Those are Stripe customers, so their value goes to pro_until_stripe — correct unless support
-- hand-set a longer expiry on top of a real subscription, in which case the next sync shortens
-- it back. Nothing in the data distinguishes the two cases. At the measured 1397 accounts this
-- is a handful of rows a person can read; move any of them to pro_until_granted afterwards.
ALTER TABLE users
    ADD COLUMN pro_until_stripe timestamp with time zone,
    ADD COLUMN pro_until_revenuecat timestamp with time zone,
    ADD COLUMN pro_until_granted timestamp with time zone;

-- Existing values are separated by the only evidence of where they came from.
--
-- Today's non-NULL values have two origins mixed together: add-plan-limits shipped the
-- column as hand-set, and add-pro-subscription then pointed the Stripe sync at it. An
-- account holding a stripe_customer_id (0129) transacted with Stripe and its value belongs
-- where a later cancellation can revoke it. An account without one was set by hand and
-- belongs where no provider will quietly undo it.
--
-- Both errors here are silent, which is why the split is by evidence rather than by guess:
-- everything into _stripe lets the next sync revoke support's manual grants, and everything
-- into _granted makes today's subscribers permanently Pro, cancellation included.
UPDATE users SET pro_until_stripe = pro_until
    WHERE pro_until IS NOT NULL AND stripe_customer_id IS NOT NULL;

UPDATE users SET pro_until_granted = pro_until
    WHERE pro_until IS NOT NULL AND stripe_customer_id IS NULL;

-- Dropped and re-added rather than altered: a plain column cannot be converted to a
-- generated one in place. The two statements are in one file, and the runner wraps this file
-- in a transaction, so there is no instant at which pro_until is missing from a committed
-- state.

-- squawk-ignore ban-drop-column -- The column is re-added two statements down with the same name, type and meaning, inside the same transaction, so no client ever observes it missing and every reader keeps working. What a client loses is the ability to WRITE it, and that is this migration's entire purpose rather than a casualty of it: the one writer in the codebase, SetProUntil, is replaced in the same change, and a hand-run UPDATE now fails with 428C9 instead of silently revoking a subscription somebody paid for. The deploy ordering the header states — migrate first, then deploy — is what keeps the gap safe in the meantime.
ALTER TABLE users DROP COLUMN pro_until;

-- squawk-ignore adding-field-with-default -- The rewrite and its ACCESS EXCLUSIVE lock are real and affordable: users held 1397 rows on prod on 2026-09-03, measured rather than estimated (see 0129), so this is milliseconds on a table nothing long-running scans. The rule's suggested alternative is a trigger, which is exactly what the design considered and rejected: a trigger keeps the rule out of the schema, so the next person to read \d users sees a plain nullable column and no hint that assigning it is meaningless. The generated column is what makes the wrong write impossible rather than merely discouraged, and that guarantee is the point of the change.
ALTER TABLE users ADD COLUMN pro_until timestamp with time zone GENERATED ALWAYS AS (GREATEST(pro_until_stripe, pro_until_revenuecat, pro_until_granted)) STORED;
