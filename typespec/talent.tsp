import "@typespec/http";

using Http;

namespace Freehire;

/** A year, with a month when the CV gives one. */
model PeriodDate {
  year: int32;
  month?: int32;
}

/** One role of a work history, with everything nameable removed. */
model TalentRole {
  title?: string;
  seniority?: string;
  category?: string;
  start?: PeriodDate;
  end?: PeriodDate;
  current?: boolean;
  stack?: string[];
}

/** One education entry: the degree level a dictionary resolved, and the year. */
model TalentEducation {
  level?: string;
  year?: PeriodDate;
}

/** The anonymised card: only what a dictionary can vouch for. */
model TalentCard {
  seniority?: string;
  category?: string;
  total_years?: int32;
  skills: string[];
  roles: TalentRole[];
  education?: TalentEducation[];
  certifications?: string[];
}

model TalentMember {
  handle: string;
  card: TalentCard;

  /** The IANA zone. */
  timezone?: string;

  /** The part of the zone before the slash; the `tz` filter reads it. */
  timezone_region?: string;

  cities: string[];
  specializations: string[];
  updated_at: utcDateTime;

  /** Whether GET /talent/{handle}/photo has something to serve. */
  has_photo: boolean;
}

model FacetRange {
  min: int32;
  max: int32;
}

/** How many members stand behind each filter value. */
model TalentFacets {
  total: int32;

  /**
   * Facet name → value → member count. A count of -1 means the value is present but
   * held by too few members to say how many; 0 means nobody holds it.
   */
  facets: Record<Record<int32>>;

  /** Ranged facets. The catalogue has none, so this is always empty. */
  stats: Record<FacetRange>;
}

/**
 * The catalogue filter. Each list param is comma-joined. A param this endpoint
 * cannot read is not refused: the answer widens and names it in `meta.ignored_params`,
 * so the numeric ones are strings on the wire.
 */
model TalentQuery {
  @query categories?: string;
  @query seniorities?: string;
  @query skills?: string;
  @query tz?: string;
  @query cities?: string;
  @query specializations?: string;
  @query min_years?: string;
}

// The public Talent Network catalogue. Unauthenticated by design: a signed-out
// visitor is who it is for. Every way of a member being absent is the same 404.

/**
 * Every filter takes values from a closed vocabulary or a number; there is no free-text
 * search, because a card carries no free text to search. Values within one filter are OR,
 * different filters narrow together, and an absent filter is identical to an empty one. A
 * parameter this endpoint does not read — including one it recognises but cannot parse,
 * like `min_years=lots` — is named in `meta.ignored_params` rather than silently widening
 * the answer.
 */
@summary("The public catalogue of members, filtered and paged.")
@useAuth(NoAuth)
@tag("Talent Network")
@route("/talent")
@get
op listTalent(...TalentQuery, /** Page size, 1–100. Defaults to 24. (e.g. `24`) */ @query limit?: string, /** Rows to skip. (e.g. `0`) */ @query offset?: string): List<TalentMember> | Problem;

/**
 * Takes the same filters as the list and answers with the count of members behind every
 * value of every facet, plus the total behind the filter itself. Same shape as the
 * job-search facet endpoint, so one control renders either.
 *
 * Each facet is counted with its OWN selection removed, while every other filter still
 * applies — otherwise picking one skill makes every other skill read zero and the control
 * silently becomes single-select.
 *
 * A value nobody carries is absent rather than reported as zero. A value carried by fewer
 * members than the floor is offered with its number withheld: the option stays usable,
 * and the number does not become a way to name somebody. Counts describe the filter you
 * sent and never the whole membership.
 */
@summary("How many members stand behind each filter value.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #{
      total: 3,
      facets: #{
        categories: #{
          backend: 3,
        },
        skills: #{
          go: 3,
          kubernetes: 2,
          postgresql: 0,
        },
        tz: #{
          Europe: 2,
        },
      },
      stats: #{},
    },
  },
})
@tag("Talent Network")
@route("/talent/facets")
@get
op getTalentFacets(...TalentQuery): DataWithIgnored<TalentFacets> | Problem;

// A card is a person: cached privately only, so leaving takes effect at once.
/**
 * Four ways of being absent — a member who left, one whose CV extract has gone stale, a
 * handle nobody holds, and a string that could not be a handle — all answer the same 404,
 * so the route cannot be used to ask whether an account exists. An account’s `username`
 * is not an address here. Membership is re-read from the database on every request, so
 * leaving takes effect immediately rather than when a cache expires.
 */
@summary("One member’s public card.")
@useAuth(NoAuth)
@tag("Talent Network")
@route("/talent/{handle}")
@get
op getTalentMember(/** The member’s minted catalogue handle. (e.g. `backend-7f2a`) */ @path handle: string): {
  @header("Cache-Control") cacheControl: string;
  @header("X-Robots-Tag") robotsTag: string;
  @body body: Data<TalentMember>;
} | Problem;

/** The member's photo, blurred past recognition. The original is never served. */
@useAuth(NoAuth)
@tag("Talent Network")
@route("/talent/{handle}/photo")
@get
op getTalentPhoto(@path handle: string): {
  @header contentType: "image/jpeg";
  @header("Cache-Control") cacheControl: string;
  @header("X-Robots-Tag") robotsTag: string;
  @body photo: bytes;
} | Problem;

/** The caller's own place in the Talent Network. */
enum TalentNetworkVisibility {
  off,
  anonymous,
}

model TalentNetworkMembership {
  talent_network_visibility: TalentNetworkVisibility;

  /** The handle in the member's public URL, once they have one. */
  talent_handle?: string;

  /** Whether the catalogue shows the member right now. */
  listed: boolean;
}

model TalentNetworkVisibilityUpdate {
  /** `off` or `anonymous`; anything else is a 400. */
  visibility?: string;
}

// With a session or an API key.
/**
 * A caller who has never touched the setting reads `"off"` — the column default, not a
 * sentinel. `talent_handle` is absent until the first time they join: it is minted then,
 * and kept forever after, so leaving and rejoining never changes the URL somebody already
 * shared.
 */
@summary("Your current Talent Network visibility and shareable id.")
@useAuth(SessionOrKey)
@tag("Talent Network")
@route("/me/talent-network")
@get
op getTalentNetworkMembership(): Data<TalentNetworkMembership> | Problem;

// Joins or leaves the catalogue. Session cookie only.
/**
 * Two states, not three: `public` was retired with the mode picker itself, and a request
 * asking for it is a 400. Joining for the first time mints the handle the public
 * catalogue addresses you by.
 */
@summary("Join or leave the Talent Network.")
@tag("Talent Network")
@route("/me/talent-network")
@put
op setTalentNetworkVisibility(@body body: TalentNetworkVisibilityUpdate): Data<TalentNetworkMembership> | Problem;
