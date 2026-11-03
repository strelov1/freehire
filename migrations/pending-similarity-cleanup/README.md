# Similarity contraction rollout

These files are deliberately outside the top-level `migrations/*.sql` glob that
`cmd/migrate`, fresh Postgres init and the integration template execute. sqlc reads
them after the top-level schema, so this release generates explicit projections
without the retired columns while remaining compatible with the existing database.

1. Remove `similar-backfill` from the ops release worker list and retire the
   `freehire-similarw` service/timer on both application and crawl hosts.
2. Deploy this code normally, including the new worker binaries on both hosts.
3. Stop the old API color and retire/drain every running binary built before this
   change. This includes long-running ingest/backfill workers and mail-ingest,
   because all old generated job projections still name the retired columns.
4. Verify the serving API and all remaining job-row consumers use the new revision.
5. In a follow-up PR, move these two SQL files to the top-level `migrations/`
   directory and restore `sqlc.yaml` to `schema: "migrations"`. Normal deployment
   can then apply the contraction before its color flip safely.

Do not run these files during the first release. After contraction, rollback must
use a revision that already omits the retired columns. Chunk vectors remain stored;
only the offline lists and their unused indexes are removed.

The promotion commit must carry
`Migration-Edit: Promote deferred similarity contraction after compatible binaries are deployed`
because the immutability check treats every SQL path in main as applied. These files
were deliberately not applied by the first release; the trailer records that exception.
