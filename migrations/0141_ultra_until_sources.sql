-- The Ultra tier's entitlement, in the shape 0135 gave the Pro one.
--
-- Three source columns, one per origin, each with exactly one writer, and an effective
-- value the schema derives as the furthest of them. That is 0135's design applied a second
-- time, and it is applied rather than reconsidered because the reason has not changed: a
-- provider reporting "no subscription" is saying *I confer nothing*, never *this account is
-- not Ultra*. With one shared column a Stripe sync would either revoke a manual grant or be
-- unable to revoke its own subscription, and before 0135 it silently did the first.
--
-- WHY NOT ONE `plan_tier` COLUMN. A tier name looks like the obvious representation and is
-- the wrong one here, for the same reason. A name has a single writer, so two providers and
-- a support grant would have to agree about who wrote it last, and "the store confers
-- nothing" would be indistinguishable from "this account is free". Keeping the answer a
-- DATE also keeps every existing reader working: the near-expiry sweep and the plan read on
-- the request path already think in instants, and a tier that worked in names would need a
-- second mechanism beside them.
--
-- No backfill and no DROP, unlike 0135. Nothing has ever written an Ultra entitlement, so
-- there is no history to separate by evidence — every column starts NULL, every account
-- resolves to the tier it already had, and the deploy changes nothing until a price is
-- configured.
ALTER TABLE users
    ADD COLUMN ultra_until_stripe timestamp with time zone,
    ADD COLUMN ultra_until_revenuecat timestamp with time zone,
    ADD COLUMN ultra_until_granted timestamp with time zone;

-- GENERATED rather than a trigger, for the reason 0135 argues at length: a trigger keeps the
-- rule out of the schema, so the next person to read `\d users` sees a plain nullable column
-- with no hint that assigning it is meaningless. A generated column makes the wrong write
-- impossible (428C9) rather than merely discouraged.
--
-- GREATEST ignores NULLs and yields NULL only when every source is NULL, which is exactly
-- "no origin confers anything".
--
-- squawk-ignore adding-field-with-default -- The rewrite and its ACCESS EXCLUSIVE lock are real and affordable, and the figure is measured rather than estimated: users held 1397 rows on prod on 2026-09-03 (see 0129 and 0135, which took the same lock on the same table for the same reason). That is milliseconds on a table nothing long-running scans, and this file adds no column to any large table.
ALTER TABLE users ADD COLUMN ultra_until timestamp with time zone GENERATED ALWAYS AS (GREATEST(ultra_until_stripe, ultra_until_revenuecat, ultra_until_granted)) STORED;

COMMENT ON COLUMN public.users.ultra_until IS
    'How far the Ultra tier reaches, derived by the schema as the furthest of '
    'ultra_until_stripe, ultra_until_revenuecat and ultra_until_granted. Refuses assignment '
    '(428C9) — write the source column of the origin that decided it. A future value here '
    'outranks pro_until: the tier is the better of the two, so that buying the more '
    'expensive plan can never give somebody less.';

COMMENT ON COLUMN public.users.ultra_until_stripe IS
    'How far the WEB Ultra subscription reaches. Written only by the Stripe sync, which '
    'tells the tiers apart by which configured price list a subscription''s price appears '
    'in. Cleared by that sync when the subscription ends, and by nothing else.';

COMMENT ON COLUMN public.users.ultra_until_revenuecat IS
    'How far an APP STORE or GOOGLE PLAY Ultra subscription reaches. No such product exists '
    'yet and the RevenueCat sync writes NULL here on every pass — deliberately, because a '
    'provider that only wrote the columns it had something to say about could never take '
    'back what it once said, which is the cancellation path.';

COMMENT ON COLUMN public.users.ultra_until_granted IS
    'Ultra GIVEN rather than sold: support''s manual grant. No provider sync touches it, '
    'which is the whole reason it is separate.';
