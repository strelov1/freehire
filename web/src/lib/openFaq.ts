// /open FAQ — the single source for both the visible section (routes/open) and the
// FAQPage JSON-LD. Google requires the schema's answers to match the on-page text,
// so they share this. Answers are written answer-first, kept to two or three
// sentences, and describe the actual load path in routes/open/+page.server.ts, not a
// promise about it. Keep the count even: the section renders in a two-column grid,
// and an odd item leaves a half-width cell hanging.

import type { FaqItem } from './seo';

export const OPEN_FAQ: FaqItem[] = [
  {
    question: 'Where do these numbers come from?',
    answer:
      "Straight from freehire's own public API at request time — the same endpoints anyone can call — plus the GitHub REST API for the repository stats. Each number links to the endpoint behind it, so you can check it rather than trust it.",
  },
  {
    question: 'How fresh are they?',
    answer:
      'Catalogue scale, daily movement, member growth and engagement are read live, then held about a minute on the server and five in your browser. Repository stats refresh hourly, because unauthenticated GitHub calls are capped at 60 per hour. The distribution breakdowns come from a daily rollup, so they lag by up to a day.',
  },
  {
    question: 'Do the engagement counts expose anyone?',
    answer:
      'No. Each one is a single integer total, and the queries behind them select no user id, email or individual row. "Inboxes connected" counts live Gmail grants plus claimed freehire addresses — never a message.',
  },
  {
    question: 'Can I query the same data myself?',
    answer:
      'Yes, and without an account. Job search, company data and facets are public reads; only personal features such as saving and tracking need a key. Every endpoint is documented, with an OpenAPI schema published for clients and AI agents.',
  },
  {
    question: 'What counts as an open job?',
    answer:
      'A posting freehire has crawled and has not yet detected as closed. Roles are never silently deleted: when a vacancy stops appearing on its source board it is marked closed and leaves the open count, which is what the "removed" bars above track.',
  },
  {
    question: 'Why publish all of this?',
    answer:
      'freehire is free and open source, so the numbers behind its claims should be checkable rather than asserted. Any figure quoted elsewhere on the site — or by an AI assistant citing freehire — should reconcile with this page, the live source rather than a rounded snapshot.',
  },
];
