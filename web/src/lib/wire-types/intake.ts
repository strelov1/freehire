import type { components } from '#lib/generated/api.js';

/** A crowdsourced board contribution: a job link a user pasted for a company board we do
 *  not crawl yet. The unit is the board — `source` (ATS) + `board` (company slug) — not a
 *  single vacancy; the ingest side later scrapes all its vacancies. `pending` means
 *  recorded but not yet proven by a crawl; `active` means its first crawl succeeded —
 *  there is no separate manual onboarding step. */
type ContributionStatus = 'pending' | 'active' | 'rejected' | 'review';

export interface Contribution {
  id: number;
  url: string;
  // Empty for a `review` row (an unrecognized link awaiting manual triage); set once a board is
  // recognized or a maintainer promotes the review row.
  source: string;
  board: string;
  status: ContributionStatus;
  /** Which door the link came through — web, telegram, extension, cli, or unknown for a row
   *  recorded before surfaces were tracked (or by a client that sends no tag). */
  surface: string;
  created_at: string | null;
}

/** What became of the BOARD behind a link handed to the intake.
 *  - `found`    the catalog already carried the posting
 *  - `tracked`  imported, and we already crawl this company's board
 *  - `imported` imported, and the board behind it is now queued for onboarding
 *  - `review`   imported, but its careers site names no board we can crawl, so it went to triage
 *  - `queued`   nothing could read the page, so the link went to manual triage */
type IntakeStatus = 'found' | 'tracked' | 'imported' | 'review' | 'queued';

/** A job page in the wild recognised as one the catalog already carries. The lookup that
 *  produces it is public and read-only, so this is deliberately narrower than ResolvedLink:
 *  nothing was imported and no board was recorded, so there is no status to report. */
export interface FoundJob {
  public_slug: string;
}

export interface ResolvedLink {
  public_slug: string | null;
  status: IntakeStatus;
  /** The employer, when the catalog already carries it — through any source, not just the one
   *  this link came from. Whether we know the COMPANY is a separate question from what became
   *  of its BOARD, so this rides along with every outcome, not just `tracked`. */
  company_slug?: string;
}

/** Employee referrals. An offer is a member's moderated "I can refer into company X". */
type ReferralOfferStatus = 'pending' | 'approved' | 'rejected';

export type ReferralRequestStatus = 'sent' | 'contacted' | 'declined';

type ReferralCvKind = 'original' | 'built';

export interface ReferralOffer {
  id: string;
  company_slug: string;
  company_name: string;
  linkedin_url: string;
  status: ReferralOfferStatus;
  decided_at: string | null;
  created_at: string | null;
}

/** What a seeker sees of their own request — no referrer identity (the request targets a pool). */
export interface SeekerReferralRequest {
  id: string;
  company_slug: string;
  company_name: string;
  job_id: number | null;
  cv_kind: ReferralCvKind;
  cv_id: string | null;
  status: ReferralRequestStatus;
  created_at: string | null;
}

/** What a referrer sees of an incoming request — the seeker's chosen contact and CV, no identity. */
export interface IncomingReferralRequest {
  id: string;
  company_slug: string;
  company_name: string;
  job_id: number | null;
  cv_kind: ReferralCvKind;
  linkedin_url?: string;
  contact_telegram?: string;
  contact_email?: string;
  note?: string;
  status: ReferralRequestStatus;
  created_at: string | null;
}

/** The seeker's referral-request payload. Exactly one CV field is meaningful per cv_kind. */
export interface ReferralRequestInput {
  company_slug: string;
  /** The vacancy the request is made from, when there is one. */
  job_slug?: string;
  cv_kind: ReferralCvKind;
  cv_id?: string;
  linkedin_url: string;
  contact_telegram?: string;
  contact_email?: string;
  note?: string;
}

/** What a seeker can attach to a referral request into one company: the CVs tailored for
 *  the vacancy it is made from (or, from the company page, for any of its vacancies),
 *  newest first, and whether they uploaded a CV file. */
export type ReferralCvOptions = components['schemas']['ReferralCVOptions'];

/** A job submitted for moderation. `status` is the review lifecycle; `review_reason`
 *  carries an optional rejection note. `submitter_email` is present only on the
 *  moderator review queue, never on a submitter's own view. */
export interface Submission {
  id: number;
  url: string;
  source?: string;
  title: string;
  company: string;
  location?: string;
  remote: boolean;
  description?: string;
  posted_at?: string | null;
  status: 'pending' | 'approved' | 'rejected';
  review_reason?: string;
  reviewed_at?: string | null;
  created_at: string | null;
  submitter_email?: string;
  // Public slug of the minted live vacancy, present on an approved submission in the
  // caller's own list, so the UI can link to /jobs/<job_slug>.
  job_slug?: string;
  // The structured facets the submitter stated, echoed back by the server.
  skills?: string[];
  regions?: string[];
  cities?: string[];
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  salary_min?: number;
  salary_max?: number;
  salary_currency?: string;
  salary_period?: string;
}

/** The content a user submits for review (mirrors the moderator create body). */
export interface SubmissionInput {
  url: string;
  title: string;
  company: string;
  location?: string;
  remote?: boolean;
  description?: string;
  source?: string;
  // Structured facets the submitter can state explicitly. The backend sanitizes them
  // (unknown work_mode/regions dropped) and applies them as overrides on the minted job;
  // salary becomes an authoritative manual salary.
  skills?: string[];
  regions?: string[];
  cities?: string[];
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  salary_min?: number;
  salary_max?: number;
  salary_currency?: string;
  salary_period?: string;
}

/** What freehire could parse from a pasted job URL, for the submit form to prefill —
 *  never persisted. A field freehire's own dictionaries derive better than any source
 *  page states it (skills, cities) is not included. */
export type PrefillResult = components['schemas']['SubmissionPrefill'];

/** A related company on a company page: the parent that owns it, or a subsidiary it owns.
 *  A separate employer with its own page and jobs (freehire#3895). */
export interface CompanyRef {
  slug: string;
  name: string;
  job_count: number;
}

/** What a person can claim about two companies: `same` (one employer under two names, the
 *  page's company retires into the target) or `subsidiary` (the page's company is owned by
 *  the target, both stay). */
export type CompanyRelationKind = 'same' | 'subsidiary';

/** One row of the moderator queue of proposed company relations. */
export interface PendingCompanyRelation {
  id: number;
  kind: CompanyRelationKind;
  company_slug: string;
  company_name: string;
  company_job_count: number;
  target_slug: string;
  target_name: string;
  target_job_count: number;
  note: string;
  created_at: string;
  /** `approved` marks a merge whose job move stopped part-way; approving again resumes it. */
  status: 'pending' | 'approved';
}

/** An account as the moderator's user lookup shows it: who it is and whether a moderator
 *  banned it (with the reason they left). */
export type ModeratedUser = components['schemas']['ModeratedUser'];
