-- Match feed source: what a user's feed matches on. NULL saved_search_id means the profile.
-- No foreign key to saved_searches on purpose: a deleted search must stay visible to the feed
-- as search_deleted instead of silently falling back to the profile. Deleting the user deletes
-- the row; the primary key covers the foreign key.
CREATE TABLE IF NOT EXISTS feed_source (
    user_id         bigint NOT NULL PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    saved_search_id bigint
);
