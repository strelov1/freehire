#!/usr/bin/env bash
# One-off, approved 2026-09-05: bulk-run the closed-job branch of the semantic
# embed worker over the ~304k semantic_outbox rows whose job is closed or a
# non-canonical repost.
#
# This is NOT a shortcut past the worker — it is exactly what Store.CompleteClosed
# does (internal/ai/embed, queries in internal/platform/db/queries/semantic.sql),
# in one transaction per chunk instead of one per 100-row batch:
#   ClearSemanticEmbeddedBatch  -> null the three embed columns on jobs
#   DeleteJobSemanticChunks     -> drop the job's job_semantic_chunks rows
#   DeleteSemanticEntriesBatch  -> drop the outbox row
# No TEI call is involved on this path, which is why it can run at SQL speed.
#
# Safe to run beside the live worker: FOR UPDATE ... SKIP LOCKED steps around any
# row the worker has claimed, and every operation is idempotent — a row the worker
# finishes first simply is not selected next chunk.
#
# Safe to stop at any time: each chunk is its own transaction, and the enqueuer
# (EnqueuePendingSemanticJobs) re-derives outstanding work from jobs, never from
# what this deleted.
set -uo pipefail

LOG=/opt/freehire/semantic-drop-closed.log
CHUNK=${CHUNK:-2000}
PAUSE=${PAUSE:-2}
MAX_CHUNKS=${MAX_CHUNKS:-400}

log() { echo "[$(date -u +%FT%TZ)] $*" >>"$LOG"; }
psqlq() { sudo -u postgres psql -d hire -v ON_ERROR_STOP=1 -tA "$@"; }

log "waiting for taxonomy-backfill-night to finish before touching an 11 GB table"
for _ in $(seq 1 144); do
	s=$(systemctl is-active taxonomy-backfill-night.service)
	if [ "$s" != "active" ] && [ "$s" != "activating" ]; then
		log "backfill state=$s -> proceeding"
		break
	fi
	sleep 300
done

s=$(systemctl is-active taxonomy-backfill-night.service)
if [ "$s" = "active" ] || [ "$s" = "activating" ]; then
	log "GAVE UP: backfill still $s after 12h, nothing deleted"
	exit 1
fi

before=$(psqlq -c "SELECT count(*) FROM semantic_outbox;")
log "start: outbox=$before chunk=$CHUNK"

total_rows=0
total_chunks=0
# Keyset cursor over semantic_outbox.id. Without it every chunk re-walks the
# growing prefix of rows it already rejected (the ~420k OPEN entries interleaved
# with the closed ones), turning a linear pass into a quadratic one. Rows below
# the cursor are open-job entries we deliberately leave alone, or entries the
# worker held locked at that moment — the worker owns those either way.
cursor=0
for i in $(seq 1 "$MAX_CHUNKS"); do
	out=$(psqlq -c "
WITH victims AS (
    SELECT o.id AS outbox_id, o.job_id
    FROM semantic_outbox o
    JOIN jobs j ON j.id = o.job_id
    WHERE o.id > $cursor
      AND (j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL)
    ORDER BY o.id
    LIMIT $CHUNK
    FOR UPDATE OF o SKIP LOCKED
),
cleared AS (
    UPDATE jobs
    SET semantic_embedded_model = NULL,
        semantic_embedded_hash  = NULL,
        semantic_embedding      = NULL
    WHERE id IN (SELECT job_id FROM victims)
    RETURNING 1
),
dropped_chunks AS (
    DELETE FROM job_semantic_chunks c
    WHERE c.job_id IN (SELECT job_id FROM victims)
    RETURNING 1
),
dropped_entries AS (
    DELETE FROM semantic_outbox o
    WHERE o.id IN (SELECT outbox_id FROM victims)
    RETURNING 1
)
SELECT (SELECT count(*) FROM victims) || ' '
    || (SELECT count(*) FROM dropped_chunks) || ' '
    || (SELECT COALESCE(max(outbox_id), $cursor) FROM victims);")
	rc=$?

	if [ "$rc" -ne 0 ]; then
		# A deadlock against a concurrent ingest write is expected occasionally:
		# both touch jobs rows. The chunk rolled back whole; the next pass re-selects.
		log "chunk $i failed (rc=$rc), retrying after backoff"
		sleep 15
		continue
	fi

	read -r victims chunks cursor <<<"$out"
	[ "${victims:-0}" -eq 0 ] && { log "no rows left to clear (cursor=$cursor)"; break; }

	total_rows=$((total_rows + victims))
	total_chunks=$((total_chunks + chunks))
	[ $((i % 10)) -eq 0 ] && log "progress: cleared=$total_rows outbox rows, $total_chunks chunk rows ($i chunks, cursor=$cursor)"
	sleep "$PAUSE"
done

after=$(psqlq -c "SELECT count(*) FROM semantic_outbox;")
left=$(psqlq -c "SELECT count(*) FROM semantic_outbox o JOIN jobs j ON j.id=o.job_id WHERE j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL;")
log "done: cleared=$total_rows outbox rows and $total_chunks chunk rows; outbox $before -> $after; closed rows still queued: $left"
log "load: $(uptime)"
