// Wire types mirroring the backend JSON (internal/db models and query rows).
// Timestamps marshal as RFC3339 strings when present, or null.

import type {
  Job,
  Card as JobCard,
  JobMatch,
  Blocker,
  Professional,
  Stage,
  Report as ATSReportContract,
  Analysis as MatchAnalysisContract,
  Education as ResumeEducation,
} from './generated/contracts';
export type { Job, Enrichment, Verdict, Gap, SkillRow } from './generated/contracts';
// The list-row projection of a job: the same names and derivations as Job, minus the posting
// text and everything else a row does not draw.
export type { Card as JobCard } from './generated/contracts';
// Per-job profile match (how well a job's skills are covered by the caller's profile).
export type { JobMatch, AdjacentSkill } from './generated/contracts';
// Hard-constraint blockers (years, education, certs, work auth, location) surfaced
// beside skill coverage. BlockerCategory/BlockerSeverity are the enum aliases.
export type { Blocker, BlockerCategory, BlockerSeverity } from './generated/contracts';
// The profile-match endpoint returns skill coverage plus the advisory blockers.
export type JobMatchResult = JobMatch & { blockers: Blocker[] };
// atscheck's Report is aliased ATSReport (a local Report — job reports — already exists);
// its category/line-item shapes come along for the report view.
export type {
  Report as ATSReport,
  ScoreCategory as ATSCategory,
  LineItem as ATSLineItem,
} from './generated/contracts';

/** The CV ATS report response: `has_cv` is false when the caller has no stored CV
 *  (the page prompts an upload); `report` is present only when `has_cv` is true. */
export interface ATSResponse {
  has_cv: boolean;
  report: ATSReportContract | null;
}

// The on-demand LLM job-match analysis wire shapes (five scored dimensions + the
// ATS-style requirement match), generated from internal/matchanalysis.
export type {
  Analysis as MatchAnalysis,
  Dimension as MatchDimension,
  Requirement as MatchRequirement,
  Signal,
} from './generated/contracts';

/** The caller's AI-credits balance. Present on GET reads (with a CV); `remaining` is the
 *  credits left this month and `resets_at` the ISO date the monthly grant renews. A new-job
 *  analysis is blocked when `remaining` is 0; a recompute of an already-analysed job is
 *  always free. */
export interface AiCredits {
  remaining: number;
  resets_at: string;
}

/** What the caller's account did this period (`GET /api/v1/me/usage`), read from the LLM
 *  gateway. Counts, never currency: the gateway's cost figure is a list price against a
 *  mixed upstream pool, so it is neither what we pay nor what the caller pays — their
 *  price is credits, over this same calendar month. */
export interface AiUsage {
  requests: number;
  failed: number;
  tokens: number;
  period: string;
  resets_at: string;
}

/** One row of the Credits transaction history (`GET /api/v1/me/credits/history`), newest
 *  first. `kind` is the ledger kind ('grant' | 'debit' | 'reward' | 'purchase'); `delta` is the
 *  signed change; `label` is the display name (e.g. "Monthly grant", "Match analysis") and
 *  `subtitle`, when present, names the job a metered debit was spent on. `created_at` is ISO. */
export interface CreditHistoryEntry {
  kind: string;
  delta: number;
  label: string;
  subtitle?: string;
  created_at: string;
}

/** The job-match analysis response: `has_cv` is false when the caller has no stored CV
 *  (the block prompts an upload); `analysis` is null when none is cached yet or the LLM
 *  is unconfigured; `stale` marks a cached analysis whose CV or job changed since (the
 *  block then offers a recompute); `credits` reports the points balance on reads (omitted
 *  on the no-CV read and on compute responses). */
export interface MatchAnalysisResponse {
  has_cv: boolean;
  stale: boolean;
  analysis: MatchAnalysisContract | null;
  credits?: AiCredits;
}

/** One row of the Activity → Matches tab: a compact projection of a cached match analysis
 *  (not the full analysis). `closed` marks a job that has since closed; `stale` marks an
 *  analysis whose CV/job/model changed since it ran. `analysed_at` is an ISO timestamp. */
export interface MyAnalysisItem {
  slug: string;
  title: string;
  company: string;
  closed: boolean;
  overall_score: number;
  verdict: string;
  analysed_at: string;
  stale: boolean;
}

/** The lower-coverage extras stored in the company's `company_info` JSONB. Every
 *  field is optional — the loader omits what the source doesn't provide. */
