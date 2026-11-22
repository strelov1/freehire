-- Match feed watermark (story: new first, per source): the instant up to which a user has paged
-- through the "new" cards of one feed source. seen_at is whole seconds chosen by the server and
-- only ever moves forward. Deleting the user deletes the row; the primary key's leading
-- user_id column covers the foreign key.
CREATE TABLE IF NOT EXISTS feed_state (
    user_id bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source  text        NOT NULL,
    seen_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, source)
);
