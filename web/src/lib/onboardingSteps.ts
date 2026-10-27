// Which screens the onboarding tour shows, and the logic its screens share: the order
// specializations are offered in, how challenge answers combine, and what helps with each.
//
// Pure and Svelte-free (mirrors onboarding.ts / facetModel.ts), so each rule is
// unit-testable in plain Node rather than only observable by signing in as somebody.

import { JOB_CHALLENGE_JUST_STARTING } from './surveyOptions';

/** One screen of the tour. */
export type StepKind = 'cv' | 'feed' | 'alerts' | 'agent' | 'challenge' | 'tailor' | 'extension';

/** Every step, in the order shown. The CV comes first because a parsed CV fills in the
 *  feed step; the two paid features come last, after everything that is free. */
export const TOUR_STEPS: StepKind[] = ['cv', 'feed', 'alerts', 'agent', 'challenge', 'tailor', 'extension'];

/** The steps that only introduce a feature and store nothing. */
const INTRO_STEPS = new Set<StepKind>(['agent', 'tailor', 'extension']);

/** What this account already has, one flag per step that stores something. */
export interface TourAnswered {
  hasResume: boolean;
  /** The search profile exists, which needs a specialization and a skill. */
  hasFeed: boolean;
  /** The profile alert is subscribed on at least one channel. */
  hasAlert: boolean;
  hasChallenge: boolean;
}

function isAnswered(step: StepKind, a: TourAnswered): boolean {
  switch (step) {
    case 'cv':
      return a.hasResume;
    case 'feed':
      return a.hasFeed;
    case 'alerts':
      return a.hasAlert;
    case 'challenge':
      return a.hasChallenge;
    default:
      return false;
  }
}

/** The steps this account is shown. Every account is routed here once — a new one, and
 *  each existing one again when the tour replaced the wizard — so a step it has already
 *  answered is skipped, and the introductions come along only when there is something to
 *  ask. An empty plan means "nothing to ask": the caller marks onboarding complete without
 *  rendering a screen. */
export function plannedSteps(answered: TourAnswered): StepKind[] {
  const asks = TOUR_STEPS.filter((s) => !INTRO_STEPS.has(s) && !isAnswered(s, answered));
  if (asks.length === 0) return [];
  return TOUR_STEPS.filter((s) => INTRO_STEPS.has(s) || asks.includes(s));
}

/** Specializations with the technical ones first, each half ordered by how many open jobs
 *  it holds. The catalogue's largest categories are not technical (healthcare alone
 *  outnumbers software engineering four to one), while most people signing up are — so a
 *  plain count order buries the answer most of them are looking for. Without counts the
 *  given order is kept within each half (the sort is stable). */
export function orderSpecializations<T extends { value: string }>(
  options: readonly T[],
  counts: Record<string, number> | null,
  tech: ReadonlySet<string>,
): T[] {
  return [...options].sort(
    (a, b) =>
      Number(tech.has(b.value)) - Number(tech.has(a.value)) ||
      (counts ? (counts[b.value] ?? 0) - (counts[a.value] ?? 0) : 0),
  );
}

/** Pick or unpick one challenge answer. "Just starting" says nothing is hard yet, so it
 *  replaces every other answer and any other answer replaces it — the same rule the server
 *  enforces. */
export function toggleChallenge(picked: readonly string[], value: string): string[] {
  if (picked.includes(value)) return picked.filter((v) => v !== value);
  if (value === JOB_CHALLENGE_JUST_STARTING) return [value];
  return [...picked.filter((v) => v !== JOB_CHALLENGE_JUST_STARTING), value];
}

/** The icon a tip is drawn with; the component maps it to a glyph so this module stays
 *  free of Svelte. */
export type TipIcon =
  | 'tailor'
  | 'extension'
  | 'referral'
  | 'tracking'
  | 'feed'
  | 'skills'
  | 'ghost'
  | 'globe'
  | 'filters'
  | 'relocation'
  | 'visa'
  | 'companies'
  | 'buddies'
  | 'mentors'
  | 'language'
  | 'junior';

/** A feature offered for a challenge answer. `href` is a site path, query included. */
export interface ChallengeTip {
  icon: TipIcon;
  title: string;
  body: string;
  href: string;
  pro?: boolean;
}

