// Referrals landing FAQ — the single source for both the visible section
// (ReferralsLandingView.svelte) and the FAQPage JSON-LD (routes/features/referrals).
// Google requires the schema's answers to match the on-page text, so they share
// this. Answers stay honest to what the product does (internal/referral).

import type { FaqItem } from './seo';

export const REFERRALS_FAQ: FaqItem[] = [
  {
    question: 'What does it cost?',
    answer:
      'Nothing. freehire is a free, open-source aggregator — referrals included. No fees, no paywall.',
  },
  {
    question: 'Will the referrer see my name?',
    answer:
      'No. Referrers only see the CV, note and contact you attach. Your identity is never surfaced — they reach out only if they decide to take your request forward.',
  },
  {
    question: 'How do I know a referrer actually works there?',
    answer:
      'Anyone offering to refer uploads proof of employment, and a moderator reviews it before the company appears as referral-available.',
  },
  {
    question: 'I work somewhere great — can I help people in?',
    answer:
      'Yes. Offer to refer from your account, upload proof once, and approved requests for your company start reaching you. You stay anonymous throughout.',
  },
];
