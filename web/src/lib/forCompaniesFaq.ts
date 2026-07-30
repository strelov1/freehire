// /for-companies FAQ — the single source for both the visible section
// (ForCompaniesView.svelte) and the FAQPage JSON-LD. Google requires the schema's
// answers to match the on-page text, so they share this. Answers restate the ingest
// mechanics the page already describes (scheduled crawl, normalize + dedup,
// stale-sweep close) rather than adding new promises.

import type { FaqItem } from './seo';

export const FOR_COMPANIES_FAQ: FaqItem[] = [
  {
    question: 'What does it cost to list our board?',
    answer:
      'Nothing. freehire is a free, open-source, non-commercial aggregator — no listing fee, no paywall, no upsell, and no paid placement that would push your roles above or below anyone else.',
  },
  {
    question: 'Which ATS platforms can we self-add?',
    answer:
      'The multi-tenant boards where a company maps to a single board entry: Greenhouse, Lever, Ashby, Workable, Recruitee, SmartRecruiters, Personio, BambooHR, Workday, Teamtailor and Rippling. On those, listing your company is one line in the source file.',
  },
  {
    question: 'Our ATS is not on that list — can we still be indexed?',
    answer:
      'Yes. freehire is open source, so an adapter for another ATS can be contributed, and the crawler already covers far more providers than the self-serve subset. Open an issue or a pull request on the repository and the board can be onboarded.',
  },
  {
    question: 'How quickly do new roles appear?',
    answer:
      'Your board is polled on a schedule — roughly once a day — and a newly published role reaches the catalogue and search within a few hours of that crawl.',
  },
  {
    question: 'What happens when we take a role down?',
    answer:
      'It closes on its own. Once a vacancy stops appearing on your board, freehire marks it closed and it drops out of search — no action needed on your side. Your ATS stays the source of truth.',
  },
  {
    question: 'Do we have to maintain a second copy of our jobs?',
    answer:
      'No. There is nothing to re-enter and nothing to keep in sync: freehire reads your existing ATS board, so what you publish there is exactly what gets listed, and what you remove gets closed.',
  },
];
