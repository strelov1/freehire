// The wire shapes the API client (api.ts) returns that no other module declares.
// Re-exported from api.ts, so callers keep importing them from there.

import type { ATSReport, Company, CompanyRef, CompanyResponse, Job, ListMeta, Verdict } from './types';
import type { JobMatch } from '#lib/generated/contracts.js';
import type { components } from '#lib/generated/api.js';

/** A page of list items, optionally the total matching the query (endpoints that
 *  report one), and whether more remain. */
export interface Slice<T> {
  items: T[];
  total?: number;
  /** `total` counts what has been read so far, not everything that matches. */
  totalIsLowerBound?: boolean;
  hasMore: boolean;
}

/** GET /companies/:slug: the company, a page of its jobs, and its relations. `parent` and
 *  `subsidiaries` are absent when there are none. */
export interface CompanyDetail {
  company: Company;
  jobs: Job[];
  referral_available: boolean;
  response?: CompanyResponse;
  parent?: CompanyRef;
  subsidiaries?: CompanyRef[];
}

export interface Page<T> {
  data: T[];
  meta: ListMeta;
}

/** What the public unsubscribe page shows. Deliberately small: the token that opens
 *  it is a bearer credential somebody could have forwarded, so it carries the address
 *  the mail already went to, three switches, and the names of the searches those
 *  switches govern — and nothing else about the account. */
export type EmailPrefs = components['schemas']['EmailPrefs'];

/** A patch, not a replace: an omitted switch keeps its stored value.
 *  `deactivateSearches` can only turn a digest OFF — a link that can only subtract
 *  cannot be used to sign anybody up for anything. */
export interface EmailPrefsUpdate {
  alerts?: boolean;
  activity?: boolean;
  news?: boolean;
  deactivateSearches?: number[];
}

/** One entry in a sitemap sub-file: the public slug and its lastmod. Kept slim
 *  on purpose — the sitemap never needs the full job/company row.
 *
 *  `updated_at` is optional because a document indexed before the attribute joined
 *  the shape has none; such a URL ships without a <lastmod> rather than dropping
 *  out of the sitemap. */
export interface SitemapEntry {
  slug: string;
  updated_at?: string;
}

// Aggregate market-insight wire shapes (the /api/v1/insights/* reads).
/** One cell of the role × country grid; `country` is '' for the all-countries column. */
export type InsightRoleCountryCell = components['schemas']['RoleCountryCell'];

/** The grid: its cells, and the countries the endpoint chose as columns, largest first. */
export interface InsightRoleCountries {
  cells: InsightRoleCountryCell[];
  countries: string[];
}

/** One category's market pulse (GET /api/v1/insights/role-pulse): raw figures for the
 *  last 30 days and, for competition and time to close, the 30 before. `web/src/lib/rolePulse.ts` turns them into the
 *  page's figures and decides which are too thin to show. */
export type InsightRolePulse = components['schemas']['RolePulse'];

export interface InsightRole {
  category: string;
  seniority: string;
  open_count: number;
  growth: number;
  /** The three below are present only when a SINGLE role was named (both category
   *  and seniority). Absent on the ranked list, so `skills === undefined` means "this
   *  answer carries no distribution" while `skills: []` means "this role's every skill
   *  fell below the sample floor" — two different facts. */
  sample_size?: number;
  skills?: InsightRoleSkill[];
  /** Present only for a signed-in caller. Zeroed rather than absent when they hold no
   *  skills, so it can be told apart from being signed out.
   *
   *  The GENERATED JobMatch, not a local copy: the handler serialises
   *  internal/candidate/jobmatch.JobMatch verbatim, and cmd/gen-contracts already emits
   *  it. A hand-written twin would drift the moment the Go type gained a field, and
   *  nothing would report it. */
  coverage?: JobMatch;
}

/** One skill inside a role's distribution. `share` divides by the role's `sample_size`
 *  (its postings carrying any tagged skill), NEVER by `open_count`. Reached through
 *  InsightRole rather than exported by name, so there is one way in. */
interface InsightRoleSkill {
  skill: string;
  open_count: number;
  share: number;
}

export type InsightSkill = components['schemas']['SkillInsight'];

/** One month of a category's salary history (`month` is YYYY-MM). */
export type InsightSalaryMonth = components['schemas']['SalaryMonth'];

export type InsightSalaryBand = components['schemas']['SalaryBand'];

export type InsightCompany = components['schemas']['CompanyInsight'];

/** One weekly snapshot in a skill's demand series (GET /me/market-pulse). */
export type SkillPulsePoint = components['schemas']['SkillPulsePoint'];

/** A signed-in user's own profile skill, joined against its retained weekly
 *  history. change_pct is null with fewer than two snapshots, or when the
 *  earliest snapshot's count is zero. Skills never yet seen in an open job are
 *  omitted by the server rather than reported with a fabricated count. */
export type SkillPulse = components['schemas']['SkillPulse'];

/** One posting in a role cluster — a single city's opening under a collapsed role
 *  (see the /jobs/:slug/copies endpoint). Each keeps its own location and apply URL. */
export type JobCopy = components['schemas']['JobCopy'];

/** The public roast's wire shape (POST /api/v1/cv/roast, no session — see RoastCV and its
 *  own roastResponse on the backend): the deterministic ATS report plus a live
 *  market-coverage reading. `role` is the category slug the reading was measured
 *  against, empty when the dictionary resolved none; `market_scoped` says which of
 *  those two happened, so the page never has to infer it from an empty string.
 *  `market` is absent exactly when `market_available` is false (the facet backend was
 *  unreachable) — the page must render that as "unavailable", never as a coverage of
 *  zero, which would read as a real measurement. */
export interface RoastResponse {
  report: ATSReport;
  role: string;
  market_scoped: boolean;
  market_available: boolean;
  market?: Verdict;
}

/** The shape handler.write402 writes: the allowance that ran out, and the upgrade link —
 *  present only when the caller's tier has something above it to buy. Typed loosely
 *  because it arrives as parsed JSON and only these two fields are read. */
export type PlanRefusalBody = { allowance?: { feature?: unknown }; upgrade_url?: unknown };

// --- Gmail inbox wire shapes ---------------------------------------------

export type GmailStatus = components['schemas']['GmailStatus'];

/** The hosted-mailbox option: the caller's address (null when none) + whether
 *  the feature is configured server-side. */
export type MailboxStatus = components['schemas']['MailboxStatus'];

/** The account switcher value: '' = all sources. 'external' is mail the caller's
 *  own agent harness pushed — it has no connection to configure, it simply
 *  arrives. */
export type InboxSource = '' | 'gmail' | 'hosted' | 'external';

/** One row in the flat inbox listing. */
export type InboxMessage = components['schemas']['InboxMessage'];

/** One message in full, for the reading pane. */
export type EmailBody = components['schemas']['InboxEmail'];

export type MyJobsFilter = 'all' | 'viewed' | 'saved' | 'applied' | 'board' | 'dismissed';
