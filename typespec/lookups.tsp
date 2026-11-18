import "@typespec/http";

using Http;

namespace Freehire;

model CityMatch {
  /** Canonical city name — pass it verbatim to the `cities` facet, which matches nothing on a near miss. */
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
  /**
   * Which vocabulary the part comes from, and so which parameter it applies to: `skill` →
   * `skills`, `category` → `category`, `company` → `company_slug`. A `title` names no facet
   * and is applied as `q`.
   */
  kind: SuggestionKind;

  /** The facet value to apply. Absent for a `title`. */
  slug?: string;

  /** How this part reads on its own. */
  text: string;
}

model Suggestion {
  /** The whole phrase the row names — the recognised prefix plus the completion, e.g. "Senior Software Engineer Google". */
  text: string;

  /**
   * Every part the row applies. Apply ALL of them: a row naming a role and a company that is
   * applied as only one of the two silently discards what the caller typed.
   */
  parts: SuggestionPart[];

  /** Open postings behind the completion. Never zero — a suggestion that leads to an empty page is withheld. */
  jobs: int64;
}

model SuggestMeta {
  count: int64;
}

// Public reads behind the search box and the location filter. No sign-in.

// Up to 20 cities whose name matches, optionally within one country.
/**
 * Population-ranked prefix search over the embedded city dictionary, optionally narrowed
 * to one country. A blank `q` returns an empty list.
 */
@summary("Prefix-search city names for autocomplete.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        value: "Berlin",
        country: "DE",
      },
    ],
  },
})
@tag("Search")
@route("/geo/cities")
@get
op searchCities(/** Prefix to match. (e.g. `berl`) */ @query q?: string, /** ISO 3166-1 alpha-2 code to narrow to. (e.g. `DE`) */ @query country?: string): Data<CityMatch[]> | Problem;

/**
 * Completes a PHRASE, not a word: `senior software engineer go` completes to Senior
 * Software Engineer at Google. Each row carries every part it names; apply them all.
 * Typos are forgiven in the trailing fragment only. An empty `q` returns no rows. 503 when
 * search is not configured.
 */
@summary("Complete what is being typed into a search box.")
@useAuth(NoAuth)
@tag("Search")
@route("/suggest")
@get
op suggest(
  /** The partly-typed query, matched against posting titles, skills, categories and companies. */
  @query q?: string,

  /** 1 to 20, default 5. A value that does not read falls back to the default. */
  @query limit?: string,
): {
  data: Suggestion[];
  meta: SuggestMeta;
} | Problem;
