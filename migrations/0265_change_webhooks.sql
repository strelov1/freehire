-- Push delivery of the job change feed (job_changes, migration 0263) to API customers.
--
-- One row is one subscription: a target URL, a filter, and the position in the feed it has
-- been delivered up to. Unlike webhook_configs (one unsigned URL per account, for saved-search
-- digests) an account can hold several, and each carries its own HMAC secret.
--
-- `secret` is stored as is, not hashed: signing needs the key itself. It is shown to the owner
-- once, at creation, and never returned again.
--
-- (cursor_xid, cursor_id) is the same keyset position the feed pages by: the subscription has
-- been delivered everything up to and including it. A new subscription starts at the oldest
-- transaction still in flight, so it sees changes made from now on and none of the history.
--
-- A failed delivery sets next_attempt_at by backoff and bumps consecutive_failures; past the
-- limit the subscription is disabled (disabled_at, disabled_reason). The worker also uses
-- next_attempt_at as a lease while it delivers, so two overlapping runs do not both send.
CREATE TABLE change_webhooks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    url text NOT NULL,
    secret text NOT NULL,
    -- Empty means every company. Otherwise a change is delivered only for these company slugs.
    company_slugs text[] NOT NULL DEFAULT '{}',
    kinds text[] NOT NULL DEFAULT '{opened,closed,reopened}'
        CHECK (kinds <@ ARRAY['opened', 'closed', 'reopened'] AND cardinality(kinds) > 0),
    cursor_xid bigint NOT NULL,
    cursor_id bigint NOT NULL DEFAULT 0,
    consecutive_failures bigint NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_success_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    disabled_at timestamptz,
    disabled_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Listing an account's subscriptions, and the foreign key (account deletion).
CREATE INDEX change_webhooks_user_idx ON change_webhooks (user_id);
-- The worker's pick of what is due.
CREATE INDEX change_webhooks_due_idx ON change_webhooks (next_attempt_at) WHERE disabled_at IS NULL;
