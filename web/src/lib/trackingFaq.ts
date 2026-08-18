// Application-tracking landing FAQ — the single source for both the visible section
// (TrackingLandingView.svelte) and the FAQPage JSON-LD (routes/features/tracking).
// Google requires the schema's answers to match the on-page text, so they share this.

import type { FaqItem } from './seo';

export const TRACKING_FAQ: FaqItem[] = [
  {
    question: 'How does a job end up on my board?',
    answer:
      'Save it or apply to it from anywhere on freehire — the job page, search, or the browser extension — and it lands on your board as a tracked application. Nothing is added without you doing one of those two things first.',
  },
  {
    question: 'What are the stages?',
    answer:
      'Preparing, Applied, Interview and Offer, in that order — the four columns on the board. A settled application, whether it ended in an offer you accepted or one that did not work out, moves into Closed, which is out of the active board so it stops competing for your attention.',
  },
  {
    question: 'Does it notice when an employer goes quiet?',
    answer:
      "Yes. Once an application has gone unanswered past a point worth noticing, its card carries a day-counter — days since the last contact — so a stalled application stays visible instead of quietly aging at the bottom of a column.",
  },
  {
    question: 'Can a reply move the card for me?',
    answer:
      'Connect your mail through the freehire inbox and a recruiter reply is tagged with what it says, attached to the application it belongs to, and walks the card to its next stage automatically — you only intervene when it guesses wrong.',
  },
  {
    question: 'Is the board the only view?',
    answer:
      'No — the same applications also render as a List for scanning, a Pipeline funnel for the shape of your search, and a Calendar for anything with a date attached, like an interview.',
  },
  {
    question: 'Can I track from the terminal?',
    answer:
      'Yes. The freehire CLI drives the same board with one API key: `save` and `apply` add a job, `stage` moves it, and `note` attaches a private note — so a script or your own agent can keep the board current without a browser.',
  },
];