export interface CompanyInfo {
  homepage?: string;
  parent?: string;
  subsidiaries?: string[];
  activities?: string[];
  funding?: {
    type?: string;
    amount?: number;
    year?: number;
    investors?: string[];
  };
  stock?: {
    symbol?: string;
    exchange?: string;
  };
  // Directory extras (populated by cmd/import-yc and cmd/import-hirebase; absent on
  // companies neither importer touched). `website`/`linkedin` are the hirebase
  // outbound links; `description` is the full company summary.
  description?: string;
  website?: string;
  linkedin?: string;
  stage?: string;
  top_company?: boolean;
  is_hiring?: boolean;
  yc_url?: string;
  logo?: string;
  // The rest of what the hirebase importer stores. These have always been on the wire
  // — the API serves `company_info` as raw JSON, so an undeclared key still reaches the
  // browser — they were simply never typed or rendered. Sampled across 40 YC companies
  // on 2026-08-16: ceo 25, twitter 22, facebook 21, instagram 5, locations 27.
  ceo?: string;
  twitter?: string;
  facebook?: string;
  instagram?: string;
  /** Countries the company has an office in, as ISO 3166-1 alpha-2 plus a display
   *  name. Distinct from a job's location: these are the employer's sites, not where
   *  any particular role sits. */
  locations?: { code?: string; name?: string }[];
}

export interface Company {
  slug: string;
  name: string;
  // Curated-collection slugs this company belongs to (e.g. yc, bigtech); empty
  // when untagged. Served by the company-detail endpoint (GetCompany SELECT *).
  collections: string[];
  created_at: string | null;
  updated_at: string | null;
  // Authoritative company-info facts (populated by the company-info backfill;
  // absent/empty on unenriched rows). GetCompany is SELECT *, so these ride along.
  industries?: string[];
  year_founded?: number | null;
  employee_count?: number | null;
  hq_country?: string | null;
  organization_type?: string | null;
  tagline?: string | null;
  company_info?: CompanyInfo;
  // Public thumbs counters (distinct signed-in up/down voters), served on every
  // company read. my_vote is the caller's own vote (-1, 0, 1) — only non-zero on
  // the auth-aware detail read.
  upvote_count: number;
  downvote_count: number;
  my_vote: number;
  // Materialized feedback counters (internal/companyfeedback): how many users
  // left a rated review and their average star rating. rating_avg is null while
  // feedback_count is 0.
  feedback_count: number;
  feedback_rating_avg: number | null;
}

/** A company's recomputed materialized feedback counters, carried alongside a
 *  write response (upsert/delete) so the caller can update its own view of the
 *  company without a second round trip. */
export interface CompanyFeedbackSummary {
  feedback_count: number;
  feedback_rating_avg: number | null;
}

/** One user's rating + category + text about a company (internal/companyfeedback).
 *  The author is a pseudonymous persona handle, the same one CommunityThread uses —
 *  the backend never sends the real user id. `company` is set only on the
 *  upsert response (see CompanyFeedbackDialog's submit). */
export interface CompanyFeedback {
  id: number;
  author: string;
  rating: number;
  feedback_type: string;
  body: string;
  created_at: string;
  updated_at: string;
  company?: CompanyFeedbackSummary;
}

/** The moderator queue's shape for a reported review (internal/companyfeedback):
 *  the review itself plus which company it's on and the aggregated report info. */
export interface ReportedCompanyFeedback extends CompanyFeedback {
  company_slug: string;
  report_count: number;
  report_reasons: string[];
}

/** The result of casting or clearing a thumbs vote: the target's resulting public
 *  counters and the caller's own vote (-1, 0, 1). Returned by the vote endpoints
 *  and shaped identically for jobs and companies. */
export interface VoteResult {
  upvote_count: number;
  downvote_count: number;
  my_vote: number;
}

/** A row of the companies catalog: company plus its computed job count and a few
 *  company-info facts for the card (present-only — absent on unenriched rows). */
export interface CompanyListItem {
  slug: string;
  name: string;
  job_count: number;
  tagline?: string | null;
  industries?: string[] | null;
  hq_country?: string | null;
  /** Curated company tags — the catalogue card renders the backer marks from these. */
  collections?: string[] | null;
  /** Materialized feedback counters (internal/companyfeedback), the same fields
   *  the single-company detail view serves. */
  feedback_count: number;
  feedback_rating_avg: number | null;
}

/** Pagination metadata returned alongside list responses. */
export interface ListMeta {
  total: number;
  limit: number;
  offset: number;
}

/** Facet-distribution counts from GET /api/v1/jobs/facets, for the analytics
 *  page. `total` is the estimated vacancy count under the current filters;
 *  `facets` maps each facet param (the same name used to filter, e.g. `regions`,
 *  `seniority`) to a value→count map; `stats` gives numeric facets' min/max
 *  (e.g. `salary_min`). Continuous numeric facets appear only in `stats`. */
export interface FacetCounts {
  total: number;
  facets: Record<string, Record<string, number>>;
  stats: Record<string, { min: number; max: number }>;
}

