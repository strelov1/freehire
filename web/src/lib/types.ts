// Wire types mirroring the backend JSON (internal/db models and query rows).
// Timestamps marshal as RFC3339 strings when present, or null.

import type { JobMatch, Blocker, Report as ATSReportContract } from './generated/contracts';
/** @public — the app's vocabulary for the generated contract; a name is carried here
 *  whether or not a screen reads it yet. */
export type { Job, Enrichment, Verdict, Gap, SkillRow } from './generated/contracts';
/** @public — the app's name for the onboarding survey record, carried here whether or not
 *  a screen reads it yet, like the contract re-exports above. It is `Responses` in the
 *  generated file only because that file is one flat namespace and `Answers` is already the
 *  screening answers' — see cmd/gen-contracts — so the useful name has to be given
 *  somewhere, and this facade is where the app gives it. */
export type { Responses as SurveyAnswers } from './generated/contracts';
// The list-row projection of a job: the same names and derivations as Job, minus the posting
// text and everything else a row does not draw.
export type { Card as JobCard } from './generated/contracts';
// Per-job profile match (how well a job's skills are covered by the caller's profile).
/** @public */
export type { JobMatch, AdjacentSkill } from './generated/contracts';
// Hard-constraint blockers (years, education, certs, work auth, location) surfaced
// beside skill coverage. BlockerCategory/BlockerSeverity are the enum aliases.
/** @public */
export type { Blocker, BlockerCategory, BlockerSeverity } from './generated/contracts';
// The profile-match endpoint returns skill coverage plus the advisory blockers.
export type JobMatchResult = JobMatch & { blockers: Blocker[] };
// atscheck's Report is aliased ATSReport (a local Report — job reports — already exists);
// its category/line-item shapes come along for the report view.
/** @public */
export type { Report as ATSReport, ScoreCategory as ATSCategory, LineItem as ATSLineItem } from './generated/contracts';

/** The CV ATS report response: `has_cv` is false when the caller has no stored CV
 *  (the page prompts an upload); `report` is present only when `has_cv` is true. */
export interface ATSResponse {
  has_cv: boolean;
  report: ATSReportContract | null;
}

// The on-demand LLM job-match analysis wire shapes (five scored dimensions + the
// ATS-style requirement match), generated from internal/matchanalysis.
/** @public */
export type {
  Analysis as MatchAnalysis,
  Dimension as MatchDimension,
  Requirement as MatchRequirement,
  Signal,
} from './generated/contracts';

/** The assembled follow-up message for a silent application. Deterministic and
 *  template-built server-side — no model, no allowance. `recipient` is present only
 *  when linked mail supplied an address; an unanswered application (the commonest
 *  silent one) has nobody to address, and the draft is issued without one. */
/** One interpreted search — a written description turned into filter values, as
 *  POST /api/v1/search/interpret returns it. It round-trips: send it back as
 *  `previous` to refine it. Every value is already canonicalised server-side against
 *  the real dictionaries (see internal/searchintent), so the client only places them. */
export interface Interpretation {
  /** One sentence describing the search these values build. Shown instead of the raw
   *  filters, and produced in the same model response as them, so the sentence can
   *  never describe a different search than the one that was resolved. */
  summary: string;
  facets: Record<string, string[]>;
  exclude: Record<string, string[]>;
  query: string;
  salary_min?: number | null;
  posted_within_days?: number | null;
  experience_years_max?: number | null;
  visa_sponsorship?: boolean;
  /** What the server could not place, verbatim as the model wrote it. It is rendered:
   *  a drop the user is not told about is indistinguishable from a value applied. */
  unresolved: string[];
  /** Nothing resolved. Distinct from a successful interpretation, because applying an
   *  empty filter shows the whole catalogue and reads as "everything matches you". */
  empty: boolean;
}

export type * from './wire-types/billing';
export type * from './wire-types/companies';
export type * from './wire-types/profile';
export type * from './wire-types/intake';
export type * from './wire-types/employer';
export type * from './wire-types/reports';
export type * from './wire-types/tracking';
export type * from './wire-types/notifications';
export type * from './wire-types/mail';
export type * from './wire-types/buddies';
export type * from './wire-types/stats';
export type * from './wire-types/settings';
export type * from './wire-types/community';
export type * from './wire-types/experience';
export type * from './wire-types/mentors';
export type * from './wire-types/onboarding';
