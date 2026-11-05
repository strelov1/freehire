-- A canonical posting's search document carries the geography of every open duplicate it
-- represents (DuplicateClosureGeoFor) and its role cluster's repost/mass counts. Both move
-- when a DUPLICATE changes — it joins or leaves the cluster, closes or reopens, or its own
-- geography is rewritten — and none of those writes queued the owner: ingest never queues a
-- duplicate, and the marker passes queue only the row whose marker flipped. So the owner's
-- document caught up only at the next full rebuild, which is what kept that rebuild daily.
--
-- A trigger rather than a statement in each writer: closures alone come from a whole family
-- of queries (TestEveryClosingQueryQueuesTheRemoval), plus ingest and three marker passes.
--
-- FOR EACH ROW with a WHEN filter, so an update that touches none of these columns — the
-- overwhelming majority, every liveness touch included — costs one comparison and no call.
-- duplicate_of chains (up to 8 deep, the same bound DuplicateClosureGeoFor walks), so the
-- walk climbs to the canonical root; only an open, public root is queued.
CREATE OR REPLACE FUNCTION public.jobs_enqueue_duplicate_owner() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    owners bigint[];
BEGIN
    IF TG_OP = 'INSERT' THEN
        owners := ARRAY[NEW.duplicate_of];
    ELSE
        owners := ARRAY[OLD.duplicate_of, NEW.duplicate_of];
    END IF;

    WITH RECURSIVE up AS (
        SELECT j.id, j.duplicate_of, j.closed_at, j.is_private, j.posted_at, j.created_at, 0 AS depth
        FROM jobs j
        WHERE j.id = ANY(owners)
        UNION ALL
        SELECT j.id, j.duplicate_of, j.closed_at, j.is_private, j.posted_at, j.created_at, up.depth + 1
        FROM up
        JOIN jobs j ON j.id = up.duplicate_of
        WHERE up.depth < 8
    )
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT DISTINCT id, COALESCE(posted_at, created_at)
    FROM up
    WHERE duplicate_of IS NULL AND closed_at IS NULL AND NOT is_private
    -- Re-stamped, not DO NOTHING: the owner's entry may be claimed by a drain wave that read
    -- the row before this change, and that wave's completion deletes the entry by id.
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL;

    RETURN NULL;
END;
$$;

CREATE TRIGGER jobs_enqueue_duplicate_owner_ins
    AFTER INSERT ON public.jobs
    FOR EACH ROW
    WHEN (NEW.duplicate_of IS NOT NULL)
    EXECUTE FUNCTION public.jobs_enqueue_duplicate_owner();

CREATE TRIGGER jobs_enqueue_duplicate_owner_upd
    AFTER UPDATE ON public.jobs
    FOR EACH ROW
    WHEN (
        OLD.duplicate_of IS DISTINCT FROM NEW.duplicate_of
        OR (NEW.duplicate_of IS NOT NULL AND (
            (OLD.closed_at IS NULL) <> (NEW.closed_at IS NULL)
            OR OLD.countries IS DISTINCT FROM NEW.countries
            OR OLD.regions IS DISTINCT FROM NEW.regions
            OR OLD.cities IS DISTINCT FROM NEW.cities
            OR OLD.metros IS DISTINCT FROM NEW.metros
        ))
    )
    EXECUTE FUNCTION public.jobs_enqueue_duplicate_owner();
