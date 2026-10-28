// The illustrative example the tailoring landing shows twice: animated in the hero
// (TailorHeroDemo.svelte) and laid out as before/after lines further down. One copy, so
// the two can never tell different stories. Every "after" line restates a fact its
// "before" line already holds — the page sells "invent nothing", and its own example
// must not invent either.

export const DEMO_ROLE = 'Senior Backend Engineer · Payments';

export const DEMO_REQUIREMENTS = [
  { id: 'kafka', label: 'Kafka' },
  { id: 'payments', label: 'Payment systems' },
  { id: 'mentoring', label: 'Mentoring' },
  { id: 'k8s', label: 'Kubernetes operators' },
];

export const DEMO_BULLETS = [
  {
    req: 'kafka',
    before: 'Checkout team: on-call, bug fixes; built the Kafka event pipeline in Go.',
    after: 'Built the Kafka event pipeline behind checkout, in Go.',
    keywords: ['Kafka'],
  },
  {
    req: 'payments',
    before: 'Various backend tasks, incl. migrating the payment ledger to Postgres.',
    after: 'Migrated the payment ledger to Postgres.',
    keywords: ['payment'],
  },
  {
    req: 'mentoring',
    before: 'Code reviews; mentored two new engineers through onboarding.',
    after: 'Mentored new engineers through onboarding and code review.',
    keywords: ['Mentored'],
  },
];

// Nothing in the example history supports it, so the agent asks instead of writing it.
export const DEMO_ASKED = 'k8s';

// Job match before any edit, then after each rewritten bullet.
export const DEMO_SCORES = [54, 68, 79, 86];

const escapeRegExp = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/** Splits text into runs, flagging the ones that match any of the words (case-insensitive). */
export function highlight(text: string, words: string[]): { text: string; hit: boolean }[] {
  if (words.length === 0) return [{ text, hit: false }];
  const lower = new Set(words.map((w) => w.toLowerCase()));
  const re = new RegExp(`(${words.map(escapeRegExp).join('|')})`, 'gi');
  return text
    .split(re)
    .filter((part) => part !== '')
    .map((part) => ({ text: part, hit: lower.has(part.toLowerCase()) }));
}
