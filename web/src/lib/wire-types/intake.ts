import type { components } from '#lib/generated/api.js';

/** A crowdsourced board contribution: a job link a user pasted for a company board we do
 *  not crawl yet. The unit is the board — `source` (ATS) + `board` (company slug) — not a
 *  single vacancy; the ingest side later scrapes all its vacancies. `pending` means
 *  recorded but not yet proven by a crawl; `active` means its first crawl succeeded —
 *  there is no separate manual onboarding step. */
export type Contribution = components['schemas']['Contribution'];

/** A job page in the wild recognised as one the catalog already carries. The lookup that
 *  produces it is public and read-only, so this is deliberately narrower than ResolvedLink:
 *  nothing was imported and no board was recorded, so there is no status to report. */
export type FoundJob = components['schemas']['JobFoundPosting'];

/** What became of the BOARD behind a link handed to the intake.
 *  - `found`    the catalog already carried the posting
 *  - `tracked`  imported, and we already crawl this company's board
 *  - `imported` imported, and the board behind it is now queued for onboarding
 *  - `review`   imported, but its careers site names no board we can crawl, so it went to triage
 *  - `queued`   nothing could read the page, so the link went to manual triage */
export type ResolvedLink = components['schemas']['ResolvedJobPage'];

/** Employee referrals. An offer is a member's moderated "I can refer into company X". */
export type ReferralRequestStatus = components['schemas']['ReferralRequestStatus'];

type ReferralCvKind = components['schemas']['ReferralCvKind'];

export type ReferralOffer = components['schemas']['ReferralOffer'];

/** What a seeker sees of their own request — no referrer identity (the request targets a pool). */
export type SeekerReferralRequest = components['schemas']['ReferralSeekerRequest'];

/** What a referrer sees of an incoming request — the seeker's chosen contact and CV, no identity. */
export type IncomingReferralRequest = components['schemas']['ReferralIncomingRequest'];

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
export type Submission = components['schemas']['Submission'];

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
export type CompanyRelationKind = components['schemas']['CompanyRelationKind'];

/** One row of the moderator queue of proposed company relations. */
export type PendingCompanyRelation = components['schemas']['PendingCompanyRelation'];

/** An account as the moderator's user lookup shows it: who it is and whether a moderator
 *  banned it (with the reason they left). */
export type ModeratedUser = components['schemas']['ModeratedUser'];