/** An authenticated account, as returned by the auth endpoints. */
export interface User {
  id: number;
  email: string;
  // Authorization role ('user' | 'moderator' | 'admin'). A UI affordance only —
  // the server re-checks it on every privileged request.
  role: string;
  // Beta-tester group membership, independent of `role`. Gates the agent
  // assistant (/my/assistant) in the UI.
  beta_tester: boolean;
  // Whether the account has proven control of its email address. Drives the
  // "confirm your email" prompt; a UI affordance only, re-checked server-side.
  email_verified: boolean;
  // Whether a password is set at all. False for an OAuth-only account, which has
  // nothing to change and must use the reset flow to set one.
  has_password: boolean;
  created_at: string | null;
  // IANA timezone name (e.g. "Europe/Moscow"), null until set. Read by the
  // profile page's timezone field and the notification-settings missing-timezone hint.
  timezone: string | null;
  // Preferred interface language, one of 'en' | 'ru' | 'es' | 'pt' | 'de' | 'fr'.
  // Never empty ('en' until changed). No UI translation ships yet — this only
  // records the preference for the profile page's language field.
  language: string;
}

/** A crowdsourced board contribution: a job link a user pasted for a company board we do
 *  not crawl yet. The unit is the board — `source` (ATS) + `board` (company slug) — not a
 *  single vacancy; the ingest side later scrapes all its vacancies. */
export type ContributionStatus = 'pending' | 'onboarded' | 'rejected' | 'review';

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
export type IntakeStatus = 'found' | 'tracked' | 'imported' | 'review' | 'queued';

export interface ResolvedLink {
  public_slug: string | null;
  status: IntakeStatus;
  /** The employer, when the catalog already carries it — through any source, not just the one
   *  this link came from. Whether we know the COMPANY is a separate question from what became
   *  of its BOARD, so this rides along with every outcome, not just `tracked`. */
  company_slug?: string;
}

/** Employee referrals. An offer is a member's moderated "I can refer into company X". */
export type ReferralOfferStatus = 'pending' | 'approved' | 'rejected';
export type ReferralRequestStatus = 'sent' | 'contacted' | 'declined';
export type ReferralCvKind = 'original' | 'built';

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
  job_id?: number;
  cv_kind: ReferralCvKind;
  cv_id?: string;
  linkedin_url: string;
  contact_telegram?: string;
  contact_email?: string;
  note?: string;
}

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
export interface PrefillResult {
  title?: string;
  company?: string;
  location?: string;
  description?: string;
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  skills?: string[];
  source?: string;
}

/** Why a job was reported. A closed vocabulary mirroring the backend's
 *  internal/report reasons; labels live in $lib/reports. */
export type ReportReason = 'no_response' | 'not_relevant' | 'spam' | 'fraud' | 'other';

/** A user's report of a problem with a live vacancy. `reporter_email` and the
 *  `job_*` fields are present only on the moderator review queue, never on the
 *  reporter's own create response (which also never carries the reporter id). */
export interface Report {
  id: number;
  reason: ReportReason;
  details: string;
  contact_telegram?: string;
  status: 'pending' | 'resolved' | 'dismissed';
  review_reason?: string;
  reviewed_at?: string | null;
  created_at: string | null;
  reporter_email?: string;
  job_slug?: string;
  job_title?: string;
  /** Present only on a decision response: whether the reporter was actually emailed.
   *  A decision always stands, so `false` means "recorded, nobody told". */
  notified?: boolean;
}

/** The content a user submits when reporting a job. */
export interface ReportInput {
  reason: ReportReason;
  /** Optional elaboration. The reason vocabulary already says what is wrong. */
  details?: string;
  contact_telegram?: string;
}

/** A ghost-signal claim: the caller applied to a job on `applied_on` and was
 *  never answered. Distinct from a moderation Report — it accumulates as
 *  evidence rather than reaching a moderator, and can be withdrawn. */
export interface GhostReportInput {
  /** Plain calendar date (YYYY-MM-DD): a day, not an instant. */
  applied_on: string;
}

/** A signed-in user's interaction with one job: when they viewed it, saved it
 *  for later, and (once they confirm an application) applied. `saved_at` and
 *  `applied_at` are null until then. Returned by the view/apply/save endpoints. */
export interface UserJob {
  job_id: number;
  viewed_at: string;
  saved_at: string | null;
  applied_at: string | null;
  // Swipe-mode triage mark; null until the job is dismissed (swipe left).
  dismissed_at: string | null;
  // Application pipeline stage + free-text notes; null until set via `track`.
  stage: string | null;
  notes: string | null;
}

/** One item of the my-jobs listing: the job in the shared wire shape with the
 *  caller's interaction timestamps riding alongside. */
