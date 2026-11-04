# semantic_embedding contraction rollout

The SQL here is deliberately outside the top-level `migrations/*.sql` glob that
`cmd/migrate`, fresh Postgres init and the integration template execute. sqlc reads it
after the top-level schema, so this release generates job projections without
`jobs.semantic_embedding` while the database still has the column.

1. Deploy this code normally, on both the application host and the crawl host.
2. Retire every running binary built before it: the old API color, and long-running
   workers (ingest, enrich, backfills, mail-ingest). Their generated `SELECT *`
   projections still name the column and would fail once it is gone.
3. In a follow-up PR, move the SQL file to the top-level `migrations/` directory
   (renumbered if 0235 is taken) and restore `sqlc.yaml` to `schema: "migrations"`.

The promotion commit must carry
`Migration-Edit: Promote deferred semantic_embedding contraction after compatible binaries are deployed`
