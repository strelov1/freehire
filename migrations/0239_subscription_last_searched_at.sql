-- When cmd/notify last ran this subscription's saved search against the index.
-- The matching stage searches a subscription only when its digest could go out
-- (see notify.searchDue), and pages back to this instant so nothing indexed since
-- is missed. NULL = never searched, which is searched on the next pass. A plain
-- nullable column: no default, no rewrite, no backfill.
ALTER TABLE subscriptions ADD COLUMN last_searched_at timestamptz;