export interface MyJob {
  /** Addresses the row. The board keys, routes and opens by this rather than by the
   *  posting's slug: an application whose posting was pruned has no slug to borrow. */
  id: string;
  /** Read from the application record, so a card renders whether or not `job` is there. */
  company_slug: string;
  role_title: string;
  /** The card projection of the posting — what a row draws. Null once the catalogue has
   *  removed the posting: the application outlives it. The full posting, description
   *  included, is on the single-application read (`getTrackedApplication`), which the
   *  panel already fetches; the listing carrying it cost 84% of its own payload. */
  job: JobCard | null;
  viewed_at: string;
  saved_at: string | null;
  applied_at: string | null;
  stage: string | null;
  notes: string | null;
  /** Live inbox messages linked to this job — the board card's ✉ badge. 0 for
   *  users without a connected mailbox / no linked mail. */
  email_count: number;
  /** When this application last moved: its apply date, or the newest message
   *  linked to it when that is later. Null on any row that is not an
   *  application awaiting a reply. */
  last_activity_at: string | null;
  /** Whole days since last_activity_at. Null together with the others. */
  days_silent: number | null;
  /** 'active' — inside the tolerated silence for its stage; 'silent' — past it;
   *  'unconfirmed' — it would read as silent, but mail awaiting confirmation may
   *  say otherwise. Null means nothing is owed here (viewed/saved only, or a
   *  settled application), which is not the same as owed-and-answered-promptly. */
  silence_state: 'active' | 'silent' | 'unconfirmed' | null;
  /** When the candidate last recorded chasing this application (RFC3339), or null
   *  for one never chased. Deliberately independent of the silence fields above: a
   *  chase is not a reply, so a chased application keeps counting its silence and
   *  the card shows both readings. */
  followed_up_at: string | null;
  /** When somebody last opened a CV sent for this application. Outside the silence derivation:
   *  a recruiter reading a CV is not a reply. */
  cv_opened_at: string | null;
}

/** The account-level notification rule: whether notifications are on, and the
 *  channels to deliver over. Gates saved-job reminders and both lifecycle nudges
 *  (follow-up, interview-prep) — one shared rule, no per-kind or per-job control. */
export interface NotificationSettings {
  enabled: boolean;
  channels: string[];
  /** 'instant' (default) or 'daily' — governs only saved-search digests, not
   *  reminders/nudges. */
  digest_frequency: string;
  /** "HH:MM" local time, meaningful only when digest_frequency is 'daily'. */
  digest_time: string | null;
  /** "HH:MM" local time; both null means quiet hours are off (the default). */
  quiet_hours_start: string | null;
  quiet_hours_end: string | null;
}

/** The five notification-center event kinds a `user_notifications` row can carry —
 *  matches internal/notify/reminder/nudge's kind strings exactly (see
 *  openspec/changes/add-notification-center/design.md decision 1). */
export type NotificationKind =
  | 'subscription_digest'
  | 'reminder'
  | 'nudge_follow_up'
  | 'nudge_interview_prep'
  | 'nudge_job_closed';

/** One matched job as recorded into a multi-job subscription digest's `jobs`
 *  snapshot — the same {title, company, slug} shape as everywhere else in this
 *  app, no internal id. */
export interface NotificationDigestJob {
  title: string;
  company: string;
  slug: string;
}

/** One row in the notification center (GET /me/notifications) — a durable,
 *  readable-by-the-owner record of a delivery event, independent of which channel
 *  carried it. `public_slug` is the job it concerns, or null for a subscription
 *  digest that matched more than one job (nothing single to deep-link to) — that
 *  case instead carries `jobs`, a snapshot of what matched as of delivery (only
 *  present on the list response for a multi-job digest; absent/null otherwise).
 *  `read_at` is null until the caller marks it read. */
export interface NotificationItem {
  id: number;
  kind: NotificationKind;
  title: string;
  body: string;
  public_slug: string | null;
  jobs?: NotificationDigestJob[] | null;
  created_at: string | null;
  read_at: string | null;
}

/** The classification/link overlay an inbox email carries: its classified status
 *  and, when resolved, the linked application (slug + company) or a pending
 *  suggestion the reading pane confirms inline. All optional/omitempty. */
export interface EmailLinking {
  status_signal?: string;
  link_source?: string;
  linked_slug?: string;
  linked_company?: string;
  suggested_slug?: string;
  suggested_company?: string;
}

/** One email linked to an application, on the application detail page. */
export interface ApplicationEmail {
  id: number;
  source: string;
  from_addr: string;
  from_name: string;
  subject: string;
  status_signal?: string;
  link_source?: string;
  received_at: string;
  read: boolean;
}

/** One message the mailbox sweep proposes for an application. It is a SUGGESTION, not a
 *  link: it is resolved through the same confirm/reject calls the inbox uses. */
export interface RecalledEmail {
  /** Our stored row's id, and 0 for a message the mailbox search found — that one is not
   *  stored at all until you link it. */
  id: number;
  /** Names the message at the mail provider. Present on the search path, and what the link
   *  call is given. */
  provider_id?: string;
  from_addr: string;
  from_name: string;
  subject: string;
  received_at: string;
  /** This message carries a calendar invitation's identifier, so confirming it is what
   *  brings the meeting in. */
  invitation: boolean;
}