const TIP = {
  tailor: {
    icon: 'tailor',
    title: 'A CV for each job',
    body: "Rewritten in the posting's own words, as an ATS-readable PDF.",
    href: '/features/tailor',
    pro: true,
  },
  forms: {
    icon: 'extension',
    title: 'Forms filled in for you',
    body: 'The browser extension fills application forms from your profile, on any site.',
    href: '/features/extension',
    pro: true,
  },
  referrals: {
    icon: 'referral',
    title: 'Anonymous employee referrals',
    body: 'Ask someone inside the company to refer you — a referral gets read.',
    href: '/referrals',
  },
  tracking: {
    icon: 'tracking',
    title: 'Track every application',
    body: 'All of them on one board. A quiet one gets a day-counter, so you know when to follow up.',
    href: '/features/tracking',
  },
  feed: {
    icon: 'feed',
    title: 'Your feed is already set',
    body: 'The feed and your alerts follow what you picked a few steps ago.',
    href: '/jobs',
  },
  skills: {
    icon: 'skills',
    title: 'Skills match on every job',
    body: 'Each job shows how many of your skills it asks for.',
    href: '/jobs',
  },
  ghost: {
    icon: 'ghost',
    title: 'Ghost-job warnings',
    body: "Every job shows whether it looks real, so you don't spend time on postings nobody is hiring for.",
    href: '/features/ghost-jobs',
  },
  remoteWorldwide: {
    icon: 'globe',
    title: 'Remote, open worldwide',
    body: 'Remote jobs that hire from any country.',
    href: '/collections/remote-worldwide',
  },
  workFilters: {
    icon: 'filters',
    title: 'Filter by how you work',
    body: 'Remote, hybrid, part-time or contract — one filter each.',
    href: '/features/advanced-search',
  },
  relocation: {
    icon: 'relocation',
    title: 'Jobs that support relocation',
    body: 'A relocation filter shows employers that help you move.',
    href: '/features/advanced-search',
  },
  visaJobs: {
    icon: 'visa',
    title: 'Jobs that sponsor a visa',
    body: 'One filter shows only the jobs that offer visa sponsorship.',
    href: '/jobs?visa_sponsorship=true',
  },
  visaCompanies: {
    icon: 'companies',
    title: 'Companies on visa-sponsor registers',
    body: 'Open roles at employers listed on official government registers of visa sponsors.',
    href: '/collections/visa-sponsors',
  },
  buddies: {
    icon: 'buddies',
    title: 'Interview buddies',
    body: 'Pair up with other candidates and practise interviews together.',
    href: '/my/buddies',
  },
  mentors: {
    icon: 'mentors',
    title: 'Talk to someone who works there',
    body: 'Book a short call with a mentor from the company or the role.',
    href: '/mentors',
  },
  language: {
    icon: 'language',
    title: 'Filter by language',
    body: 'Show only jobs in a language you know, or cap the level of English a job may ask for.',
    href: '/features/advanced-search',
  },
  junior: {
    icon: 'junior',
    title: 'Intern and junior roles',
    body: 'Pick Intern or Junior as your level and the feed shows only those.',
    href: '/jobs',
  },
  organised: {
    icon: 'tracking',
    title: 'Keep it organised from day one',
    body: 'Save jobs and track every application on one board — see the whole search at a glance.',
    href: '/features/tracking',
  },
} satisfies Record<string, ChallengeTip>;

// Keyed by the survey's closed vocabulary (surveyOptions.ts); a test holds every answer to
// at least one tip.
const TIPS: Record<string, ChallengeTip[]> = {
  recruiter_contact: [TIP.tailor, TIP.forms, TIP.referrals, TIP.tracking],
  job_fit: [TIP.feed, TIP.skills, TIP.ghost],
  remote_flexible: [TIP.remoteWorldwide, TIP.workFilters],
  working_abroad: [TIP.relocation, TIP.remoteWorldwide],
  visa_sponsorship: [TIP.visaJobs, TIP.visaCompanies],
  technical_interviews: [TIP.buddies, TIP.mentors],
  english: [TIP.language],
  first_job: [TIP.junior, TIP.buddies],
  just_starting: [TIP.organised],
  other: [TIP.feed, TIP.tracking],
};

/** The tips for the picked answers together, in the order picked, each tip once. */
export function challengeTips(picked: readonly string[]): ChallengeTip[] {
  return [...new Set(picked.flatMap((v) => TIPS[v] ?? []))];
}
