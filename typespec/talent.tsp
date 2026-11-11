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

@tag("Talent Network")
@route("/talent")
@get
op listTalent(...TalentQuery, @query limit?: string, @query offset?: string): List<TalentMember> | Problem;

@tag("Talent Network")
@route("/talent/facets")
@get
op getTalentFacets(...TalentQuery): DataWithIgnored<TalentFacets> | Problem;

/** A card is a person: cached privately only, so leaving takes effect at once. */
@tag("Talent Network")
@route("/talent/{handle}")
@get
op getTalentMember(@path handle: string): {
  @header("Cache-Control") cacheControl: string;
  @header("X-Robots-Tag") robotsTag: string;
  @body body: Data<TalentMember>;
} | Problem;

/** The member's photo, blurred past recognition. The original is never served. */
@tag("Talent Network")
@route("/talent/{handle}/photo")
@get
op getTalentPhoto(@path handle: string): {
  @header contentType: "image/jpeg";
  @header("Cache-Control") cacheControl: string;
  @header("X-Robots-Tag") robotsTag: string;
  @body photo: bytes;
} | Problem;
