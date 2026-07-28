// Inbox landing FAQ — the single source for both the visible section
// (InboxLandingView.svelte) and the FAQPage JSON-LD (routes/features/inbox).
// Google requires the schema's answers to match the on-page text, so they share
// this. Answers are written answer-first and stay honest to what the mail stack
// actually does (internal/mailclassify, internal/maillink).

import type { FaqItem } from './seo';

export const INBOX_FAQ: FaqItem[] = [
  {
    question: 'How does freehire get my job emails?',
    answer:
      'Two ways. Every account can claim a freehire address — you forward recruiter mail to it, or use it when you apply, and replies land there directly. Or you connect Gmail read-only, and freehire syncs the job-related mail it finds. Both feed the same inbox; you can use either or both.',
  },
  {
    question: 'Does freehire read all of my email?',
    answer:
      'No. The hosted address only ever receives what you forward or what employers send to it — freehire never sees the rest of your mailbox. The Gmail connection is read-only and scoped to job-related mail: freehire learns which senders write about your applications and syncs those. You can disconnect Gmail or release the freehire address at any time.',
  },
  {
    question: 'How does an email get attached to the right application?',
    answer:
      'By matching the mail thread, or the company name carried in the sender name or subject. When that match is certain, the email is linked to the application automatically. When it is not, freehire offers it as a suggestion for you to confirm — the AI classifier can propose a match, but it never links one on its own.',
  },
  {
    question: 'What statuses can an email be tagged with?',
    answer:
      'Acknowledgement, screening, interview invitation, assessment, offer, rejection, information request, and an unfinished application you started but never submitted. Anything that does not fit one of those is recorded as "other" rather than guessed at.',
  },
  {
    question: 'What if an email is attached to the wrong application?',
    answer:
      'Unlink it and pick the right one — every link is reversible from the message itself. If the mail is about a job you never recorded, one action creates the application from it and links it in the same step, dated by the email rather than by today.',
  },
  {
    question: 'Will it move cards on my tracking board by itself?',
    answer:
      'Only forward, and only for a linked email it is confident about: applied, then screening, responded, interview, offer. A card never moves backwards, an application you already marked as rejected, accepted or withdrawn is never touched automatically, and a rejection email never moves a card on its own — you stay in control of the outcome.',
  },
];
