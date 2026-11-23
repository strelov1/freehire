-- Apply before starting the API and inbound mail worker that use this table.
CREATE TABLE public.direct_application_messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id bigint NOT NULL REFERENCES public.direct_applications(id) ON DELETE CASCADE,
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 12000),
    status text NOT NULL CHECK (status IN ('sending', 'sent', 'failed', 'received')),
    request_key uuid,
    reply_token uuid UNIQUE,
    recipient_email text,
    external_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz,
    CHECK (
        (status = 'received' AND external_id IS NOT NULL
          AND request_key IS NULL AND reply_token IS NULL AND recipient_email IS NULL)
        OR
        (status <> 'received' AND external_id IS NULL
          AND request_key IS NOT NULL AND reply_token IS NOT NULL AND recipient_email IS NOT NULL
          AND read_at IS NULL)
    )
);

CREATE INDEX direct_application_messages_history_idx
    ON public.direct_application_messages (application_id, id DESC);
CREATE UNIQUE INDEX direct_application_messages_request_idx
    ON public.direct_application_messages (application_id, request_key) WHERE request_key IS NOT NULL;
CREATE UNIQUE INDEX direct_application_messages_received_idx
    ON public.direct_application_messages (application_id, external_id) WHERE external_id IS NOT NULL;
CREATE INDEX direct_application_messages_unread_idx
    ON public.direct_application_messages (application_id, id) WHERE status = 'received' AND read_at IS NULL;
