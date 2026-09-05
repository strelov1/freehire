-- Discounts on the Pro subscription: the codes an operator hands out, and the invite
-- links subscribers hand out for us.
--
-- Everything here exists because THIS REPOSITORY IS PUBLIC. A discount whose codes,
-- seat limits or reward amounts can be read out of the source is a discount that gets
-- drained the day somebody greps it, so the source carries only the rules for reading
-- these tables and never a value they hold. A launch offer is a row, not a constant --
-- including its spelling, which is why no document in this repository names one.
--
-- Four tables and one shape: a discount on an account's next invoice. Two of them are
-- offers (promo_codes, invite_codes), two are what happened (promo_redemptions,
-- invite_rewards).

-- The offers an operator creates by hand. There is no admin UI and no code path that
-- writes here — an INSERT is the interface, which is also what makes `active = false`
-- a rollback that needs no deploy.
CREATE TABLE public.promo_codes (
    -- Upper case is enforced rather than assumed, because the redemption path folds the
    -- caller's input up before looking it up. A row inserted in lower case would be a
    -- code nobody could ever redeem, and the failure would look exactly like a typo on
    -- the buyer's side. The shape also bounds what a guesser has to work through; the
    -- rate limit on the preview route is the other half of that.
    -- The 'ZZ' exclusion is what makes a guard elsewhere honest rather than decorative.
    -- A test walks the discount sources for anything this constraint would accept and
    -- fails the build, exempting only literals with that prefix so that FIXTURES can name
    -- codes. An exemption the database did not also refuse would be a way to write a real
    -- code past the guard; here, a code nobody can ever create.
    code         text        PRIMARY KEY
                             CHECK (code = upper(code)
                                    AND code ~ '^[A-Z0-9]{4,32}$'
                                    AND code NOT LIKE 'ZZ%'),

    -- A percentage and never an amount. An amount off would have to name a currency and
    -- then agree with a price it cannot see; a percentage is right whatever the price
    -- becomes. The referral reward is the one amount-denominated discount in this file,
    -- and it is computed from the price at the moment it is earned.
    -- squawk-ignore prefer-bigint-over-smallint -- a percentage, bounded 1..100 by the check
    percent_off  smallint    NOT NULL CHECK (percent_off BETWEEN 1 AND 100),

    -- NULL means unlimited seats. A zero would be an offer nobody can take, which is
    -- what `active = false` already says more clearly.
    -- squawk-ignore prefer-bigint-over-int -- a seat count for one launch offer
    max_uses     integer     CHECK (max_uses IS NULL OR max_uses > 0),
    -- squawk-ignore prefer-bigint-over-int -- counts against max_uses, must not be wider
    uses         integer     NOT NULL DEFAULT 0 CHECK (uses >= 0),

    expires_at   timestamptz,
    active       boolean     NOT NULL DEFAULT true,

    -- Why this code exists, for whoever reads the table in six months. Not shown to
    -- anyone: the redemption path returns a percentage and a refusal, never this.
    note         text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE public.promo_codes IS
    'Operator-created discount codes. Written by INSERT only — no code path creates a '
    'row, and a test fails the build if a redeemable code appears in the repository. '
    'Seats are claimed by the same UPDATE that tests them, so two accounts cannot both '
    'take the last one. Setting active = false stops new redemptions without a deploy.';

-- Who redeemed what. Keyed on the ACCOUNT and not on (code, account): an account
-- redeems one code in its lifetime, full stop.
--
-- That is stricter than "one use per code per person" and deliberately so. Stripe
-- admits a single coupon per checkout session, so two offers held at once cannot both
-- be honoured anyway — and the version of this table keyed on the pair would let an
-- account collect offers it can never spend, then discover that at the till.
CREATE TABLE public.promo_redemptions (
    user_id     bigint      PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    code        text        NOT NULL REFERENCES public.promo_codes(code),
    redeemed_at timestamptz NOT NULL DEFAULT now()
);

-- The unindexed side of the foreign key. Without it, deactivating or reworking a code
-- takes a sequential scan of every redemption ever made, and "how did that offer do?" —
-- the one question an operator actually asks of this table — has no index to answer it.
CREATE INDEX promo_redemptions_code_idx ON public.promo_redemptions (code);

COMMENT ON TABLE public.promo_redemptions IS
    'One row per account, ever: user_id is the primary key, so an account redeems at '
    'most one promo code in its lifetime. This is what stops two percentage discounts '
    'stacking on a subscription that only admits one coupon per checkout session.';

-- Each account's own invite link.
--
-- Minted on first ask rather than at registration: most accounts never open the invite
-- page, and a row per account would be a table the size of `users` that nothing reads.
CREATE TABLE public.invite_codes (
    user_id    bigint      PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,

    -- Long and random, from crypto/rand. This code appears in a URL people paste into
    -- chats, so it is a public identifier for an account — which is exactly why it must
    -- not be derived from the account id or from anything else about the person. Short
    -- enough to guess would make the invite link an account enumerator.
    code       text        NOT NULL UNIQUE CHECK (code ~ '^[A-Za-z0-9_-]{16,64}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE public.invite_codes IS
    'One invite code per account, minted from crypto/rand the first time the account '
    'asks for its link, and never rotated. Uniqueness is the constraint here rather '
    'than a read-then-write, because a read-then-write is a race.';

-- Who brought whom, and what came of it.
--
-- A row is written when an attributed account REGISTERS, so the invitee's own discount
-- has something to read; it becomes `granted` only once that invitee has an invoice
-- that actually collected money. The gap between the two is the whole anti-abuse
-- design: attribution is a cookie the visitor controls and is therefore free to forge,
-- while granting requires a real payment, which is not.
CREATE TABLE public.invite_rewards (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    referrer_id  bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,

    -- UNIQUE, and that is the anti-abuse rule rather than a tidiness one: an account can
    -- be the subject of exactly one reward for its whole life, however many times it is
    -- attributed and however many codes it carries. A check running before the insert
    -- would be a race; a constraint is not.
    referee_id   bigint      NOT NULL UNIQUE REFERENCES public.users(id) ON DELETE CASCADE,

    status       text        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'granted')),

    -- Fixed at the moment of granting, from the list price then. A later price change
    -- must not revalue credit somebody has already earned — in either direction.
    amount_cents bigint      NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),

    -- Two stamps, because earning and receiving are different events that can be far
    -- apart. A referrer who has never bought anything has no provider customer to
    -- credit, so their reward sits granted-but-undelivered until their own first
    -- checkout consumes it.
    granted_at   timestamptz,
    delivered_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT invite_rewards_not_self CHECK (referrer_id <> referee_id),

    -- The stamps and the status are one fact written two ways, and the check keeps them
    -- from drifting: nothing is delivered before it is granted, and nothing is granted
    -- without a time and an amount.
    CONSTRAINT invite_rewards_status_stamps CHECK (
        (status = 'pending' AND granted_at IS NULL AND delivered_at IS NULL)
        OR (status = 'granted' AND granted_at IS NOT NULL AND amount_cents > 0)
    )
);

-- The referrer's own page reads this, and so does the ceiling check on every grant.
CREATE INDEX invite_rewards_referrer_idx ON public.invite_rewards (referrer_id);

-- The worker's two passes, each given only the rows it wants. Both are partial because
-- the interesting rows are a vanishing fraction of the table: almost every attributed
-- signup never pays, and almost every granted reward is delivered within the hour.
CREATE INDEX invite_rewards_pending_idx ON public.invite_rewards (referee_id)
    WHERE status = 'pending';
CREATE INDEX invite_rewards_undelivered_idx ON public.invite_rewards (referrer_id)
    WHERE status = 'granted' AND delivered_at IS NULL;

COMMENT ON TABLE public.invite_rewards IS
    'Who invited whom, and whether it earned anything. Written pending at the invitee''s '
    'registration; moved to granted by cmd/billing-sync only once one of that invitee''s '
    'invoices collected a non-zero amount — an active subscription that collected nothing '
    'never grants. referee_id is UNIQUE, so an account is worth one reward for life.';
