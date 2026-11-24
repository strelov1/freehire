-- migrate: no-transaction

-- One current consumer Checkout Session per account. Older sessions are irrelevant
-- once a user starts a new checkout. Store the ID and time only, never the checkout URL.
CREATE TABLE IF NOT EXISTS checkout_feedback_sessions (
    user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    stripe_session_id text NOT NULL,
    created_at timestamptz NOT NULL
);

ALTER TABLE cv_lifecycle_sends
    DROP CONSTRAINT IF EXISTS cv_lifecycle_sends_step_check,
    ADD CONSTRAINT cv_lifecycle_sends_step_check CHECK (step BETWEEN 1 AND 5) NOT VALID;

ALTER TABLE cv_lifecycle_sends VALIDATE CONSTRAINT cv_lifecycle_sends_step_check;

COMMENT ON TABLE cv_lifecycle_sends IS
    'Lifecycle mail ledger. One claim per (user_id, step), ever. Steps 1-3 are CV letters, step 4 is paywall and step 5 is checkout feedback.';