/** The result of one sweep. `scanned` is what makes an empty `suggested` legible: nothing
 *  found among forty messages is a different answer from nothing to look at. */
export interface MailRecallResult {
  scanned: number;
  suggested: RecalledEmail[];
  /** How many of the proposed messages carry an invitation identifier. The meetings
   *  themselves arrive on the next calendar sync. */
  invitations: number;
}

/** An offer to move the application to the stage its newest classified message implies.
 *  Present only when the two disagree and the candidate has not already answered — mail
 *  never settles an application by itself, and this is how that rule is stated rather
 *  than left to be inferred from a stage that did not move. */
export interface StageSuggestion {
  stage: string;
  signal: string;
  email_id: number;
}

/** The caller's application for one job slug, with the emails linked to it. */
export interface TrackedApplication {
  job: Job;
  viewed_at: string | null;
  saved_at: string | null;
  applied_at: string | null;
  stage?: string;
  notes?: string;
  /** When the caller last recorded chasing this application, or null for never. */
  followed_up_at: string | null;
  /** When somebody last opened a CV sent for this application. Outside the silence derivation:
   *  a recruiter reading a CV is not a reply. */
  cv_opened_at: string | null;
  emails: ApplicationEmail[];
  stage_suggestion?: StageSuggestion;
  /** The application's history from the ledger, newest first — what happened to it, as
   *  against the marks on the posting the listing row carries. Empty when nothing has. */
  events: TimelineEvent[];
}

/** The assembled follow-up message for a silent application. Deterministic and
 *  template-built server-side — no model, no credits. `recipient` is present only
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

export interface FollowUpDraft {
  subject: string;
  body: string;
  recipient?: string;
  recipient_name?: string;
  days_silent: number;
}

/** Per-tab row counts for the my-jobs page, from the listing's meta. `viewed`
 *  is the view-only subset: rows neither saved nor applied. */
export interface MyJobCounts {
  all: number;
  viewed: number;
  saved: number;
  applied: number;
  dismissed: number;
}

/** The caller's application count at each stage of the vocabulary. Every stage is
 *  present, zero included, and the counts always sum to the application total —
 *  so a count of nothing is readable without being confused for a missing key. */
export type PipelineStageCounts = Record<Stage, number>;

/** The application-pipeline snapshot for the Pipeline tab: the caller's total
 *  application count and the count at each stage. Grouping those stages into the
 *  bands the funnel draws is the renderer's job, from the generated STAGE_GROUPS —
 *  the server does not ship a second vocabulary alongside the first. */
export interface PipelineStats {
  applications: number;
  stages: PipelineStageCounts;
}

/** The bucketing period for the job-activity time series. */
export type ActivityGranularity = 'day' | 'week' | 'month';

/** One period on the job-activity dashboard: the ISO date labelling the bucket
 *  and the counts of vacancies added vs. removed within it. The backend returns a
 *  dense, gap-free series (empty periods carry zeros). */
export interface ActivityPoint {
  period: string;
  added: number;
  removed: number;
}

/** One point on the member-growth series: the ISO date and the cumulative count of
 *  registered members as of that day. The backend returns a dense, gap-free series
 *  (days with no new signups repeat the running total), monotonically
 *  non-decreasing. */
export interface UserGrowthPoint {
  date: string;
  total: number;
}

/** Aggregate engagement counts across all users: jobs saved, applications marked,
 *  jobs viewed, and what people build on top — CVs uploaded and tailored, matches
 *  analyzed, inboxes connected, searches saved. `inboxes_connected` sums live Gmail
 *  grants and claimed hosted mailboxes, so a user holding both counts twice.
 *  Aggregate-only — no per-user or row-level field. */
export interface EngagementStats {
  saved: number;
  applied: number;
  viewed: number;
  cvs_uploaded: number;
  cvs_tailored: number;
  match_analyses: number;
  inboxes_connected: number;
  saved_searches: number;
}

/** How big the catalogue is, as one snapshot.
 *
 *  Every surface that quotes catalogue scale reads this, so two pages rendered from
 *  the same response cannot disagree. `open_jobs` and `companies` are exact counts over
 *  the set the public listings paginate; `sources`, `ats_platforms` and
 *  `telegram_channels` describe reach — what the crawler can read, whether or not each
 *  currently holds an open posting.
 *
 *  `exact` is false when the backend had no published snapshot and fell back to an
 *  approximate open-job count. On that path only `open_jobs`, `sources` and
 *  `ats_platforms` are meaningful; the rest are zero. */
export interface CatalogScale {
  open_jobs: number;
  companies: number;
  sources: number;
  ats_platforms: number;
  telegram_channels: number;
  computed_at: string;
  exact: boolean;
}

