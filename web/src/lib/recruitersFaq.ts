// The visible FAQ and its FAQPage JSON-LD share these answers.

import type { FaqItem } from './seo';

export const RECRUITERS_FAQ: FaqItem[] = [
  {
    question: 'What does it cost to post a job?',
    answer: 'Posting a tech vacancy on freehire is free. You can also submit an existing job link for free.',
  },
  {
    question: 'Do I need a company account?',
    answer:
      'To publish and manage your own vacancies, sign in and verify your company using a work email. To submit an existing job link for review, you only need a freehire account.',
  },
  {
    question: 'Can we keep using our current ATS?',
    answer:
      'Yes. When you publish a vacancy, you can direct candidates to your existing ATS or careers page. Choose on-site applications if you want to review applications and email candidates in freehire.',
  },
  {
    question: 'What happens when I submit an existing job link?',
    answer:
      'A moderator checks the submission before it appears in the public catalogue. You can follow the review under “My submissions” in your account.',
  },
  {
    question: 'Can freehire list our whole careers board?',
    answer:
      'If your jobs are on a supported ATS such as Greenhouse, Lever, Ashby, or Workable, freehire can crawl your board and close roles when you take them down.',
  },
];
