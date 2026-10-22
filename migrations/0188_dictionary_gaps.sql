CREATE TABLE dictionary_gaps (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind               text NOT NULL CHECK (kind IN ('specialization', 'skill')),
    raw_term           text NOT NULL,
    normalized_term    text NOT NULL,
    -- squawk-ignore prefer-bigint-over-int -- counts asks for one term; it never nears 2^31
    request_count      integer NOT NULL DEFAULT 1 CHECK (request_count > 0),
    first_requested_at timestamptz NOT NULL DEFAULT now(),
    last_requested_at  timestamptz NOT NULL DEFAULT now(),
    submitted_by       bigint NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    status             text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    resolution_note    text NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX dictionary_gaps_live_key
    ON dictionary_gaps (kind, normalized_term)
    WHERE status IN ('pending', 'accepted');

CREATE INDEX dictionary_gaps_submitted_by_idx ON dictionary_gaps (submitted_by);

CREATE TABLE dictionary_gap_daily (
    user_id bigint NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    day     date NOT NULL,
    -- squawk-ignore prefer-bigint-over-int -- capped at 20 per user per day
    writes  integer NOT NULL CHECK (writes > 0),
    PRIMARY KEY (user_id, day)
);
