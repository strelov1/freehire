\pset pager off
\echo '=== is pg_stat_statements available? ==='
SELECT count(*) AS has_pg_stat_statements FROM pg_extension WHERE extname = 'pg_stat_statements';

\echo ''
\echo '=== any recorded query that orders jobs by posted_at ==='
SELECT calls, round(total_exec_time)::bigint AS total_ms, left(query, 160) AS query
FROM pg_stat_statements
WHERE query ILIKE '%jobs%' AND query ILIKE '%posted_at%'
ORDER BY calls DESC
LIMIT 15;

\echo ''
\echo '=== the index right now: scans, and when postgres last reset ==='
SELECT s.indexrelname, s.idx_scan, s.idx_tup_read, s.idx_tup_fetch,
       pg_size_pretty(pg_relation_size(s.indexrelid)) AS size
FROM pg_stat_user_indexes s
WHERE s.indexrelname = 'jobs_posted_at_id_idx';

\echo ''
\echo '=== is it backing any constraint? (a PK/UNIQUE cannot just be dropped) ==='
SELECT con.conname, con.contype
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conindid
WHERE c.relname = 'jobs_posted_at_id_idx';

\echo ''
\echo '=== is it a dependency of anything (view, FK, replica identity)? ==='
SELECT deptype, refobjid::regclass AS depends_on
FROM pg_depend
WHERE objid = 'jobs_posted_at_id_idx'::regclass
   OR refobjid = 'jobs_posted_at_id_idx'::regclass;
