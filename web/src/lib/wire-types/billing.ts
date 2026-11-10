import type { Analysis as MatchAnalysisContract } from '../generated/contracts';

/** Where the caller stands on one metered feature today. Usage against a limit, never a
 *  balance: `used` is what they have spent since midnight UTC, `limit` what the day allows
 *  (absent when `unlimited`), and `resets_at` the ISO instant it starts over. A recompute of
 *  an already-analysed job is always free.
 *
 *  `enforced` says whether the ceiling turns anybody away yet. It is false through the
 *  shadow run, when the server counts a spent allowance and lets the action through anyway,
 *  so nothing here may block on `used >= limit` alone — see `refuses` in #lib/allowance. */
export interface Allowance {
  feature: string;
  used: number;
  limit?: number;
  unlimited: boolean;
  enforced: boolean;
  resets_at: string;
}

/** The caller's plan and every metered feature's standing today (`GET /api/v1/me/plan`).
 *  Every feature is listed, including untouched ones: the surface shows what the plan IS,
 *  and a feature missing because no row exists yet would read as one they do not have. */
/** One thing a visitor may buy (`GET /api/v1/plans`). The money comes from the payment
 *  provider, never from our configuration: a price written beside its id in an env file is a
 *  second source of truth about what something costs, and the two disagree the first time
 *  one is changed — on the page a customer can hold us to. */
export interface PublicPrice {
  id: string;
  /** Smallest currency unit, as the provider stores it. Formatting happens in the view. */
  amount_cents: number;
  currency: string;
  interval: 'month' | 'year';
  /** The price a new subscriber is sold unless they choose otherwise — one per tier. */
  default: boolean;
  /** Which plan this price buys. Derived on the server from the same lists that decide what
   *  a subscription confers, so a column can never offer another plan's price. */
  tier: 'pro' | 'ultra' | 'api' | 'api_data';
}

/** Where a buyer goes to pay, and which offer was applied on the way
 *  (`GET /api/v1/billing/checkout`).
 *
 *  A session carries at most one discount, so this is a single percentage rather than a
 *  list: the provider admits one coupon, and stacking two offers is how a subscription
 *  becomes free by accident. `discount_percent` is 0 when there was none. */
export interface CheckoutSession {
  url: string;
  discount_percent: number;
  /** `'promo'` or `'invite'`, or empty when nothing was applied. */
  discount_source: string;
}

/** This account's invite link and what it has earned (`GET /api/v1/me/invite`).
 *
 *  Counts and a total, naming nobody. Telling a referrer which of their contacts signed up
 *  would disclose that a particular person is looking for work, which is not theirs to
 *  know — so there is no field here that could carry it. */
export interface InviteSummary {
  link: string;
  invitees: number;
  rewarded: number;
  credit_cents: number;
  /** What both sides of an invite are worth, so the page states one number from one place. */
  percent_off: number;
}

/** What one tier allows for one feature in a day. `unlimited` makes `limit` the fair-use
 *  guard behind it rather than a ceiling — the same two field names, in the same meaning,
 *  that every allowance response uses, so a page never has to learn a second vocabulary for
 *  one idea. */
export interface PlanTierAllowance {
  limit: number;
  unlimited: boolean;
}

/** What each plan allows and what Pro costs (`GET /api/v1/plans`). Public and
 *  unauthenticated — a pricing page that needs an account cannot do a pricing page's job.
 *
 *  `features` comes from the same configuration the metering path reads, so the comparison
 *  cannot drift from the limits actually enforced. `enforced` names the features whose
 *  ceiling really refuses today; while the shadow run is on it is empty, and a page claiming
 *  otherwise would be selling a limit nobody meets. */

export interface PlansMatrix {
  features: { feature: string; free: PlanTierAllowance; pro: PlanTierAllowance; ultra: PlanTierAllowance }[];
  prices: PublicPrice[];
  enforced: string[];
}

