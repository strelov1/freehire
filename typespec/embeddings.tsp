import "@typespec/http";

using Http;

namespace Freehire;

/**
 * The embedding backfill's current progress document. Figures not yet measured are null.
 * The `/stats/embeddings/stream` route is Server-Sent Events and stays hand-written.
 */
@tag("Stats")
@route("/stats/embeddings")
@get
op getEmbeddingProgress(): Data<unknown> | Problem;
