-- Every billing webhook we have accepted, append-only. See the add-pro-subscription
-- change.
--
-- One table doing three jobs, which is why it exists at all rather than the webhook
-- writing users.pro_until directly:
--
--   Idempotency. The provider retries a delivery it did not get a 200 for, reusing the
--   same event id, and states plainly that duplicates are possible and delivery is not
--   ordered. The unique index below is what makes a redelivery a no-op.
--
--   Retry. The handler records the event and answers 200 BEFORE trying to apply it, so a
--   provider we cannot reach at that moment costs nothing: the row stays unprocessed and
--   cmd/billing-sync picks it up. This matters more than it looks — the provider gives up
--   after five attempts over about two and a half hours, and after that this table is the
--   only remaining record that the purchase happened.
--
--   Audit. "Why is this account Pro?" is a question about money and needs an answer in
--   writing. The stored payload also makes a mapping bug replayable rather than
--   archaeological.
--
-- app_user_id is stored as it arrived AND user_id is resolved from it, because the two
-- can disagree: a TEST event from the dashboard, an event for an account since deleted,
-- or an identifier that never was one of ours all have to be recorded rather than
-- dropped. A row we cannot attribute is evidence; a row we refused to write is nothing.
-- Hence user_id is nullable, and the cascade below simply does not reach the rows that
-- belong to nobody.
--
-- payload is the whole event object, not the fields we currently read. What we read will
-- change; what arrived will not.
--
-- No CHECK on provider or event_type. The provider's event vocabulary is theirs and it
-- grows — TEMPORARY_ENTITLEMENT_GRANT and PRICE_INCREASE_CONSENT_REQUIRED were not in
-- the first version of it — and a constraint here would turn "they added an event type"
-- into a rejected delivery and a schema change. The code deliberately does not branch on
-- the type at all, so there is nothing for a constraint to protect.

CREATE TABLE public.billing_events (
    id bigint NOT NULL,
    provider text NOT NULL,
    event_id text NOT NULL,
    app_user_id text NOT NULL,
    user_id bigint,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    processed_at timestamp with time zone,
    CONSTRAINT billing_events_pkey PRIMARY KEY (id)
);

ALTER TABLE public.billing_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.billing_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

-- Deleting an account erases its billing events with the rest of its data. Rows we could
-- not attribute to a user carry NULL and are untouched, which is correct: they are not
-- anybody's personal data, and they are the only evidence that an unattributable event
-- was received at all.
ALTER TABLE ONLY public.billing_events
    ADD CONSTRAINT billing_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- The idempotency key. Scoped by provider so that a second provider's event ids cannot
-- collide with this one's — they are opaque strings from different namespaces, and
-- discovering that they overlap by silently dropping a payment is not a good way to find
-- out.
CREATE UNIQUE INDEX billing_events_provider_event_id_uniq
    ON public.billing_events (provider, event_id);

-- The reconciler's first pass: unprocessed events, oldest first. Partial, because the
-- processed rows are the overwhelming majority and none of them is ever read this way.
CREATE INDEX billing_events_unprocessed_idx
    ON public.billing_events (received_at) WHERE processed_at IS NULL;

-- The reconciler's second pass reaches users through this, and so does the audit question
-- "what happened on this account".
CREATE INDEX billing_events_user_id_received_at_idx
    ON public.billing_events (user_id, received_at DESC);
