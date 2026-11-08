-- The CV lifecycle letters: up to three emails to a free user whose CV parsed, until
-- they pay (internal/engage/cvlifecycle), and the one-letter-a-day rule every optional
-- sender now shares (internal/engage/maildaily).

-- The UTC day a letter counts against. One definition, so the claim, the candidate
-- queries and the prune pass cannot disagree about where midnight is.
CREATE FUNCTION mail_today() RETURNS date
LANGUAGE sql STABLE PARALLEL SAFE
AS $$ SELECT (now() AT TIME ZONE 'UTC')::date $$;

-- Whether the stored CV was parsed into the structured form the experience bank and the
-- ATS score read: the extraction finished OK and for the CURRENT upload. The two
-- timestamps agree only when the parse is of the file that is stored now, so a re-upload
-- that has not been parsed yet is not "parsed". NULL-safe: an account with no CV is false.
CREATE FUNCTION cv_parsed(extract_status text, structured_uploaded_at timestamptz, uploaded_at timestamptz) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT coalesce(extract_status = 'ok' AND structured_uploaded_at = uploaded_at, false) $$;

-- Whether an account is in the control group that is never mailed: a stable tenth of
-- users, so the letters' effect on paying can be read against people who got none.
-- Hashed with a fixed salt, and masked rather than abs(): abs(-2^63) overflows.
CREATE FUNCTION cv_lifecycle_holdout(user_id bigint) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT (hashint8extended(user_id, 20261108) & 9223372036854775807) % 10 = 0 $$;

-- One row per account per UTC day, taken by whichever optional sender gets there first.
-- It is what turns "at most one letter a day" from a hope into a constraint: the INSERT
-- ... ON CONFLICT DO NOTHING either writes the row (send) or does not (somebody already
-- has). A definite send failure deletes the row; a crash keeps it, which fails toward
-- fewer letters.
CREATE TABLE mail_day_claims (
    user_id    bigint      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day        date        NOT NULL,
    source     text        NOT NULL,
    claimed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, day)
);

-- The prune pass walks rows by day.
CREATE INDEX mail_day_claims_day_idx ON mail_day_claims (day);

COMMENT ON TABLE mail_day_claims IS
    'The shared one-optional-letter-a-day claim. PK (user_id, day), day in UTC; source says who took it.';

-- Who is in the sequence. One row per account, ever: the upload date at enrollment is the
-- clock the offsets run from, and a later re-upload does not move it.
CREATE TABLE cv_lifecycle_enrollments (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     bigint      NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    uploaded_at timestamptz NOT NULL,
    enrolled_at timestamptz NOT NULL DEFAULT now(),
    holdout     boolean     NOT NULL,
    status      text        NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'unsubscribed', 'paid', 'entitled')),
    paid_at     timestamptz,
    exited_at   timestamptz
);

COMMENT ON TABLE cv_lifecycle_enrollments IS
    'CV lifecycle letters: one row per account. uploaded_at is the clock anchor and never moves; holdout rows are observed but never mailed.';
COMMENT ON COLUMN cv_lifecycle_enrollments.status IS
    'active = in the sequence; unsubscribed = turned the letters off; paid = a Stripe or RevenueCat tier went live; entitled = only a granted tier is live, which is not a payment.';
COMMENT ON COLUMN cv_lifecycle_enrollments.paid_at IS
    'When the paying observation first saw a Stripe or RevenueCat tier live. NULL for a granted-only account.';

-- The paying observation scans rows still inside its window and without a paid_at.
CREATE INDEX cv_lifecycle_enrollments_open_idx
    ON cv_lifecycle_enrollments (enrolled_at) WHERE paid_at IS NULL;

-- Step state: one row per (account, step), written BEFORE the send (claim-then-send), so
-- a repeated or concurrent run cannot send a step twice. A definite failure keeps the row
-- with its error and burns the step; a crash keeps it empty-errored, which also never
-- retries. Re-arming one step is a deliberate DELETE.
CREATE TABLE cv_lifecycle_sends (
    user_id bigint      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    step    smallint    NOT NULL CHECK (step BETWEEN 1 AND 3),
    sent_at timestamptz NOT NULL DEFAULT now(),
    error   text        NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, step)
);

COMMENT ON TABLE cv_lifecycle_sends IS
    'CV lifecycle letters step ledger. PK (user_id, step) = one claim per pair, ever; error is the transport failure that burned the step.';
