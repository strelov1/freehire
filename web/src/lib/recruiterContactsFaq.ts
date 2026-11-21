// Recruiter-contacts landing FAQ: the single source for the visible section
// (RecruiterContactsLandingView.svelte) and the FAQPage JSON-LD (routes/features/recruiter-contacts),
// which Google requires to match the page. Honest to internal/api/handler/recruiter_contacts.go:
// a lookup with nothing found is not counted, and the data source is never named.

import type { FaqItem } from './seo';

export const RECRUITER_CONTACTS_FAQ: FaqItem[] = [
  {
    question: 'Where do the contacts come from?',
    answer:
      'Public professional sources: work addresses people at the company have used openly. We show only people whose title says they recruit or work in hiring, and only addresses that look deliverable. Each one is marked Verified or Likely.',
  },
  {
    question: 'What if nobody is found?',
    answer:
      "Then the lookup does not count toward your daily limit. Small companies often have no public recruiter address; for them, the company's careers page or a referral is the better route.",
  },
  {
    question: 'Can I send the same email to all of them?',
    answer:
      'Please do not. Write to one person, about this job, in your own words. The draft button gives you a starting point from your CV and the vacancy that you can edit before sending from your own mail.',
  },
  {
    question: 'How many companies can I look up?',
    answer:
      'Pro and Ultra each include a daily number of lookups; your plan page shows how many are left today. Opening a job you already looked up again is free.',
  },
  {
    question: 'I recruit at a company and do not want to be listed.',
    answer: 'Email us from your work address and we will remove it and keep it from being shown again.',
  },
];
