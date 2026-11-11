import "@typespec/http";

using Http;

namespace Freehire;

model WatchedCompany {
  slug: string;
  name: string;
  added_at: utcDateTime;
}

model WatchlistMeta {
  ...PageMeta;

  /** How many companies one list may hold. */
  max: int64;
}

model WatchlistPage {
  data: WatchedCompany[];
  meta: WatchlistMeta;
}

model Watching {
  watched: boolean;
}

model WatchlistImport {
  /** Company names or slugs, one per entry. An empty list is refused with a 400. */
  names?: string[];
}

model WatchlistImportResult {
  added: int64;

  /** Already on the list. */
  already: int64;

  /** Entries that did not resolve to exactly one company, as typed. */
  unmatched: string[];
}

// A signed-in person's company watchlist (session cookie). The alert for new postings
// from it is a saved search of `watchlist=true`, so there is nothing to subscribe to here.
// The answers are one person's own and never cacheable.

/** One page, newest first. A page param that does not read falls back to its default. */
@tag("Watchlist")
@route("/me/watchlist")
@get
op listWatchlist(
  /** A substring of the company name. */
  @query q?: string,

  @query limit?: string,
  @query offset?: string,
): {
  @header("Cache-Control") cacheControl: string;
  @body body: WatchlistPage;
} | Problem;

/** Adds the companies the names identify and reports what it could not resolve. */
@tag("Watchlist")
@route("/me/watchlist/import")
@post
op importWatchlist(@body body: WatchlistImport): Data<WatchlistImportResult> | Problem;

@tag("Watchlist")
@route("/me/watchlist/{slug}")
@get
op getWatching(@path slug: string): {
  @header("Cache-Control") cacheControl: string;
  @body body: Data<Watching>;
} | Problem;

@tag("Watchlist")
@route("/me/watchlist/{slug}")
@put
op addToWatchlist(@path slug: string): Data<WatchedCompany> | Problem;

/** A company that was not on the list is not an error. */
@tag("Watchlist")
@route("/me/watchlist/{slug}")
@delete
op removeFromWatchlist(@path slug: string): NoContentResponse | Problem;
