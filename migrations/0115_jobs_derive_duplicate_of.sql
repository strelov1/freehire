-- Derive jobs.duplicate_of from the three owned columns added in 0114.
--
-- A generated column would say this more directly and is not available: PostgreSQL will not
-- convert an existing column into one, and adding a GENERATED ... STORED column rewrites the
-- table under ACCESS EXCLUSIVE — 7.4M rows on prod. duplicate_of also has to stay a real
-- materialized column regardless, because partial index predicates read it (0012, 0042, 0107),
-- which a view or a bare expression could not carry. So a trigger it is.
--
-- PRECEDENCE: aggregator, then role, then fuzzy. This is not "most deterministic first" — it
-- reproduces which pass wins a contested row TODAY. 8,279 of 1,162,487 open marked rows on prod
-- (2026-08-19) are both aggregator-shaped and share a role_fingerprint with their canon. The
-- aggregator pass's candidate set is documented as "canonical OR already pointing at a
-- non-aggregator row", so a row the role pass pointed at a non-aggregator canon is deliberately
-- re-decided by the aggregator pass, while one pointed at another aggregator is deliberately
-- left alone. Role-first would have inverted the first case for every one of those rows — a
-- behaviour change smuggled in under a refactor. The row in the second case never enters the
-- aggregator pass's candidate set, so its aggregator column stays NULL and the COALESCE falls
-- through to role either way. Fuzzy contends with neither: it is scoped to rows the other two
-- left canonical.
--
-- UPDATE OF is the whole performance story. jobs is the hottest table in the schema and the
-- overwhelming majority of writes to it — every ingest upsert — touch no marker at all. Listing
-- the four columns makes the engine skip the trigger entirely unless one of them appears in the
-- statement's SET list, rather than paying a per-row function call to discover there is nothing
-- to do. UpsertJob names none of them, so the ingest path does not fire this.
--
-- duplicate_of is in that list on purpose: a writer that sets it directly must not be able to
-- put the row into a state the passes disagree with. The derivation simply overwrites it. That
-- is what makes ownership enforceable rather than a convention, and it is why
-- MarkJobDuplicateOfRole names an owned column instead.
CREATE OR REPLACE FUNCTION public.jobs_derive_duplicate_of() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.duplicate_of := COALESCE(
        NEW.duplicate_of_aggregator,
        NEW.duplicate_of_role,
        NEW.duplicate_of_fuzzy
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER jobs_derive_duplicate_of
    BEFORE INSERT OR UPDATE OF
        duplicate_of_aggregator, duplicate_of_role, duplicate_of_fuzzy, duplicate_of
    ON public.jobs
    FOR EACH ROW
    EXECUTE FUNCTION public.jobs_derive_duplicate_of();