/** The caller's paid API product entitlement (`GET /api/v1/me/api-plan`). */
export interface ApiPlan {
  tier: '' | 'api' | 'api_data';
  until?: string;
}

/** One charge as the receipt list shows it (`GET /api/v1/billing/subscription`).
 *
 *  Not exported: it is reached through `BillingOverview.invoices`, and nothing outside
 *  this module names it on its own — which is the line knip.config.js draws. */
interface BillingInvoice {
  /** The provider's invoice id. Carried so a list can key on it: two invoices can share a
   *  second, and a duplicate key in an `{#each}` throws and kills the block. */
  id: string;
  date: string;
  amount_cents: number;
  currency: string;
  /** The provider's word: paid, open, void, uncollectible. A failed charge stays visible —
   *  hiding it leaves a subscriber wondering why their card was declined. */
  status: string;
  receipt_url?: string;
}

/** What the caller is paying and what has been taken. Read through to the payment provider
 *  on every request rather than mirrored: a receipt list that has quietly missed a refund is
 *  worse than no receipt list.
 *
 *  `ends_at` replaces `renews_at` after a cancellation — "renews on the 4th" beside "you
 *  cancelled" is the kind of contradiction that generates support mail. */
export interface BillingOverview {
  status: string;
  amount_cents: number;
  currency: string;
  interval: string;
  renews_at?: string;
  ends_at?: string;
  invoices: BillingInvoice[];
  /** True when the provider reports more than one active entitling subscription for this
   *  customer at once — the duplicate-subscription bug, for a customer it already happened
   *  to. Absent (never `false`) for the ordinary case. */
  multiple_subscriptions?: boolean;
}

export interface PlanState {
  plan: 'free' | 'pro' | 'ultra';
  resets_at: string;
  /** When the caller's PAID plan lapses — whichever tier they are on, Pro or Ultra. Absent
   *  on the free plan, and absent once it has lapsed: the server omits a past value, so a
   *  date here is always still in force. Read from the stored column, never from the billing
   *  provider, so this endpoint keeps answering when the provider does not.
   *
   *  The `pro_` spelling is historical and stays because clients read it; what it has always
   *  meant is "when does the plan you are paying for run out". */
  pro_until?: string;
  allowances: Allowance[];
}

/** What the caller's account did this period (`GET /api/v1/me/usage`), read from the LLM
 *  gateway. Counts, never currency: the gateway's cost figure is a list price against a
 *  mixed upstream pool, so it is neither what we pay nor what the caller pays — what they
 *  spend is a plan allowance, over this same UTC day. */
export interface AiUsage {
  requests: number;
  failed: number;
  tokens: number;
  period: string;
  resets_at: string;
}

/** One row of the plan's usage history (`GET /api/v1/me/plan/history`), newest first.
 *  `kind` is 'consume' | 'release' | 'grant'; `feature` names what it was spent on; `label`
 *  is the display name (e.g. "Job analysis") and `subtitle`, when present, names the
 *  vacancy. `day` is the UTC day it counted against; `created_at` is ISO. */
export interface UsageHistoryEntry {
  feature: string;
  day: string;
  kind: string;
  label: string;
  subtitle?: string;
  created_at: string;
}

/** The job-match analysis response: `has_cv` is false when the caller has no stored CV
 *  (the block prompts an upload); `analysis` is null when none is cached yet or the LLM
 *  is unconfigured; `stale` marks a cached analysis whose CV or job changed since (the
 *  block then offers a recompute); `allowance` reports where the caller stands today on
 *  reads (omitted on the no-CV read and on compute responses). `tailor_allowance` rides
 *  along on the same reads because a caller deciding between the two features needs both
 *  ceilings; the job page itself no longer reads it, the pre-flight tailoring dialog
 *  stating that allowance from its own /me/plan read instead. */
export interface MatchAnalysisResponse {
  has_cv: boolean;
  stale: boolean;
  analysis: MatchAnalysisContract | null;
  allowance?: Allowance;
  tailor_allowance?: Allowance;
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
