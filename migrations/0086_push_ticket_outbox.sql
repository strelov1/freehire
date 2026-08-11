-- push_ticket_outbox queues Expo push-send tickets for a later receipt check.
-- Expo's send response is a ticket, not a final delivery outcome — a token
-- that just went dead (fresh uninstall, revoked permission) is only
-- discoverable later via Expo's getReceipts endpoint, once Expo has actually
-- attempted delivery through APNs/FCM. cmd/push-receipts polls this queue on
-- a schedule. No FK to users: a token may already be gone by check time
-- (unregistered, or its owning row already reassigned/deleted), and pruning
-- is idempotent by token value regardless. See the push-notification-infra
-- change, design.md Decision 2.

CREATE TABLE public.push_ticket_outbox (
    id bigint NOT NULL,
    token text NOT NULL,
    ticket_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.push_ticket_outbox ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.push_ticket_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.push_ticket_outbox
    ADD CONSTRAINT push_ticket_outbox_pkey PRIMARY KEY (id);

-- The claim scan: oldest-first, filtered to tickets past the minimum wait.
CREATE INDEX push_ticket_outbox_created_at_idx
    ON public.push_ticket_outbox (created_at);