/** The derived health verdict for a provider (and the fleet) on the public
 *  /status page. */
export type HealthStatus = 'operational' | 'degraded' | 'down';

/** How a source is shaped: a multi-tenant ATS platform, a many-company job
 *  aggregator, a single company's own careers page, or an unclassified source. */
export type ProviderKind = 'ats' | 'aggregator' | 'company' | 'other';

/** One provider's health on the public /status page: board counts, freshness, and
 *  the derived status. Sanitized — no error text or board identifiers are exposed.
 *  Timestamps are RFC3339 strings, or null when the provider has never run/succeeded. */
export interface ProviderHealth {
  provider: string;
  kind: ProviderKind;
  status: HealthStatus;
  total_boards: number;
  healthy_boards: number;
  cooled_boards: number;
  last_run: string | null;
  last_success: string | null;
  ingested_total: number;
}

/** The public ingest-fleet status: the overall (worst-provider) verdict, the
 *  generation timestamp, and one entry per provider. */
export interface IngestStatus {
  overall: HealthStatus;
  generated_at: string;
  providers: ProviderHealth[];
}

/** An API key as returned by the management endpoints — metadata only; the
 *  plaintext token is never part of this shape. `token_prefix` is a short,
 *  non-secret leading slice (e.g. "fhk_Ab12cd") shown so the user can tell keys
 *  apart. Timestamps are RFC3339 strings or null. */
export interface ApiKey {
  id: number;
  name: string;
  token_prefix: string;
  created_at: string | null;
  last_used_at: string | null;
  expires_at: string | null;
}

/** The response of creating a key: the metadata plus the plaintext `token`,
 *  returned exactly once and never retrievable again. */
export interface CreatedApiKey extends ApiKey {
  token: string;
}

export interface ConnectedIdentity {
  provider: string;
  linked_at: string;
  status: 'active' | 'revocation_pending';
  can_unlink: boolean;
}

export interface ConnectedIdentities {
  has_password: boolean;
  identities: ConnectedIdentity[];
}

/** A user's saved search: a named snapshot of the job-search filter state. `query`
 *  is the canonical search query string (the same the filter URL serializes), so
 *  applying it reproduces the exact filters. An empty `query` is the "show all" view.
 *  Timestamps are RFC3339 strings or null. */
export interface SavedSearch {
  id: number;
  name: string;
  query: string;
  /** Public board slug when the set is shared, or "" when private. Shared boards are
   *  readable by anyone at /b/<public_slug>. */
  public_slug: string;
  /** Optional attribution shown on the public board; "" renders the board anonymously. */
  author_label: string;
  /** True for the single search auto-generated by the "notify me about jobs matching
   *  my profile" toggle — at most one per user. */
  derived_from_profile: boolean;
  created_at: string | null;
  updated_at: string | null;
}

/** A public "board": a shared saved search readable by anyone at /b/<slug>. Carries only
 *  display fields — no owner identity. `query` is the canonical search query string, applied
 *  to the jobs list to render the board's results. `author_label` is "" when anonymous. */
export interface Board {
  name: string;
  query: string;
  author_label: string;
}

/** A user's single profile: their professional self — a non-empty set of
 *  `specializations` (job categories) and a non-empty set of canonical skill tokens. One
 *  per user (no id, no name); the foundation for finding relevant work. Timestamps are
 *  RFC3339 strings or null. */
/** A geographic reach: controlled regions and/or ISO country codes. Empty = anywhere. */
export interface LocationGeoSet {
  regions?: string[];
  countries?: string[];
}

/** The user's current single base: an ISO country code and a free-text city. */
export interface LocationBase {
  country?: string;
  city?: string;
}

/** Willingness to relocate: an open flag plus acceptable destinations (open + empty
 *  targets = anywhere). */
export interface LocationRelocation {
  open: boolean;
  regions?: string[];
  countries?: string[];
  cities?: string[];
}

/** Optional "where & how I want to work" block: accepted work arrangements plus three
 *  geographic parts (remote reach, current base, relocation), freely combined. Mirrors the
 *  backend userprofile.LocationPreferences; null on a profile means the user set none. */
export interface LocationPreferences {
  work_modes?: string[];
  remote: LocationGeoSet;
  base: LocationBase;
  relocation: LocationRelocation;
}

/** Where the CV says the candidate IS, resolved by the location dictionary. Read-only:
 *  it is produced by the CV derivation alone and no profile write can set it. Deliberately
 *  a sibling of location_preferences rather than part of it — that block is what the user
 *  ASSERTED, this is what was DERIVED for them, and only the former may be saved back. */
export interface DerivedLocation {
  countries: string[];
  regions: string[];
  cities: string[];
}

