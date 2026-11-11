import "@typespec/http";

using Http;

namespace Freehire;

// The caller's single profile: what they are looking for. Readable with a session or an
// API key; written and cleared with the session cookie only. A save replaces the whole
// profile; `specializations` and `skills` must be arrays (null or absent is a 400).

/** A geographic reach: regions and/or ISO country codes. Empty means worldwide. */
model GeoSet {
  regions?: string[] | null;
  countries?: string[] | null;
}

/** The current single place: an ISO country code and a free-text city. */
model BaseLocation {
  country?: string | null;
  city?: string | null;
}

/** The willingness to move. Open with empty targets means anywhere. */
model Relocation {
  open?: boolean | null;
  regions?: string[] | null;
  countries?: string[] | null;
  cities?: string[] | null;
}

/**
 * Where and how the caller wants to work. Every member is optional on a save; an
 * entirely empty block clears it. Echoed back normalised, with `remote`, `base` and
 * `relocation` always present.
 */
model LocationPreferences {
  work_modes?: string[] | null;
  remote?: GeoSet | null;
  base?: BaseLocation | null;
  relocation?: Relocation | null;
}

/** The profile as saved. */
model ProfileUpdate {
  specializations?: string[] | null;
  skills?: string[] | null;
  seniorities?: string[] | null;
  excluded_skills?: string[] | null;
  excluded_sources?: string[] | null;
  job_languages?: string[] | null;
  excluded_companies?: string[] | null;
  /** Shaped like LocationPreferences; null or absent keeps none. */
  location_preferences?: unknown;
}

/** One work-history entry of the contact-free CV. */
model ProfileExperience {
  title?: string;
  company?: string;
  location?: string;
  start?: PeriodDate;
  end?: PeriodDate;
  current?: boolean;
  summary?: string;
  highlights?: string[];
  stack?: string[];
}

model ProfileEducation {
  degree?: string;
  institution?: string;
  year?: PeriodDate;
}

model ProfileProject {
  name?: string;
  link?: string;
  highlights?: string[];
}

/** The caller's CV without contacts: what they have done. */
model ProfileCV {
  headline?: string;
  location?: string;
  summary?: string;
  total_years?: int64;
  experience?: ProfileExperience[];
  education?: ProfileEducation[];
  languages?: string[];
  skills?: string[];
  certifications?: string[];
  projects?: ProfileProject[];
}

/** Where the CV says the caller IS, as the location dictionary resolved it. Read-only. */
model DerivedLocation {
  countries: string[] | null;
  regions: string[] | null;
  cities: string[] | null;
}

model Profile {
  specializations: string[] | null;
  skills: string[] | null;
  seniorities: string[] | null;
  excluded_skills: string[] | null;
  excluded_sources: string[] | null;
  job_languages: string[] | null;
  excluded_companies: string[] | null;

  /**
   * What the caller asserted, shaped like LocationPreferences, sent exactly as stored;
   * null when they set none.
   */
  location_preferences: unknown;

  /** What was derived from their CV; null when there is no current structured CV. */
  derived_location: DerivedLocation | null;

  /**
   * The contact-free CV, shaped like ProfileCV, sent exactly as the CV store writes it;
   * null when no current structure exists.
   */
  cv: unknown;

  created_at: utcDateTime | null;
  updated_at: utcDateTime | null;
}

/** `data` is null before the first save. */
@tag("Account")
@route("/me/profile")
@get
op getProfile(): {
  data: Profile | null;
} | Problem;

/** Creates or replaces the profile. */
@tag("Account")
@route("/me/profile")
@put
op putProfile(@body body: ProfileUpdate): Data<Profile> | Problem;

/** Clears the profile. Idempotent: deleting when none exists is still a 204. */
@tag("Account")
@route("/me/profile")
@delete
op deleteProfile(): {
  @statusCode _: 204;
} | Problem;
