// FAQ for the /jobs-api landing — the single source for both the visible section
// (JobsApiLandingView.svelte) and the FAQPage JSON-LD, which Google drops when the two
// disagree. Answers state what the product does today; what is coming is said to be coming.

import type { FaqItem } from './seo';

export const JOBS_API_FAQ: FaqItem[] = [
  {
    question: 'Can I use the freehire API in a commercial product?',
    answer:
      'Yes, with an API plan. A plan licenses you to store the postings you read and show them in your own product — a job board, a recruiting tool, an agent you sell. Without a plan the API is for personal and non-commercial use. Either way you may not resell the raw data as a dataset or feed of its own.',
  },
  {
    question: 'Do I pay per job, per record or per request?',
    answer:
      'None of those. A plan has a flat monthly price, and every request returns full postings — up to 100 per page — so the size of the catalogue you read does not change the bill.',
  },
  {
    question: 'What is the difference between the API and Data plans?',
    answer:
      'Both carry the same fields, filters, company data and commercial-use licence. The Data plan adds bulk catalogue files: full snapshots as gzip JSON Lines, listed by GET /api/v1/catalogue/snapshots with short-lived download links.',
  },
  {
    question: 'Which plan fits research or skills analytics?',
    answer:
      'The API plan includes the fields, filters and licence needed for research and skills analytics. If your project needs something it does not offer, write to hello@freehire.me — we add features on request.',
  },
  {
    question: 'Can I keep historical postings and train models on them?',
    answer:
      'Yes, with a plan. You may keep the postings you read as a history, analyse how skills, salaries and demand change over time, and use the data to build, train and evaluate statistical or machine-learning models, including in a commercial product. What you may not do is hand the raw postings on as a dataset or feed of their own.',
  },
  {
    question: 'Is there a free tier?',
    answer:
      'Yes: the read API answers without a key for personal and non-commercial use. It is the same API, so you can build against it for free and switch to a plan when you go live.',
  },
  {
    question: 'How do I authenticate?',
    answer:
      'Create a full-access key on your API keys page and send it as an Authorization: Bearer header. Signed-in MCP and CLI connections can also authenticate through OAuth.',
  },
  {
    question: 'How fresh is the data, and do closed jobs disappear?',
    answer:
      'Sources are re-crawled continuously, and a posting that leaves its source is closed rather than deleted. Search returns open postings only; a single posting fetched by its slug still answers after it closes, carrying its closed_at. A change feed with webhooks for new, updated and removed postings is the next thing we are building.',
  },
  {
    question: 'Can I cancel?',
    answer: 'Any time, from the billing portal. The plan stays until the end of the period you paid for.',
  },
  {
    question: 'What happens to the data I stored when my plan ends?',
    answer:
      'We delete nothing on your side. Stop showing the postings in your product; you may keep what you stored for your own internal analysis. What you derived from the data — aggregated figures, trends, indicators and the models you trained — stays yours, and you may keep using it commercially as long as it does not reproduce the postings themselves.',
  },
];
