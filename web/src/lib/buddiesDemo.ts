import { buddyFeedPosts } from './buddyFeed.js';
import type { BuddyFeedItem } from './types.js';

// The illustrative feed the interview-buddies landing shows in its hero
// (BuddiesHeroDemo.svelte), drawn by the real feed's own BuddyPost so the picture can
// never drift from the product again. Handles come from the real word banks
// (internal/engage/buddy/handle.go) and the companies are invented. Only the event types
// the real feed shows appear: a rejection, a withdrawal or an accepted offer never does.
// `card_summary` is in the dictionaries' own ids, so BuddyIdentity words it as the product does.

const today = '2026-11-08';
const none: BuddyFeedItem['reactions'] = { counts: {}, mine: '' };

export const DEMO_ITEMS: BuddyFeedItem[] = [
  {
    id: 1,
    handle: 'tidy-kestrel-318',
    card_summary: { category: 'backend', seniority: 'senior', country: 'pl' },
    company: { slug: 'globex', name: 'Globex' },
    job_title: 'Senior Backend Engineer',
    job_slug: null,
    job_location: 'Remote, Europe',
    reactions: { counts: { luck: 4, rooting: 2 }, mine: '' },
    type: 'interview',
    day: today,
  },
  ...[
    { name: 'Vandelay', title: 'Backend Engineer', where: 'Berlin' },
    { name: 'Globex', title: 'Platform Engineer', where: 'Remote, Europe' },
    { name: 'Initech', title: 'Go Developer', where: 'Munich' },
  ].map(
    (job, i): BuddyFeedItem => ({
      id: 2 + i,
      handle: 'amber-heron-42',
      card_summary: { category: 'backend', seniority: 'mid', country: 'de' },
      company: { slug: job.name.toLowerCase(), name: job.name },
      job_title: job.title,
      job_slug: null,
      job_location: job.where,
      reactions: none,
      type: 'applied',
      day: today,
    }),
  ),
];

/** The hero's posts: one interview, one day of folded applications. */
export const DEMO_POSTS = buddyFeedPosts(DEMO_ITEMS);
