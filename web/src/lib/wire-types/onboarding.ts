// The CV-first onboarding flow's two public routes (internal/api/handler/onboarding_*.go).
// Neither stores anything: the parse answers with what it read, the match search with
// the jobs that fit, and the browser keeps both until an account exists.

import type { Job, Report as ATSReport } from '../generated/contracts';
import type { ListMeta } from './companies';

/** What `POST /api/v1/onboarding/cv` read from a CV. The model-read fields (name,
 *  headline, country, years, companies) are empty when `structured_available` is false;
 *  the dictionary fields (skills, categories, seniority) and the ATS report are always
 *  there. Contact fields are never part of it, and the name is cut to the first name. */
export interface OnboardingParse {
  structured_available: boolean;
  first_name: string;
  headline: string;
  /** ISO 3166-1 alpha-2, lowercase; empty when the residence resolved to no country. The
   *  residence itself is never sent as text. */
  country: string;
  total_years: number;
  /** At most four, most recent first. */
  companies: string[];
  /** Canonical skill tags. */
  skills: string[];
  /** Classify precedence order, primary first. */
  categories: string[];
  seniority?: string;
  report: ATSReport;
}

/** One ranked job from `POST /api/v1/onboarding/matches`: the same job view the search
 *  serves, plus how well its skills match the posted ones. */
export interface OnboardingMatch {
  job: Job;
  match_percent: number;
}

/** A query param the search did not read (the contract's `IgnoredParam`): unknown, or
 *  known but unreadable. Either way the answer is wider than asked for. */
interface IgnoredParam {
  param: string;
  did_you_mean?: string;
}

export interface OnboardingMatches {
  data: OnboardingMatch[];
  meta: ListMeta & { ignored_params?: IgnoredParam[] };
}
