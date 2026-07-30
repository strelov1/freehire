// /open FAQ — the single source for both the visible section (routes/open) and the
// FAQPage JSON-LD. Google requires the schema's answers to match the on-page text,
// so they share this. Answers are written answer-first and describe the actual load
// path in routes/open/+page.server.ts, not a promise about it.

import type { FaqItem } from './seo';

export const OPEN_FAQ: FaqItem[] = [
  {
    question: 'Where do these numbers come from?',
    answer:
      "Every figure is read at request time from freehire's own public API — the same endpoints anyone can call — plus the GitHub REST API for the repository stats. Each number on this page links to the endpoint it came from, so you can check it yourself rather than take it on trust.",
  },
  {
    question: 'How fresh are they?',
    answer:
      'The catalogue scale, daily movement and member growth are read live and held for about a minute on the server, then five minutes in your browser. The repository stats refresh hourly, because unauthenticated GitHub calls are capped at 60 per hour. The distribution breakdowns — countries, skills, seniority, work mode — come from a daily rollup, so they lag the live counts by up to a day.',
  },
  {
    question: 'Can I query the same data myself?',
    answer:
      'Yes, and without an account. Job search, company data and facets are public reads on the freehire HTTP API; only personal features such as saving and tracking need a key. The API reference documents every endpoint, and an OpenAPI schema is published for clients and AI agents.',
  },
  {
    question: 'What counts as an open job?',
    answer:
      'A posting freehire has crawled and has not yet detected as closed. Roles are never silently deleted: when a vacancy stops appearing on the source board it is marked closed and drops out of the open count, which is what the "removed" bars on this page track.',
  },
  {
    question: 'Why publish all of this?',
    answer:
      'freehire is free and open source, so the numbers behind its claims should be checkable rather than asserted. Any figure quoted elsewhere on the site — or by an AI assistant citing freehire — should reconcile with this page, which is the live source rather than a rounded snapshot.',
  },
];
