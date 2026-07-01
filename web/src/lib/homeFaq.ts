// Homepage FAQ content — the single source for both the visible section
// (HomeView.svelte) and the FAQPage JSON-LD (routes/+page.svelte). Google
// requires the schema's answers to match the on-page text, so they share this.
// Answers are written answer-first and stay honest to what the product does.

import type { FaqItem } from './seo';

export const HOME_FAQ: FaqItem[] = [
  {
    question: 'What is freehire?',
    answer:
      'freehire is a free, open-source IT job aggregator. It pulls tech job postings straight from company career boards — Greenhouse, Lever, Ashby, Teamtailor and more — removes duplicates, and tags each role with its stack, seniority, salary and location, so you search jobs, not job boards.',
  },
  {
    question: 'Is freehire free to use?',
    answer:
      'Yes. freehire is completely free and open source under the MIT license. There are no paywalls, no sponsored listings, and no résumé harvesting — it works for job seekers, not recruiters.',
  },
  {
    question: 'Where do the job listings come from?',
    answer:
      'Listings come directly from companies’ public applicant tracking systems (ATS), such as Greenhouse, Lever, Ashby, Teamtailor and SuccessFactors. Every posting is normalized to one schema and deduplicated, so the same role never appears twice across boards.',
  },
  {
    question: 'How is freehire different from other job boards?',
    answer:
      'Unlike traditional job boards, freehire sources jobs directly from company career pages instead of paid listings. There are no sponsored posts and no paywalls, and because it is open source, anyone can add a new company or ATS source.',
  },
  {
    question: 'Can I use freehire from the command line or with AI agents?',
    answer:
      'Yes. freehire offers a CLI and an HTTP API, so a script or an AI agent can search, open and track jobs over the same interface — no browser needed. Create an API key and add --json for machine-readable output.',
  },
  {
    question: 'How do I get notified about new jobs?',
    answer:
      'Save a search — your stack, seniority, region and salary — and subscribe to it. When a matching job is added, freehire sends it to you on Telegram as a tidy digest, so the openings come to you.',
  },
];
