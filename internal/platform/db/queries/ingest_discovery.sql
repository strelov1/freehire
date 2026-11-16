-- name: AdoptDiscoveredIngestProvider :exec
-- A genuinely new native provider joins the existing scheduler. An existing row
-- belongs to the operator: do not unpause it, change cadence, or take over a static
-- provider still outside the managed rollout.
INSERT INTO ingest_schedule (provider, shards, cadence_sec, timeout_sec, enabled, managed, notes)
SELECT sqlc.arg(provider), sqlc.arg(default_shards), sqlc.arg(default_cadence_sec),
       sqlc.arg(default_timeout_sec), true, true, 'Native ATS discovered from an external posting'
WHERE NOT EXISTS (SELECT 1 FROM boards WHERE provider = sqlc.arg(provider) OFFSET 1)
ON CONFLICT (provider) DO NOTHING;

-- name: GetIngestSchedule :one
SELECT * FROM ingest_schedule WHERE provider = $1;
