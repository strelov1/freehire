import "@typespec/http";

using Http;

namespace Freehire;

/** Every figure is null until it has been measured. */
model EmbeddingProgress {
  eligible: int64 | null;
  covered: int64 | null;
  snapshot_covered: int64 | null;
  queued: int64 | null;
  rate_per_minute: float64 | null;
  eta_seconds: int64 | null;
  snapshot_at: utcDateTime | null;
  measured_at: utcDateTime | null;
}

// The embedding backfill's current progress document. Figures not yet measured are
// null. The `/stats/embeddings/stream` route is Server-Sent Events and stays
// hand-written.
/**
 * Backs the public /semantic-search page. `eligible` and `snapshot_covered` come from a
 * snapshot `cmd/rollup-stats` measures every few hours (the count is too slow for a
 * request); `queued` is the embedding queue, sampled every 15 seconds, and `covered` is
 * `snapshot_covered` plus how much the queue has drained since that snapshot — not
 * `eligible − queued`, since the queue also carries closed postings waiting to be
 * cleared. `rate_per_minute` counts only what left the queue over the last 10 minutes, so
 * a batch of new postings arriving at once does not read as negative progress. Any figure
 * not yet measured is `null`, never `0`.
 */
@summary("How much of the catalogue carries a semantic embedding, and how fast the rest is going.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #{
      eligible: 1903114,
      covered: 1897020,
      snapshot_covered: 1889512,
      queued: 6094,
      rate_per_minute: 570.4,
      eta_seconds: 641,
      snapshot_at: utcDateTime.fromISO("2026-10-19T09:22:03Z"),
      measured_at: utcDateTime.fromISO("2026-10-19T12:04:15Z"),
    },
  },
})
@tag("Stats")
@route("/stats/embeddings")
@get
op getEmbeddingProgress(): Data<EmbeddingProgress> | Problem;
