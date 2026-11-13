// The onboarding flow's last question, "What's in your way right now?": how its answers
// combine, and what on freehire helps with each. Pure and Svelte-free, so each rule is
// tested in plain Node. The vocabulary and its labels live in surveyOptions.ts.

import { JOB_CHALLENGE_JUST_STARTING, JOB_CHALLENGE_OPTIONS } from './surveyOptions';

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
    title: 'A feed that follows your answers',
    body: 'Once saved, your feed and alert follow the role and place you picked.',
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
    href: '/buddies',
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

export const MAX_TIPS = 4;

/** The tips for the picked answers together, in the order picked, each tip once, at most
 *  `MAX_TIPS` — a short list gets read. */
export function challengeTips(picked: readonly string[]): ChallengeTip[] {
  return [...new Set(picked.flatMap((v) => TIPS[v] ?? []))].slice(0, MAX_TIPS);
}

const LABELS: Record<string, string> = Object.fromEntries(JOB_CHALLENGE_OPTIONS.map((o) => [o.value, o.label]));

/** The answer as one chip: the first pick, and how many more. Null for none. */
export function challengeChip(picked: readonly string[]): string | null {
  const [first, ...rest] = picked;
  if (first === undefined) return null;
  const label = LABELS[first] ?? first;
  return rest.length ? `${label} +${rest.length}` : label;
}
