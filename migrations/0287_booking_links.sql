-- Candidate booking links (GH #4915): a paid candidate turns on one unguessable link per
-- application, and a recruiter books a 30-minute slot through it with no account. Every
-- reference cascades and is indexed on this side (TestEveryUserForeignKeyIsIndexed).

-- btree_gist backs the EXCLUDE below (a scalar = beside a range &&). 0145 created it;
-- repeated so this file stands on its own.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- One link per application. The token is never stored: token_seed is 32 bytes from
-- crypto/rand, the token is HMAC(server key, seed), and token_hash is SHA-256 of the token,
-- the only thing the public routes look up by. A database dump alone books nothing.
-- zone is the IANA name the browser sent when the link was turned on; the default hours
-- are wall-clock times in it.
CREATE TABLE IF NOT EXISTS booking_links (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    application_id bigint      NOT NULL UNIQUE REFERENCES applications (id) ON DELETE CASCADE,
    token_seed     bytea       NOT NULL,
    token_hash     bytea       NOT NULL UNIQUE,
    zone           text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS booking_links_user_idx ON booking_links (user_id);

-- One meeting a recruiter booked through a link. user_id is the link owner's, copied so ONE
-- constraint refuses two meetings at once across all of that candidate's links. Half-open
-- '[)' bounds match slots.Interval.Overlaps, so back-to-back slots are legal. The WHERE
-- keeps a future cancel (#4919) from blocking the hour forever.
CREATE TABLE IF NOT EXISTS booking_link_bookings (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id         bigint      NOT NULL REFERENCES booking_links (id) ON DELETE CASCADE,
    user_id         bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    starts_at       timestamptz NOT NULL,
    ends_at         timestamptz NOT NULL,
    recruiter_name  text        NOT NULL,
    recruiter_email text        NOT NULL,
    status          text        NOT NULL DEFAULT 'confirmed',
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT booking_link_bookings_span_check   CHECK (ends_at > starts_at),
    CONSTRAINT booking_link_bookings_status_check CHECK (status IN ('confirmed', 'cancelled')),
    CONSTRAINT booking_link_bookings_name_check   CHECK (char_length(recruiter_name) BETWEEN 1 AND 200),
    CONSTRAINT booking_link_bookings_email_check  CHECK (char_length(recruiter_email) BETWEEN 3 AND 254),
    CONSTRAINT booking_link_bookings_no_overlap
        EXCLUDE USING gist (
            user_id WITH =,
            tstzrange(starts_at, ends_at, '[)') WITH &&
        ) WHERE (status = 'confirmed')
);

-- The owner's busy time for the slot engine, and account deletion (the EXCLUDE's index is
-- partial on status and cannot serve the cascade).
CREATE INDEX IF NOT EXISTS booking_link_bookings_user_idx ON booking_link_bookings (user_id, starts_at);
CREATE INDEX IF NOT EXISTS booking_link_bookings_link_idx ON booking_link_bookings (link_id);
