import "@typespec/http";

using Http;

namespace Freehire;

model CityMatch {
  /** The city's canonical name, the value the filters take. */
  value: string;

  /** ISO 3166-1 alpha-2, lower case. */
  country: string;
}

enum SuggestionKind {
  title,
  skill,
  category,
  company,
}

/** One recognised piece of the query: a dictionary term, or text still being typed. */
model SuggestionPart {
  kind: SuggestionKind;

  /** The dictionary slug, when the part is one. */
  slug?: string;

  text: string;
}

model Suggestion {
  text: string;
  parts: SuggestionPart[];

  /** How many open jobs the suggestion finds. */
  jobs: int64;
}

model SuggestMeta {
  count: int64;
}

// Public reads behind the search box and the location filter. No sign-in.

/** Up to 20 cities whose name matches, optionally within one country. */
@tag("Search")
@route("/geo/cities")
@get
op searchCities(@query q?: string, @query country?: string): Data<CityMatch[]> | Problem;

/** Search-box suggestions. 503 when search is not configured. */
@tag("Search")
@route("/suggest")
@get
op suggest(
  @query q?: string,

  /** 1 to 20, default 5. A value that does not read falls back to the default. */
  @query limit?: string,
): {
  data: Suggestion[];
  meta: SuggestMeta;
} | Problem;
