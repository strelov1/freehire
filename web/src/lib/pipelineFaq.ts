// How-it-works landing FAQ — the single source for both the visible section
// (HowItWorksLandingView.svelte) and the FAQPage JSON-LD (routes/how-it-works). Answers
// are written from docs/architecture.md and the root AGENTS.md.
import type { FaqItem } from './seo';

export const PIPELINE_FAQ: FaqItem[] = [
  {
    question: 'How fresh is a posting when I see it?',
    answer:
      'Each source is crawled on its own schedule, several times a day for most boards. A posting typically clears ingest, tagging and dedup within the same day it goes up on the source.',
  },
  {
    question: 'Why do some postings have more detail than others?',
    answer:
      'Enrichment runs from a queue and reads the full posting text, so the depth of detail depends on how much the source itself wrote. A one-line listing stays a one-line listing — nothing here is invented to fill a gap.',
  },
  {
    question: 'If a job is on three boards, which one do I see?',
    answer:
      "One listing, picked by preferring an employer's own ATS or careers page over an aggregator's copy of the same role. The listing still tells you where it's actually hosted.",
  },
  {
    question: 'Does deduplication ever merge two different jobs by mistake?',
    answer:
      'It clusters on role and company together, not on title text alone, so two different openings with the same title at the same company stay separate. The clustering is re-run periodically as new postings arrive, which is also how a wrongly split pair gets caught and merged later.',
  },
  {
    question: 'Can search show a job that was already deleted?',
    answer:
      'No — the index is rebuilt from the current catalogue and swapped in as a whole, never patched in place, so a search can only return what was true at that rebuild.',
  },
];