export interface UserProfile {
  specializations: string[];
  skills: string[];
  /** Canonical skill tokens the user wants to avoid; seeded into the jobs filter's skills exclude set by "Apply my profile". Empty when the user excludes nothing. */
  excluded_skills: string[];
  location_preferences: LocationPreferences | null;
  /** Null when nothing was derived (no CV, no current structure, or a location the
   *  dictionary could not resolve). Used to pre-fill "where you're based" for a user who
   *  has stated no base, so they confirm a fact already on their CV instead of retyping it. */
  derived_location: DerivedLocation | null;
  /** The caller's structured CV without its contact fields — served beside the profile so
   *  one read covers both what the user wants and what they have done. Null when no current
   *  structure exists. Contacts come from GET /me/resume instead (the profile page's contact
   *  card reads them there). */
  cv: Professional | null;
  created_at: string | null;
  updated_at: string | null;
}

/** The three Talent Network visibility modes: hidden, discoverable under the candidate's
 *  own name, or discoverable with name and current employer masked. Mirrors the Postgres
 *  enum backing `users.talent_network_visibility` (see internal/handler/me_talent_network.go). */
export type TalentNetworkVisibility = 'off' | 'public' | 'anonymous';

/** The caller's own Talent Network setting. `talent_network_public_id` rides along even
 *  when visibility is "off", so the settings UI can preview the shareable URL before the
 *  candidate turns it on. */
export interface TalentNetworkSetting {
  talent_network_visibility: TalentNetworkVisibility;
  talent_network_public_id: string;
}

/** The public, unauthenticated Talent Network profile page's payload
 *  (`GET /talent-network/:publicID`, internal/handler/talent_network_profile.go). Mirrors
 *  UserProfile's split between facets and CV, but nests the CV under `cv` instead of
 *  flattening it — `Professional` already has its own `skills`, which would otherwise
 *  collide with the top-level facet of the same name. `full_name` is present only in
 *  "public" mode; the backend omits the key entirely in "anonymous" mode rather than
 *  sending an empty string, so there is nothing to accidentally render. Any current-role
 *  employer masking in `cv.experience` is applied server-side — this type does not
 *  distinguish it. */
export interface TalentNetworkProfile {
  full_name?: string;
  specializations: string[];
  skills: string[];
  cv: Professional;
}

/** A notification subscription on a saved search. */
export interface Subscription {
  id: number;
  saved_search_id: number;
  saved_search_name?: string;
  channel: string;
  active: boolean;
  created_at: string | null;
}

/** Telegram link status for the current user. `enabled` is whether the feature is
 *  configured server-side at all (so the UI can hide it); `linked` is whether this
 *  user has connected their chat. */
export interface TelegramStatus {
  enabled: boolean;
  linked: boolean;
  chat_id?: number;
}

/** Discord link status for the current user (the `/contribute` bot, not a
 *  notification channel). `enabled` is whether the feature is configured
 *  server-side at all; `linked` is whether this user has connected their account. */
export interface DiscordStatus {
  enabled: boolean;
  linked: boolean;
  discord_id?: number;
}

/** A one-time token to run `/link token:<token>` in the freehire Discord server,
 *  plus the exact instruction text to show the user. */
export interface DiscordLinkResult {
  token: string;
  instructions: string;
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
export type {
  Structured as ResumeStructured,
  Experience as ResumeExperience,
} from './generated/contracts';
export type { ResumeEducation };

/** Candidate-owned overrides, editable independently of the CV parse that seeded them
 *  (`GET /me/resume`'s `contacts`, and the body of `PUT /me/resume/contacts`). Despite
 *  the name (kept for the route/wire field, `contacts`), this covers identity
 *  (full_name/email/phone/location/links) *and* the flat part of the semantic body
 *  (headline/summary/languages/certifications/education) — see
 *  `internal/resume/owned.go`. A PUT replaces the whole block, so a caller editing only
 *  some of these fields must spread the current object first (see
 *  CandidateContactsEditor.svelte / CvSummaryCard.svelte / EducationCard.svelte). */
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
  // The five body fields above are owned per field, and a non-empty value has always
  // implied that. But clearing one to "" leaves no non-empty value to signal it, so the
  // editor that owns that field sends its *_set flag alongside a save that empties it —
  // see internal/resume/owned.go's Owned for why identity fields need no equivalent.
  headline_set?: boolean;
  summary_set?: boolean;
  languages_set?: boolean;
  certifications_set?: boolean;
  education_set?: boolean;
}

/** The résumé status (`GET /me/resume`): storage flags, owned contacts, parse status,
 *  and the structured résumé (current semantic sections when stamped). */
