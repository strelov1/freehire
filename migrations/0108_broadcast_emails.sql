-- broadcast_emails is the send ledger for one-off campaigns: a single letter to the
-- whole audience, sent once, on a date someone picked.
--
-- Separate from onboarding_emails (0106) rather than a fourth step there, because
-- the two answer different questions. Onboarding asks "where is this account in its
-- first fortnight" and its steps are a fixed, checked vocabulary. A campaign asks
-- "has this person been told about this event yet", and the events are not known in
-- advance — the Product Hunt launch is the first, and there is no way to enumerate
-- the rest today. So `campaign` is free text with no CHECK, deliberately.
--
-- Same idempotency rule: PK (user_id, campaign), one send per pair, ever, with the
-- row written even when the send failed. A campaign goes to the entire audience at
-- once, which makes an accidental repeat far more damaging here than in a drip —
-- and far more likely, since the natural instinct after a partial run is to re-run
-- the command.

CREATE TABLE public.broadcast_emails (
    user_id bigint NOT NULL,
    campaign text NOT NULL,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    error text NOT NULL DEFAULT '',
    CONSTRAINT broadcast_emails_pkey PRIMARY KEY (user_id, campaign),
    CONSTRAINT broadcast_emails_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES public.users(id) ON DELETE CASCADE
);

-- Campaign-wide reads ("how far did the launch mail get?") scan by campaign, which
-- the primary key cannot serve: its leading column is the user.
CREATE INDEX broadcast_emails_campaign_idx ON public.broadcast_emails (campaign, sent_at);

COMMENT ON TABLE public.broadcast_emails IS
    'Send ledger for one-off campaigns. PK (user_id, campaign) = one send per pair, ever.';
