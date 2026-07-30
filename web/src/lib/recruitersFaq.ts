// /recruiters FAQ — the single source for both the visible section
// (RecruitersView.svelte) and the FAQPage JSON-LD. Google requires the schema's
// answers to match the on-page text, so they share this. Answers restate the
// submission flow the page already shows (sign in → submit URL → moderator review →
// live) and deliberately promise no review turnaround, which is not guaranteed.

import type { FaqItem } from './seo';

export const RECRUITERS_FAQ: FaqItem[] = [
  {
    question: 'What does it cost to post a job?',
    answer:
      'Nothing. freehire is an open-source, non-commercial aggregator — no fees, no paywall, no upsell, and no paid placement. A submitted role sits in the same catalogue, ranked the same way, as one crawled from a company board.',
  },
  {
    question: 'Do I need an account to submit?',
    answer:
      'Yes. Submitting is tied to an account so the posting has an owner and you can follow it afterwards — signing in takes a moment, and it is the only thing standing between you and the form.',
  },
  {
    question: 'How does a submission go live?',
    answer:
      'A moderator reads every submission before it is published, so the catalogue stays free of spam and your posting sits among real roles. Once approved, the role joins the public catalogue and search, enriched and deduplicated like every other source.',
  },
  {
    question: 'Can I check the status of a submission?',
    answer:
      'Yes. Every posting you send in appears under "My submissions" in your account with its current state, so you can see whether it is still awaiting review or already live.',
  },
  {
    question: 'Our company already has an ATS board — should I still submit one by one?',
    answer:
      'No, list the whole board instead. If you hire through Greenhouse, Lever, Ashby, Workable or another supported ATS, freehire can crawl every role you publish and close them when you take them down, which is less work than submitting each vacancy by hand.',
  },
];
