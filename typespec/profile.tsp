import "@typespec/http";

using Http;

namespace Freehire;

// The caller's single profile: what they are looking for. Readable with a session or an
// API key; written and cleared with the session cookie only. A save replaces the whole
// profile; `specializations` and `skills` must be arrays (null or absent is a 400).

/** A geographic reach: regions and/or ISO country codes. Empty means worldwide. */
model GeoSet {
  regions?: string[];
  countries?: string[];
}

/** The current single place: an ISO country code and a free-text city. */
model BaseLocation {
  country?: string;
  city?: string;
}

/** The willingness to move. Open with empty targets means anywhere. */
model Relocation {
  open: boolean;
  regions?: string[];
  countries?: string[];
  cities?: string[];
}

/**
 * Where and how the caller wants to work, as it is stored and echoed back: normalised,
 * with `remote`, `base` and `relocation` always present and empty lists left out. Every
 * member is optional on a save, and an entirely empty block clears it.
 */
model LocationPreferences {
  work_modes?: string[];
  remote: GeoSet;
  base: BaseLocation;
  relocation: Relocation;
}

/** The profile as saved. */
model ProfileUpdate {
  /** One or more job categories (max 5). (e.g. `["backend"]`) */
  specializations?: string[] | null;
  /** Canonical skill tokens (non-empty). (e.g. `["go","postgresql"]`) */
  skills?: string[] | null;
  seniorities?: string[] | null;
  excluded_skills?: string[] | null;
  excluded_sources?: string[] | null;
  /**
   * Preferred vacancy languages as two-letter codes (max 200); empty or omitted means any
   * language. (e.g. `["en","ru"]`)
   */
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
  countries: string[];
  regions: string[];
  cities: string[];
}

model Profile {
  specializations: string[];
  skills: string[];
  seniorities: string[];
  excluded_skills: string[];
  excluded_sources: string[];
  job_languages: string[];
  excluded_companies: string[];

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
@summary("Your career profile, or null if you have not saved one.")
@useAuth(SessionOrKey)
@tag("Account")
@route("/me/profile")
@get
op getProfile(): {
  data: Profile | null;
} | Problem;

// Creates or replaces the profile.
/**
 * The whole profile is replaced on each save. An unknown specialization (must be a job
 * category), empty skills, or an out-of-vocabulary location value is a `400`.
 */
@summary("Create or replace your profile.")
@tag("Account")
@route("/me/profile")
@put
op putProfile(@body body: ProfileUpdate): Data<Profile> | Problem;

// Clears the profile. Idempotent: deleting when none exists is still a 204.
/** Returns `204 No Content`. */
@summary("Clear your profile (idempotent).")
@tag("Account")
@route("/me/profile")
@delete
op deleteProfile(): {
  @statusCode _: 204;
} | Problem;
