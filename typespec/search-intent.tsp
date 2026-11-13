import "@typespec/http";

using Http;

namespace Freehire;

model SearchIntentRequest {
  /** The description of the search; refused over the length limit. */
  text?: string;

  /** The result being refined, echoed back from a prior answer. */
  previous?: unknown;
}

/**
 * Turns a written description of a job search into filter values. Open to signed-out
 * callers too, and a free AI route for every account. A description that resolved nothing
 * is a 200 with `empty: true`.
 */
@useAuth(NoAuth)
@tag("Search")
@route("/search/interpret")
@post
op interpretSearch(@body body: SearchIntentRequest): Data<unknown> | Problem;
