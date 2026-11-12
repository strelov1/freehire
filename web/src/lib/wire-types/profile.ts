import type { components } from '../generated/api.js';
import type { Education as ResumeEducation, Professional } from '../generated/contracts';

/** An authenticated account, as returned by the auth endpoints. `role` ('user' | 'moderator'
 *  | 'admin') and `email_verified` are UI affordances only; the server re-checks both on
 *  every privileged request. `tier` rides along so the header's badge needs no second
 *  request, `agent_connected` stops the "connect your agent" strip asking once true, and a
 *  null `onboarding_completed_at` routes the account to /onboarding. */
export type User = components['schemas']['AuthUser'];

/** Optional "where & how I want to work" block: accepted work arrangements plus three
 *  geographic parts (remote reach, current base, relocation), freely combined. Mirrors the
 *  backend userprofile.LocationPreferences; null on a profile means the user set none. */
export type LocationPreferences = components['schemas']['LocationPreferences'];

/** Where the CV says the candidate IS, resolved by the location dictionary. Read-only:
 *  it is produced by the CV derivation alone and no profile write can set it. Deliberately
 *  a sibling of location_preferences rather than part of it — that block is what the user
 *  ASSERTED, this is what was DERIVED for them, and only the former may be saved back. */
export type DerivedLocation = components['schemas']['DerivedLocation'];

/** A user's single profile: their professional self — a non-empty set of
 *  `specializations` (job categories) and a non-empty set of canonical skill tokens. One
 *  per user (no id, no name); the foundation for finding relevant work. Timestamps are
 *  RFC3339 strings or null. */
export type UserProfile = Omit<components['schemas']['Profile'], 'location_preferences' | 'cv'> & {
  /** What the user asserted, sent exactly as stored; null when they set none. The contract
   *  leaves it raw (an old stored shape must still read), so the precise type is kept here. */
  location_preferences: LocationPreferences | null;
  /** The caller's structured CV without its contact fields — served beside the profile so
   *  one read covers both what the user wants and what they have done. Null when no current
   *  structure exists. Contacts come from GET /me/resume instead (the profile page's contact
   *  card reads them there). Raw in the contract for the same reason as above. */
  cv: Professional | null;
};

/** Talent Network membership: in, or not. Two values, not three — `public` was retired
 *  with the mode picker itself (migration 0148), because the product should answer how
 *  much of a candidate to disclose rather than ask them. Mirrors the CHECK constraint on
 *  `users.talent_network_visibility`. */
export type TalentNetworkVisibility = components['schemas']['TalentNetworkVisibility'];

/** The caller's own Talent Network membership.
 *
 *  `talent_handle` is ABSENT until the first join: it is minted then and kept forever
 *  after, so an empty one means "not a member yet", never "waiting for one". A surface
 *  reading it must show no link rather than build one that would 404. `listed` is whether
 *  a visitor can actually SEE them — not the same as membership. A candidate who joins
 *  before uploading a CV is a member, holds a handle, and is still excluded from the
 *  catalogue by the stamp gate, so their card 404s. Any surface that offers "view your
 *  public profile" must read this and not `talent_network_visibility`. */
export type TalentNetworkSetting = components['schemas']['TalentNetworkMembership'];

/** A notification subscription on a saved search. */
export type Subscription = components['schemas']['Subscription'];

/** Telegram link status for the current user. `enabled` is whether the feature is
 *  configured server-side at all (so the UI can hide it); `linked` is whether this
 *  user has connected their chat. */
export interface TelegramStatus {
  enabled: boolean;
  linked: boolean;
  chat_id?: number;
}

/**
 * Whether this account has linked a Discord account, and whether the paid role is on it.
 *
 * `enabled` is always true when the endpoint answers at all — the routes are not mounted on a
 * deployment without Discord credentials, so an unconfigured server 404s rather than
 * reporting a feature that half exists. The caller treats that 404 as "hide the card".
 */
export interface DiscordStatus {
  enabled: boolean;
  linked: boolean;
  role_granted?: boolean;
}

/** What a résumé yields through the deterministic dictionaries: canonical skills and
 *  every specialization the résumé spans, plus the seniority grade. `skills` and
 *  `categories` are always arrays (empty when nothing resolved); `seniority` is omitted
 *  when unresolved (never guessed). */
export interface ResumeProfile {
  skills: string[];
  categories: string[];
  seniority?: string;
}

/** The read-only structured résumé parsed from the uploaded CV by the LLM. */
/** @public */
export type { Structured as ResumeStructured, Experience as ResumeExperience } from '../generated/contracts';

export type { ResumeEducation };

/** Candidate-owned overrides, editable independently of the CV parse that seeded them
 *  (`GET /me/resume`'s `contacts`, and the body of `PUT /me/resume/contacts`). Despite
 *  the name (kept for the route/wire field, `contacts`), this covers identity
 *  (full_name/email/phone/location/links) *and* the flat part of the semantic body
 *  (headline/summary/languages/certifications/education) — see
 *  `internal/resume/owned.go`. A PUT replaces the whole block, so a caller editing only
 *  some of these fields goes through resumeStore.saveContacts, which builds each save
 *  from the current object (see resume.svelte.ts). */
export interface CandidateContacts {
  full_name?: string;
  email?: string;
  phone?: string;
  location?: string;
  links?: string[];
  headline?: string;
  summary?: string;
  languages?: string[];
  certifications?: string[];
  education?: ResumeEducation[];
  /** Years of experience as the CANDIDATE states them, overlaying whatever the CV extract
   *  computed. Captured on the onboarding wizard's experience step, pre-filled from the
   *  extract's own figure. */
  total_years?: number;
  // The five body fields above are owned per field, and a non-empty value has always
  // implied that. But clearing one to "" leaves no non-empty value to signal it, so the
  // editor that owns that field sends its *_set flag alongside a save that empties it —
  // see internal/resume/owned.go's Owned for why identity fields need no equivalent.
  headline_set?: boolean;
  summary_set?: boolean;
  languages_set?: boolean;
  certifications_set?: boolean;
  education_set?: boolean;
  /** The same explicit-clear signal for the numeric field, where it matters more: 0 is a
   *  real answer ("less than a year"), so without this flag a deliberate zero would be
   *  indistinguishable from never having answered and the CV's figure would come back. */
  total_years_set?: boolean;
}

/** The résumé status (`GET /me/resume`): storage flags, owned contacts, parse status,
 *  and the structured résumé (current semantic sections when stamped). */
export interface ResumeMeta {
  enabled: boolean;
  present: boolean;
  uploaded_at: string | null;
  structured: import('../generated/contracts').Structured | null;
  /** True when a résumé is stored but its structured parse stamp does not match yet. */
  structure_pending?: boolean;
  /** pending | ok | failed — omit when no résumé. */
  parse_status?: string;
  parse_detail?: string;
  contacts?: CandidateContacts | null;
}

/** The caller's profile headshot: `enabled` is false when object storage is unconfigured
 *  (the upload control is then not offered at all), `present` says whether one is stored,
 *  and `uploaded_at` doubles as the cache buster on the image URL. */
export interface PhotoMeta {
  enabled: boolean;
  present: boolean;
  // Omitted by the server when there is no headshot, so `undefined` — not null — is the
  // absent case.
  uploaded_at?: string;
}
