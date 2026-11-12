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

/**
 * The embedding backfill's current progress document. Figures not yet measured are null.
 * The `/stats/embeddings/stream` route is Server-Sent Events and stays hand-written.
 */
@tag("Stats")
@route("/stats/embeddings")
@get
op getEmbeddingProgress(): Data<EmbeddingProgress> | Problem;