export interface ResumeMeta {
  enabled: boolean;
  present: boolean;
  uploaded_at: string | null;
  structured: import('./generated/contracts').Structured | null;
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

/** A community discussion thread (see the add-community-threads change). The author
 *  is a pseudonymous persona handle — the backend never sends the real user id. */
export interface CommunityThread {
  id: number;
  subject_type: string;
  subject_slug: string;
  title: string;
  body: string;
  author: string;
  reply_count: number;
  status: string;
  created_at: string;
}

/** A reply within a thread, optionally nested. `author` is a persona handle, or
 *  "AI" for a (future) system-authored reply. `parent_id` is 0 for a top-level
 *  reply, or another reply's id when nested under it. */
export interface CommunityReply {
  id: number;
  thread_id: number;
  parent_id: number;
  author: string;
  body: string;
  created_at: string;
}

/** One place where evidence was produced: a job or a project. Dates are free-form labels
 *  exactly as printed on a CV ("2021-03", "Mar 2021", "Present").
 *  Jobs expose `company`; projects expose the place label as `name` (same storage column). */
export type ExperienceEmployment = {
  id: string;
  kind: 'job' | 'project';
  /** Employer name — present for `kind: 'job'`. */
  company?: string;
  /** Project title — present for `kind: 'project'` (not a company). */
  name?: string;
  role?: string;
  location?: string;
  start?: string;
  end?: string;
  current?: boolean;
  summary?: string;
  /** Portfolio URL — typically set on projects. */
  link?: string;
  stack?: string[];
};

/** How the bank came to hold an achievement. Only the first three may be written into a
 *  CV: they are the candidate speaking. `agent_inferred` is the assistant's own reading,
 *  kept so it can be asked about and barred from the page until confirmed. */
export type ExperienceProvenance = 'cv_import' | 'stated_in_chat' | 'manual' | 'agent_inferred';

/** One piece of evidence, at the grain of a CV bullet.
 *
 *  `needs_context`, `needs_metrics`, and `cluster_id` arrive only on the list
 *  projection — they are computed at read time and must not be POSTed back. */
export type ExperienceAtom = {
  id: string;
  employment_id?: string;
  claim: string;
  context?: string;
  metrics?: string[];
  skills?: string[];
  provenance: ExperienceProvenance;
  source_ref?: string;
  needs_context?: boolean;
  needs_metrics?: boolean;
  /** Shared id of a soft-duplicate group (first member), when the list clustered this atom. */
  cluster_id?: string;
};

export type ExperienceEmploymentWithAtoms = ExperienceEmployment & { atoms: ExperienceAtom[] };

/** The bank as its owner sees it. `unplaced` carries achievements belonging to no role —
 *  usually the ones volunteered in conversation, so they are shown rather than hidden. */
export type ExperienceBank = {
  employments: ExperienceEmploymentWithAtoms[];
  unplaced: ExperienceAtom[];
};

/** One thing that happened to one application, as GET /me/timeline serves it.
 *
 *  `kind` and `signal` are values from the server's vocabularies rather than a closed
 *  union: `application_events` splits the two precisely so a growing vocabulary is not a
 *  change to a table that must not change, and a union here would make every addition a
 *  change to the SPA as well. Render an unrecognised kind, do not drop it.
 *
 *  `occurred_at` is an instant, not a day. Which day it falls on depends on the reader's
 *  clock — see calendarModel.
 *
 *  `observed` is the server's verdict on the source: mail-derived events carry a date
 *  somebody other than the candidate set, a stage they moved themselves carries the date
 *  they got around to recording it. Never re-derive this from `source` here; the rule
 *  lives in Go and a second copy would drift from it. */
export interface TimelineEvent {
  id: number;
  kind: string;
  signal?: string;
  source: string;
  observed: boolean;
  occurred_at: string;
  company_slug: string;
  role_title?: string;
  application_id?: number;
  job_slug?: string;
  /** Absent for an event with no message, and for one whose message was deleted — the
   *  deletion hides the content and leaves the event standing. */
  email_id?: number;
  email_subject?: string;
}

/** One interview arranged for an application, as GET /me/interviews serves it.
 *
 *  Deliberately a different type from [[TimelineEvent]], and not a variant of it: an
 *  event happened and cannot change, a meeting is arranged and can be moved or called
 *  off. One shape for both would invite the calendar to draw them with one meaning.
 *
 *  `starts_at` is an instant like `occurred_at`, and belongs to the reader's day for the
 *  same reason — see calendarModel.
 *
 *  `status` is `suggested` when only the meeting's title named the employer, `confirmed`
 *  when the invitation's own identifier attached it, and `cancelled` when the organiser
 *  called it off. A cancelled meeting is still shown: an interview that vanished from a
 *  Thursday cannot be told apart from a calendar that failed to load. */
export interface ScheduledInterview {
  id: number;
  application_id: number;
  starts_at: string;
  /** Absent when the meeting has no end — an all-day entry has none. Never a zero
   *  timestamp: the server omits the field rather than sending the year 1. */
  ends_at?: string;
  title?: string;
  join_url?: string;
  status: string;
  company_slug: string;
  role_title?: string;
  job_slug?: string;
}
