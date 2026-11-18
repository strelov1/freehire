-- A saved search's query revision rises when its query text changes; each subscription
-- match records the revision it answered, so a match of an older query is never claimed.
-- Constant defaults make both columns catalogue-only, and every existing row agrees at 0.
-- squawk-ignore prefer-bigint-over-int -- a per-search edit counter, nowhere near 2^31.
ALTER TABLE saved_searches ADD COLUMN query_revision integer NOT NULL DEFAULT 0;
-- squawk-ignore prefer-bigint-over-int -- the same counter, copied onto a large ledger table.
ALTER TABLE subscription_matches ADD COLUMN query_revision integer NOT NULL DEFAULT 0;
