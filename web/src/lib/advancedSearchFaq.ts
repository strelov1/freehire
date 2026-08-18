// Advanced-search landing FAQ — the single source for both the visible section
// (AdvancedSearchLandingView.svelte) and the FAQPage JSON-LD
// (routes/features/advanced-search). Answers stay honest to the real facet
// registry (web/src/lib/facets.ts) and the saved-search/notify pipeline
// (internal/notify) rather than describing an idealized version of either.

import type { FaqItem } from './seo';

export const ADVANCED_SEARCH_FAQ: FaqItem[] = [
  {
    question: 'How many filters are there?',
    answer:
      'Twenty facets: role, specialization, seniority, skills and AI specialization; region, country, city, work format and relocation; company type, company and industry; employment type, salary currency and English level; posting freshness, language and source; plus curated collections for lists that don’t map to one field, like remote-worldwide roles or YC companies. The filter panel and the CLI’s `facets` command read the same live registry.',
  },
  {
    question: 'Can I exclude a value instead of just not selecting it?',
    answer:
      'For nineteen of the twenty, yes. Click a pill once to include it, again to exclude it, a third time to clear it. Leaving a filter untouched means “don’t care”; excluding a value means “never this one” — ruling out a company you already applied to, a source you don’t trust, or a stack you’re done with, without narrowing anything else. Only the curated-collections facet has no exclude state.',
  },
  {
    question: 'What happens when I save a search?',
    answer:
      'It’s stored on your profile and listed under Saved searches & alerts. Turning on a channel — Telegram, email or push — makes freehire message you the moment a new job matches it, instead of you coming back to check by hand.',
  },
  {
    question: 'Do the same filters work outside the browser?',
    answer:
      'Yes. The public API and the freehire CLI use the same parameter names as the filter panel — role, regions, skills, company_type and the rest. `freehire facets` prints the live vocabulary before you search from a terminal or a script.',
  },
];
