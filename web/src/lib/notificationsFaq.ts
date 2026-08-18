// Notifications landing FAQ — the single source for both the visible section
// (NotificationsLandingView.svelte) and the FAQPage JSON-LD (routes/features/notifications).
// Google requires the schema's answers to match the on-page text, so they share this.

import type { FaqItem } from './seo';

export const NOTIFICATIONS_FAQ: FaqItem[] = [
  {
    question: 'How fast does a job alert arrive?',
    answer:
      'Choose Instant and it goes out as soon as the next match run finds a job against your saved search, or a daily digest at a time you set — either way, over email, Telegram or push, whichever you connect.',
  },
  {
    question: 'What besides new jobs sends a notification?',
    answer:
      "Your own tracking board. A saved job nudges you after 3 days if you haven't applied, a stalled application follows up at 21, 18, 15, 12 and 5 days of silence depending on the stage, and an interview nudges you the moment it's scheduled.",
  },
  {
    question: 'Can I go quiet for part of the day?',
    answer:
      'Yes — set quiet hours and every reminder, nudge and instant alert waits until the window ends. A daily digest is exempt, since it already fires once at a time you chose.',
  },
  {
    question: 'Which channels are supported?',
    answer: 'Email, Telegram, and push to the freehire mobile app — pick any combination; a channel you never connect is simply skipped, not an error.',
  },
  {
    question: 'Where do I change any of this?',
    answer:
      'One settings page covers every notification the account sends: which channels, how often search alerts fire, and your quiet hours.',
  },
];
